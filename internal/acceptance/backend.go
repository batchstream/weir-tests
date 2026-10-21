package acceptance

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"time"

	weir "github.com/batchstream/weir-go"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
	"go.mongodb.org/mongo-driver/v2/mongo/readpref"
	"go.mongodb.org/mongo-driver/v2/mongo/writeconcern"
)

const maxResponse = 16 << 10

type backend interface {
	Owned() bool
	Prepare(context.Context) error
	Oracle(context.Context, int) error
	Close(context.Context) error
	Cleanup(context.Context) error
}

type target struct {
	kind, name, store, owner string
	backend                  backend
}

func (t *target) resource(id string) string {
	base := t.name
	if t.kind == "mongo" {
		base += "/records"
	}
	return base + "/s:" + id
}

func (t *target) document(value int) (*weir.Document, error) {
	var raw []byte
	var err error
	content := "application/json"
	if t.kind == "mongo" {
		fields := bson.D{{Key: "_id", Value: "record"}, {Key: "owner", Value: t.owner}, {Key: "value", Value: int32(value)}}
		raw, err = bson.Marshal(fields)
		content = "application/bson"
	} else {
		fields := map[string]any{"owner": t.owner, "value": value}
		raw, err = json.Marshal(fields)
	}
	doc := &weir.Document{ContentType: content, Data: raw}
	return doc, err
}

func (t *target) validDocument(doc *weir.Document, value int) bool {
	if doc == nil || len(doc.Data) > maxResponse {
		return false
	}
	if t.kind == "mongo" {
		if doc.ContentType != "application/bson" {
			return false
		}
		var fields bson.M
		if bson.Unmarshal(doc.Data, &fields) != nil || len(fields) != 3 {
			return false
		}
		return fields["_id"] == "record" && fields["owner"] == t.owner && number(fields["value"]) == float64(value)
	}
	if doc.ContentType != "application/json" {
		return false
	}
	var fields map[string]any
	if json.Unmarshal(doc.Data, &fields) != nil || len(fields) != 2 {
		return false
	}
	return fields["owner"] == t.owner && number(fields["value"]) == float64(value)
}

func number(value any) float64 {
	switch v := value.(type) {
	case int32:
		return float64(v)
	case int64:
		return float64(v)
	case float64:
		return v
	}
	return -999
}

func connect(config Config, name string) ([]*target, error) {
	opts := mongoOptions(config.MongoURI)
	client, err := mongo.Connect(opts)
	if err != nil {
		return nil, err
	}
	m := &mongoBackend{uri: config.MongoURI, name: name + "_m", owner: config.RunOwner, client: client}
	s := &searchBackend{base: config.SearchURL, name: name + "_s", owner: config.RunOwner, client: searchClient()}
	mt := &target{kind: "mongo", name: m.name, owner: config.RunOwner, store: config.MongoStore, backend: m}
	st := &target{kind: "search", name: s.name, owner: config.RunOwner, store: config.SearchStore, backend: s}
	targets := []*target{mt, st}
	return targets, nil
}

func mongoOptions(uri string) *options.ClientOptions {
	opts := options.Client().ApplyURI(uri).SetDirect(!strings.Contains(uri, "replicaSet=")).
		SetMaxPoolSize(2).SetMinPoolSize(0).SetMaxConnecting(1).
		SetRetryWrites(false).SetRetryReads(false).SetMaxAdaptiveRetries(0).
		SetEnableOverloadRetargeting(false).SetCompressors(nil).
		SetServerMonitoringMode(options.ServerMonitoringModePoll).SetHeartbeatInterval(time.Second).
		SetReadPreference(readpref.Primary()).SetWriteConcern(writeconcern.Majority()).
		SetServerSelectionTimeout(callBudget).SetConnectTimeout(callBudget)
	return opts
}

type mongoBackend struct {
	uri, name, owner string
	client           *mongo.Client
	owned            bool
}

func (m *mongoBackend) Owned() bool { return m.owned }

func (m *mongoBackend) Prepare(ctx context.Context) error {
	filter := bson.D{{Key: "name", Value: m.name}}
	names, err := m.client.ListDatabaseNames(ctx, filter)
	if err != nil || len(names) != 0 {
		return errors.New("mongo_not_fresh")
	}
	// The orchestrator exclusively owns this prefix; retain ambiguous create attempts for cleanup.
	m.owned = true
	return m.client.Database(m.name).CreateCollection(ctx, "records")
}

func (m *mongoBackend) Oracle(ctx context.Context, value int) error {
	collection := m.client.Database(m.name).Collection("records")
	filter := bson.D{{Key: "_id", Value: "record"}}
	raw, err := collection.FindOne(ctx, filter).Raw()
	if err != nil {
		return err
	}
	doc := &weir.Document{ContentType: "application/bson", Data: raw}
	t := &target{kind: "mongo", owner: m.owner}
	if !t.validDocument(doc, value) {
		return errors.New("mongo_oracle_document")
	}
	all := bson.D{}
	count, err := collection.CountDocuments(ctx, all)
	if err != nil || count != 1 {
		return errors.New("mongo_oracle_count")
	}
	return nil
}

func (m *mongoBackend) Close(ctx context.Context) error { return m.client.Disconnect(ctx) }

func (m *mongoBackend) Cleanup(ctx context.Context) error {
	if !m.owned {
		return nil
	}
	opts := mongoOptions(m.uri)
	client, err := mongo.Connect(opts)
	if err != nil {
		return err
	}
	dropErr := client.Database(m.name).Drop(ctx)
	filter := bson.D{{Key: "name", Value: m.name}}
	names, verifyErr := client.ListDatabaseNames(ctx, filter)
	closeErr := client.Disconnect(ctx)
	if len(names) != 0 {
		verifyErr = errors.New("mongo_cleanup_leftovers")
	}
	return errors.Join(dropErr, verifyErr, closeErr)
}

type searchBackend struct {
	base, name, owner string
	client            *http.Client
	owned             bool
}

func searchClient() *http.Client {
	transport := &http.Transport{Proxy: nil, MaxConnsPerHost: 2, MaxIdleConns: 2, MaxIdleConnsPerHost: 2, DisableCompression: true, ResponseHeaderTimeout: callBudget, IdleConnTimeout: time.Second}
	client := &http.Client{Transport: transport, Timeout: callBudget, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	return client
}

type httpCall struct {
	method, path string
	body         []byte
}

func (s *searchBackend) request(ctx context.Context, call httpCall) (int, []byte, error) {
	request, err := http.NewRequestWithContext(ctx, call.method, s.base+call.path, bytes.NewReader(call.body))
	if err != nil {
		return 0, nil, err
	}
	request.Header.Set("Content-Type", "application/json")
	response, err := s.client.Do(request)
	if err != nil {
		return 0, nil, err
	}
	defer response.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(response.Body, maxResponse+1))
	if len(raw) > maxResponse {
		return response.StatusCode, nil, errors.New("search_response_bound")
	}
	return response.StatusCode, raw, err
}

func (s *searchBackend) Owned() bool { return s.owned }

func (s *searchBackend) Prepare(ctx context.Context) error {
	call := httpCall{method: http.MethodHead, path: "/" + s.name}
	status, _, err := s.request(ctx, call)
	if err != nil || status != http.StatusNotFound {
		return errors.New("search_not_fresh")
	}
	s.owned = true
	call = httpCall{method: http.MethodPut, path: "/" + s.name, body: []byte(`{"settings":{"number_of_shards":1,"number_of_replicas":0}}`)}
	status, raw, err := s.request(ctx, call)
	var response struct {
		Acknowledged bool `json:"acknowledged"`
	}
	if err != nil || status != http.StatusOK || json.Unmarshal(raw, &response) != nil || !response.Acknowledged {
		return errors.New("search_create")
	}
	return nil
}

func (s *searchBackend) Oracle(ctx context.Context, value int) error {
	call := httpCall{method: http.MethodGet, path: "/" + s.name + "/_doc/record"}
	status, raw, err := s.request(ctx, call)
	var response struct {
		Found  bool            `json:"found"`
		Index  string          `json:"_index"`
		ID     string          `json:"_id"`
		Source json.RawMessage `json:"_source"`
	}
	if err != nil || status != http.StatusOK || json.Unmarshal(raw, &response) != nil || !response.Found || response.Index != s.name || response.ID != "record" {
		return errors.New("search_oracle_read")
	}
	doc := &weir.Document{ContentType: "application/json", Data: response.Source}
	t := &target{kind: "search", owner: s.owner}
	if !t.validDocument(doc, value) {
		return errors.New("search_oracle_document")
	}
	call = httpCall{method: http.MethodGet, path: "/" + s.name + "/_count"}
	status, raw, err = s.request(ctx, call)
	if err != nil || status != http.StatusOK || !validSearchCount(raw) {
		return errors.New("search_oracle_count")
	}
	return nil
}

func validSearchCount(raw []byte) bool {
	var response struct {
		Count  int `json:"count"`
		Shards struct {
			Total      int `json:"total"`
			Successful int `json:"successful"`
			Failed     int `json:"failed"`
		} `json:"_shards"`
	}
	return json.Unmarshal(raw, &response) == nil && response.Count == 1 && response.Shards.Total == 1 && response.Shards.Successful == 1 && response.Shards.Failed == 0
}

func (s *searchBackend) Close(context.Context) error { s.client.CloseIdleConnections(); return nil }

func (s *searchBackend) Cleanup(ctx context.Context) error {
	if !s.owned {
		return nil
	}
	cleaner := &searchBackend{base: s.base, name: s.name, client: searchClient()}
	defer cleaner.client.CloseIdleConnections()
	call := httpCall{method: http.MethodDelete, path: "/" + s.name}
	status, raw, err := cleaner.request(ctx, call)
	var response struct {
		Acknowledged bool `json:"acknowledged"`
	}
	if err != nil || status != http.StatusOK || json.Unmarshal(raw, &response) != nil || !response.Acknowledged {
		return errors.New("search_cleanup_delete")
	}
	call = httpCall{method: http.MethodHead, path: "/" + s.name}
	status, _, err = cleaner.request(ctx, call)
	if err != nil || status != http.StatusNotFound {
		return errors.New("search_cleanup_leftovers")
	}
	return nil
}
