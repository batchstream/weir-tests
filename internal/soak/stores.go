package soak

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
	"go.mongodb.org/mongo-driver/v2/mongo/readconcern"
	"go.mongodb.org/mongo-driver/v2/mongo/readpref"
)

// store reaches one backend directly, without Weir, to create the workload's
// namespace and check what the database holds. A sequence of 0 means absent.
type store interface {
	resource(id string) string
	contentType() string
	encode(id string, seq int64, padding string) ([]byte, error)
	decode(raw []byte) (int64, error)
	read(ctx context.Context, id string) (int64, error)
	readAll(ctx context.Context, ids []string) (map[string]int64, error)
	connections(ctx context.Context) (int64, error)
	create(ctx context.Context) error
	drop(ctx context.Context) error
	close(ctx context.Context) error
}

const mongoCollection = "records"

type mongoStore struct {
	client    *mongo.Client
	namespace string
	records   *mongo.Collection
}

func openMongo(ctx context.Context, uri, namespace string) (*mongoStore, error) {
	clientOptions := options.Client().ApplyURI(uri).SetAppName("weir-soak").SetTimeout(10 * time.Second)
	client, err := mongo.Connect(clientOptions)
	if err != nil {
		return nil, fmt.Errorf("direct Mongo client: %w", err)
	}
	if err := client.Ping(ctx, readpref.Primary()); err != nil {
		return nil, errors.Join(fmt.Errorf("direct Mongo ping: %w", err), client.Disconnect(ctx))
	}
	// Checks read majority-committed data from the primary, the durability
	// Weir's majority write concern promises for APPLIED writes.
	collectionOptions := options.Collection().SetReadConcern(readconcern.Majority()).SetReadPreference(readpref.Primary())
	records := client.Database(namespace).Collection(mongoCollection, collectionOptions)
	opened := &mongoStore{client: client, namespace: namespace, records: records}
	return opened, nil
}

func (m *mongoStore) resource(id string) string {
	return m.namespace + "/" + mongoCollection + "/s:" + id
}

func (m *mongoStore) contentType() string { return "application/bson" }

func (m *mongoStore) encode(id string, seq int64, padding string) ([]byte, error) {
	document := bson.D{{Key: "_id", Value: id}, {Key: "seq", Value: seq}, {Key: "padding", Value: padding}}
	return bson.Marshal(document)
}

func (m *mongoStore) decode(raw []byte) (int64, error) {
	seq, ok := bson.Raw(raw).Lookup("seq").Int64OK()
	if !ok || seq < 1 {
		return 0, errors.New("document lacks a positive int64 seq")
	}
	return seq, nil
}

func (m *mongoStore) read(ctx context.Context, id string) (int64, error) {
	filter := bson.D{{Key: "_id", Value: id}}
	projection := bson.D{{Key: "seq", Value: 1}}
	findOptions := options.FindOne().SetProjection(projection)
	raw, err := m.records.FindOne(ctx, filter, findOptions).Raw()
	if errors.Is(err, mongo.ErrNoDocuments) {
		return 0, nil
	}
	if err != nil {
		return 0, err
	}
	return m.decode(raw)
}

func (m *mongoStore) readAll(ctx context.Context, ids []string) (map[string]int64, error) {
	projection := bson.D{{Key: "seq", Value: 1}}
	findOptions := options.Find().SetProjection(projection)
	filter := bson.D{}
	cursor, err := m.records.Find(ctx, filter, findOptions)
	if err != nil {
		return nil, err
	}
	defer cursor.Close(ctx)
	found := make(map[string]int64, len(ids))
	for cursor.Next(ctx) {
		id, ok := cursor.Current.Lookup("_id").StringValueOK()
		if !ok {
			return nil, errors.New("document lacks a string _id")
		}
		seq, err := m.decode(cursor.Current)
		if err != nil {
			return nil, fmt.Errorf("document %s: %w", id, err)
		}
		found[id] = seq
	}
	return found, cursor.Err()
}

func (m *mongoStore) connections(ctx context.Context) (int64, error) {
	command := bson.D{{Key: "serverStatus", Value: 1}}
	raw, err := m.client.Database("admin").RunCommand(ctx, command).Raw()
	if err != nil {
		return 0, err
	}
	current, ok := raw.Lookup("connections", "current").AsInt64OK()
	if !ok {
		return 0, errors.New("serverStatus lacks connections.current")
	}
	return current, nil
}

func (m *mongoStore) create(ctx context.Context) error {
	return m.client.Database(m.namespace).CreateCollection(ctx, mongoCollection)
}

func (m *mongoStore) drop(ctx context.Context) error {
	return m.client.Database(m.namespace).Drop(ctx)
}

func (m *mongoStore) close(ctx context.Context) error {
	return m.client.Disconnect(ctx)
}

type searchStore struct {
	base      string
	namespace string
	http      *http.Client
}

func openSearch(ctx context.Context, rawURL, namespace string) (*searchStore, error) {
	parsed, err := url.Parse(rawURL)
	if err != nil || parsed.Scheme != "http" && parsed.Scheme != "https" || parsed.Host == "" || parsed.User != nil {
		return nil, errors.New("Search URL must be an explicit credential-free http(s) root URL")
	}
	transport := &http.Transport{Proxy: nil, MaxIdleConnsPerHost: 64}
	client := &http.Client{Transport: transport, Timeout: 30 * time.Second}
	opened := &searchStore{base: strings.TrimSuffix(rawURL, "/"), namespace: namespace, http: client}
	if _, _, err := opened.request(ctx, http.MethodGet, "/", nil); err != nil {
		return nil, fmt.Errorf("direct Search ping: %w", err)
	}
	return opened, nil
}

func (s *searchStore) resource(id string) string { return s.namespace + "/s:" + id }

func (s *searchStore) contentType() string { return "application/json" }

type searchDocument struct {
	Seq     int64  `json:"seq"`
	Padding string `json:"padding,omitempty"`
}

func (s *searchStore) encode(_ string, seq int64, padding string) ([]byte, error) {
	document := searchDocument{Seq: seq, Padding: padding}
	return json.Marshal(document)
}

func (s *searchStore) decode(raw []byte) (int64, error) {
	var document struct {
		Seq *int64 `json:"seq"`
	}
	if err := json.Unmarshal(raw, &document); err != nil || document.Seq == nil || *document.Seq < 1 {
		return 0, errors.New("document lacks a positive seq")
	}
	return *document.Seq, nil
}

type searchHit struct {
	ID     string          `json:"_id"`
	Found  bool            `json:"found"`
	Source json.RawMessage `json:"_source"`
}

func (s *searchStore) read(ctx context.Context, id string) (int64, error) {
	status, body, err := s.request(ctx, http.MethodGet, "/"+s.namespace+"/_doc/"+url.PathEscape(id)+"?_source_includes=seq", nil)
	if err != nil {
		return 0, err
	}
	var hit searchHit
	if status != http.StatusOK && status != http.StatusNotFound || json.Unmarshal(body, &hit) != nil || hit.ID != id {
		return 0, fmt.Errorf("Search GET status %d", status)
	}
	if !hit.Found {
		return 0, nil
	}
	return s.decode(hit.Source)
}

func (s *searchStore) readAll(ctx context.Context, ids []string) (map[string]int64, error) {
	found := make(map[string]int64, len(ids))
	for start := 0; start < len(ids); start += 1000 {
		chunk := ids[start:min(start+1000, len(ids))]
		query := struct {
			IDs []string `json:"ids"`
		}{IDs: chunk}
		body, err := json.Marshal(query)
		if err != nil {
			return nil, err
		}
		status, raw, err := s.request(ctx, http.MethodPost, "/"+s.namespace+"/_mget?_source_includes=seq", body)
		if err != nil {
			return nil, err
		}
		var reply struct {
			Docs []searchHit `json:"docs"`
		}
		if status != http.StatusOK || json.Unmarshal(raw, &reply) != nil || len(reply.Docs) != len(chunk) {
			return nil, fmt.Errorf("Search _mget status %d", status)
		}
		for _, hit := range reply.Docs {
			if !hit.Found {
				continue
			}
			seq, err := s.decode(hit.Source)
			if err != nil {
				return nil, fmt.Errorf("document %s: %w", hit.ID, err)
			}
			found[hit.ID] = seq
		}
	}
	return found, nil
}

func (s *searchStore) connections(ctx context.Context) (int64, error) {
	status, raw, err := s.request(ctx, http.MethodGet, "/_nodes/stats/http", nil)
	if err != nil {
		return 0, err
	}
	var reply struct {
		Nodes map[string]struct {
			HTTP struct {
				CurrentOpen int64 `json:"current_open"`
			} `json:"http"`
		} `json:"nodes"`
	}
	if status != http.StatusOK || json.Unmarshal(raw, &reply) != nil || len(reply.Nodes) == 0 {
		return 0, fmt.Errorf("Search node stats status %d", status)
	}
	var open int64
	for _, node := range reply.Nodes {
		open += node.HTTP.CurrentOpen
	}
	return open, nil
}

// One shard without replicas keeps a single node green; padding is stored
// only in _source.
const searchIndex = `{"settings":{"number_of_shards":1,"number_of_replicas":0},` +
	`"mappings":{"dynamic":"strict","properties":{"seq":{"type":"long"},` +
	`"padding":{"type":"keyword","index":false,"doc_values":false}}}}`

func (s *searchStore) create(ctx context.Context) error {
	status, raw, err := s.request(ctx, http.MethodPut, "/"+s.namespace, []byte(searchIndex))
	if err != nil {
		return err
	}
	if status != http.StatusOK {
		return fmt.Errorf("create index %s: status %d: %s", s.namespace, status, raw)
	}
	return nil
}

func (s *searchStore) drop(ctx context.Context) error {
	status, _, err := s.request(ctx, http.MethodDelete, "/"+s.namespace, nil)
	if err != nil {
		return err
	}
	if status != http.StatusOK {
		return fmt.Errorf("delete index %s: status %d", s.namespace, status)
	}
	return nil
}

func (s *searchStore) close(context.Context) error {
	s.http.CloseIdleConnections()
	return nil
}

func (s *searchStore) request(ctx context.Context, method, path string, body []byte) (int, []byte, error) {
	request, err := http.NewRequestWithContext(ctx, method, s.base+path, bytes.NewReader(body))
	if err != nil {
		return 0, nil, err
	}
	if body != nil {
		request.Header.Set("Content-Type", "application/json")
	}
	response, err := s.http.Do(request)
	if err != nil {
		return 0, nil, err
	}
	defer response.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(response.Body, 64<<20))
	if err != nil {
		return 0, nil, err
	}
	return response.StatusCode, raw, nil
}
