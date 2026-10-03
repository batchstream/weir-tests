//go:build integration

package integration_test

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"

	weir "github.com/batchstream/weir-go"
	"github.com/batchstream/weir-tests/internal/fixture"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

type backendData struct {
	name, collection, searchURL string
	mongo                       *mongo.Collection
	http                        *http.Client
}

type record struct {
	id string
	n  int64
}

type httpRequest struct {
	method, path string
	body         []byte
}

func provisionBackends(t *testing.T, ctx context.Context, cluster *fixture.Cluster) []*backendData {
	t.Helper()
	random := make([]byte, 12)
	if _, err := rand.Read(random); err != nil {
		t.Fatal(err)
	}
	namespace := "weir_it_" + hex.EncodeToString(random)
	mongoOptions := options.Client().ApplyURI(cluster.MongoURI).SetServerSelectionTimeout(rpcTimeout).SetTimeout(rpcTimeout)
	direct, err := mongo.Connect(mongoOptions)
	if err != nil {
		t.Fatal("connect direct MongoDB verifier:", err)
	}
	t.Cleanup(func() {
		cleanup, cancel := context.WithTimeout(context.Background(), rpcTimeout)
		defer cancel()
		if err := direct.Disconnect(cleanup); err != nil {
			t.Error("close direct MongoDB verifier:", err)
		}
	})
	rpc, cancel := context.WithTimeout(ctx, rpcTimeout)
	err = direct.Database(namespace).CreateCollection(rpc, "records")
	cancel()
	if err != nil {
		t.Fatal("create exclusive MongoDB test collection:", err)
	}
	t.Cleanup(func() {
		cleanup, cancel := context.WithTimeout(context.Background(), rpcTimeout)
		defer cancel()
		if err := direct.Database(namespace).Drop(cleanup); err != nil {
			t.Error("drop exclusively created MongoDB test database:", err)
		}
	})
	mongodb := &backendData{
		name: "mongo", collection: namespace + "/records",
		mongo: direct.Database(namespace).Collection("records"),
	}
	search := &backendData{
		name: "search", collection: namespace, searchURL: strings.TrimRight(cluster.SearchURL, "/"),
		http: &http.Client{Timeout: rpcTimeout},
	}
	create := httpRequest{method: http.MethodPut, path: "/" + namespace}
	code, body := search.httpDo(t, ctx, create)
	if code != http.StatusOK {
		t.Fatalf("create exclusive Elasticsearch test index: status=%d body=%s", code, body)
	}
	t.Cleanup(func() {
		cleanup, cancel := context.WithTimeout(context.Background(), rpcTimeout)
		defer cancel()
		remove := httpRequest{method: http.MethodDelete, path: "/" + namespace}
		code, body := search.httpDo(t, cleanup, remove)
		if code != http.StatusOK {
			t.Errorf("drop exclusively created Elasticsearch test index: status=%d body=%s", code, body)
		}
		search.http.CloseIdleConnections()
	})
	backends := []*backendData{mongodb, search}
	return backends
}

func (b *backendData) resource(id string) string {
	return b.collection + "/s:" + weir.EncodeSegment(id)
}

func (b *backendData) write(t *testing.T, id string, n int64) *weir.WriteRequest {
	t.Helper()
	document := &weir.Document{MediaType: "application/json", Data: []byte(fmt.Sprintf("{\"n\":%d}", n))}
	if b.name == "mongo" {
		value := bson.D{{Key: "_id", Value: id}, {Key: "n", Value: n}}
		document = bsonDocument(t, value)
	}
	request := &weir.WriteRequest{Resource: b.resource(id), Document: document}
	return request
}

func (b *backendData) transform(t *testing.T, id string) *weir.AtomicTransformRequest {
	t.Helper()
	expression := &weir.Document{
		MediaType: "application/vnd.weir.search-update.v1+json",
		Data:      []byte("{\"doc\":{\"n\":4}}"),
	}
	if b.name == "mongo" {
		fields := bson.D{{Key: "n", Value: int64(1)}}
		update := bson.D{{Key: "$inc", Value: fields}}
		expression = bsonDocument(t, update)
		expression.MediaType = "application/vnd.weir.mongodb-update.v1+bson"
	}
	request := &weir.AtomicTransformRequest{Resource: b.resource(id), BackendExpression: expression}
	return request
}

func bsonDocument(t *testing.T, value bson.D) *weir.Document {
	t.Helper()
	data, err := bson.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	document := &weir.Document{MediaType: "application/bson", Data: data}
	return document
}

func (b *backendData) number(document *weir.Document) (int64, error) {
	if document == nil {
		return 0, errors.New("missing document")
	}
	if b.name == "mongo" {
		if document.GetMediaType() != "application/bson" {
			return 0, fmt.Errorf("MongoDB document media type %q", document.GetMediaType())
		}
		raw := bson.Raw(document.GetData())
		if err := raw.Validate(); err != nil {
			return 0, err
		}
		number, ok := raw.Lookup("n").AsInt64OK()
		if !ok {
			return 0, errors.New("MongoDB document has no numeric n")
		}
		return number, nil
	}
	var value struct {
		N int64 `json:"n"`
	}
	if document.GetMediaType() != "application/json" {
		return 0, fmt.Errorf("Search document media type %q", document.GetMediaType())
	}
	if err := json.Unmarshal(document.GetData(), &value); err != nil {
		return 0, err
	}
	return value.N, nil
}

func (b *backendData) assertPersisted(t *testing.T, ctx context.Context, id string, want int64) {
	t.Helper()
	rpc, cancel := context.WithTimeout(ctx, rpcTimeout)
	defer cancel()
	if b.name == "mongo" {
		filter := bson.D{{Key: "_id", Value: id}}
		var value struct {
			N int64 `bson:"n"`
		}
		if err := b.mongo.FindOne(rpc, filter).Decode(&value); err != nil {
			t.Fatal("direct MongoDB persistence check:", err)
		}
		if value.N != want {
			t.Fatalf("direct MongoDB n=%d, want %d", value.N, want)
		}
		return
	}
	read := httpRequest{method: http.MethodGet, path: "/" + b.collection + "/_doc/" + id}
	code, body := b.httpDo(t, rpc, read)
	var value struct {
		Found  bool `json:"found"`
		Source struct {
			N int64 `json:"n"`
		} `json:"_source"`
	}
	if code != http.StatusOK || json.Unmarshal(body, &value) != nil || !value.Found || value.Source.N != want {
		t.Fatalf("direct Search persistence check: status=%d body=%s, want n=%d", code, body, want)
	}
}

func (b *backendData) assertMissing(t *testing.T, ctx context.Context, id string) {
	t.Helper()
	rpc, cancel := context.WithTimeout(ctx, rpcTimeout)
	defer cancel()
	if b.name == "mongo" {
		filter := bson.D{{Key: "_id", Value: id}}
		err := b.mongo.FindOne(rpc, filter).Err()
		if !errors.Is(err, mongo.ErrNoDocuments) {
			t.Fatal("direct MongoDB missing check:", err)
		}
		return
	}
	read := httpRequest{method: http.MethodGet, path: "/" + b.collection + "/_doc/" + id}
	code, body := b.httpDo(t, rpc, read)
	if code != http.StatusNotFound {
		t.Fatalf("direct Search missing check: status=%d body=%s", code, body)
	}
}

func (b *backendData) httpDo(t *testing.T, ctx context.Context, request httpRequest) (int, []byte) {
	t.Helper()
	rpc, cancel := context.WithTimeout(ctx, rpcTimeout)
	defer cancel()
	input, err := http.NewRequestWithContext(rpc, request.method, b.searchURL+request.path, bytes.NewReader(request.body))
	if err != nil {
		t.Fatal(err)
	}
	if len(request.body) != 0 {
		input.Header.Set("Content-Type", "application/json")
	}
	response, err := b.http.Do(input)
	if err != nil {
		t.Fatal("direct Search verifier:", err)
	}
	defer response.Body.Close()
	body, err := io.ReadAll(io.LimitReader(response.Body, (1<<20)+1))
	if err != nil || len(body) > 1<<20 {
		t.Fatal("invalid or oversized direct Search verifier response:", err)
	}
	return response.StatusCode, body
}

func (b *backendData) assertRead(t *testing.T, ctx context.Context, client *weir.Client, want record) {
	t.Helper()
	request := &weir.ReadRequest{Resource: b.resource(want.id)}
	options := weir.ReadOneOptions{StoreName: b.name, Request: request}
	rpc, cancel := context.WithTimeout(ctx, rpcTimeout)
	result, err := client.ReadOne(rpc, options)
	cancel()
	b.assertReadResult(t, result, err, want.n)
}

func (b *backendData) assertReadResult(t *testing.T, result *weir.ReadResult, err error, want int64) {
	t.Helper()
	if err != nil || result.GetFailure() != nil || result.GetMissing() {
		t.Fatalf("Read result=%v err=%v", result, err)
	}
	got, decodeError := b.number(result.GetDocument())
	if decodeError != nil || got != want {
		t.Fatalf("Read n=%d err=%v; want %d", got, decodeError, want)
	}
}

func (b *backendData) waitRead(t *testing.T, ctx context.Context, client *weir.Client, want record) {
	t.Helper()
	request := &weir.ReadRequest{Resource: b.resource(want.id)}
	options := weir.ReadOneOptions{StoreName: b.name, Request: request}
	poll(t, ctx, "read-only recovery "+b.name, func(attempt context.Context) error {
		result, err := client.ReadOne(attempt, options)
		if err != nil {
			return err
		}
		if result.GetFailure() != nil || result.GetMissing() {
			return fmt.Errorf("business result: %v", result)
		}
		got, err := b.number(result.GetDocument())
		if err != nil || got != want.n {
			return fmt.Errorf("n=%d err=%v; want %d", got, err, want.n)
		}
		return nil
	})
}
