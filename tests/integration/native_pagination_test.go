//go:build integration

package integration_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"testing"

	weir "github.com/batchstream/weir-go"
	pb "github.com/batchstream/weir-protocol/api/weir/v1"
	"go.mongodb.org/mongo-driver/v2/bson"
)

const (
	nativePageSize = 1
	nativeMaxPages = 8
	nativeMaxCalls = nativeMaxPages + 2 // Reserve cleanup and a closed-cursor probe.
	nativeMaxBody  = 1 << 20
	nativeMaxBytes = nativeMaxBody * nativeMaxCalls
)

type nativeInvoke func(context.Context, weir.NativeOptions) (*weir.NativeResult, error)

func ownerNative(wire pb.StoreServiceClient) nativeInvoke {
	return func(ctx context.Context, opts weir.NativeOptions) (*weir.NativeResult, error) {
		return weir.Native(ctx, wire, opts)
	}
}

type nativeHTTPQuery struct {
	resource, method, path string
	body                   any
}

func nativeJSONRequest(query nativeHTTPQuery) (*weir.NativeRequest, error) {
	var body []byte
	if query.body != nil {
		var err error
		body, err = json.Marshal(query.body)
		if err != nil {
			return nil, err
		}
	}
	request, err := http.NewRequest(query.method, "http://weir.invalid"+query.path, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	request.Header.Set("Content-Type", "application/json")
	return weir.NewHTTPNativeRequest(query.resource, request)
}

func nativeMongoRequest(database string, command bson.D) (*weir.NativeRequest, error) {
	body, err := bson.Marshal(command)
	if err != nil {
		return nil, err
	}
	document := &weir.Document{ContentType: "application/bson", Data: body}
	request := &weir.NativeRequest{Resource: weir.EncodeSegment(database), Request: document}
	return request, nil
}

type nativePager struct {
	store, database, collection string
	open                        *weir.NativeRequest
	invoke                      nativeInvoke
	mongoCursor                 int64
	scrollID                    string
	pages, calls, bytes         int
	started, exhausted, closed  bool
	reclaimed                   bool
}

// exchange checks transfer completion only. Callers must also inspect Mongo ok
// or HTTP status/body; a complete backend error is not a typed Weir failure.
func (p *nativePager) exchange(ctx context.Context, request *weir.NativeRequest) (*weir.NativeResponse, []byte, error) {
	if p.calls >= nativeMaxCalls {
		return nil, nil, errors.New("Native call bound exceeded")
	}
	p.calls++
	var head *weir.NativeResponse
	var body []byte
	opts := weir.NativeOptions{StoreName: p.store, Request: request}
	opts.Consume = func(_ context.Context, response *weir.NativeResponse, chunk []byte) error {
		if response == nil || head != nil && head != response {
			return errors.New("Native metadata changed")
		}
		head = response
		if len(body)+len(chunk) > nativeMaxBody || p.bytes+len(chunk) > nativeMaxBytes {
			return errors.New("Native response byte bound exceeded")
		}
		p.bytes += len(chunk)
		body = append(body, chunk...)
		return nil
	}
	rpc, cancel := context.WithTimeout(ctx, rpcTimeout)
	defer cancel()
	end, err := p.invoke(rpc, opts)
	if err != nil {
		return nil, nil, err
	}
	if end == nil || end.Response == nil || head != nil && end.Response != head || end.Completion != weir.NativeResponseComplete || end.Failure != nil {
		return nil, nil, fmt.Errorf("Native transfer incomplete: %v", end)
	}
	return end.Response, body, nil
}

func (p *nativePager) continuation() (*weir.NativeRequest, error) {
	if p.store == "mongo" {
		command := bson.D{{Key: "getMore", Value: p.mongoCursor}, {Key: "collection", Value: p.collection}, {Key: "batchSize", Value: nativePageSize}}
		return nativeMongoRequest(p.database, command)
	}
	body := map[string]any{"scroll": "1m", "scroll_id": p.scrollID}
	query := nativeHTTPQuery{method: http.MethodPost, path: "/_search/scroll", body: body}
	return nativeJSONRequest(query)
}

func (p *nativePager) next(ctx context.Context) ([]*weir.Document, error) {
	if p.closed || p.exhausted {
		return nil, errors.New("Native pagination already finished")
	}
	if p.pages >= nativeMaxPages {
		return nil, errors.New("Native page bound exceeded")
	}
	request := p.open
	if p.started {
		var err error
		request, err = p.continuation()
		if err != nil {
			return nil, err
		}
	}
	p.pages++
	head, body, err := p.exchange(ctx, request)
	if err != nil {
		return nil, err
	}
	first := !p.started
	p.started = true
	if p.store == "mongo" {
		return p.mongoPage(body, first)
	}
	metadata, err := weir.ParseHTTPNativeResponse(head)
	if err != nil {
		return nil, err
	}
	if metadata.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("Native Search business status=%d body=%s", metadata.StatusCode, body)
	}
	return p.searchPage(body)
}

func mongoBusinessOK(body []byte) (bson.Raw, error) {
	raw := bson.Raw(body)
	if err := raw.Validate(); err != nil {
		return nil, err
	}
	ok, valid := raw.Lookup("ok").AsFloat64OK()
	if !valid || ok != 1 {
		return raw, fmt.Errorf("Native Mongo business error: %s", raw)
	}
	return raw, nil
}

func (p *nativePager) mongoPage(body []byte, first bool) ([]*weir.Document, error) {
	raw, err := mongoBusinessOK(body)
	if err != nil {
		return nil, err
	}
	cursor, valid := raw.Lookup("cursor", "id").Int64OK()
	if !valid {
		return nil, errors.New("Native Mongo cursor id missing")
	}
	// Retain the backend-owned cursor even if the remaining page is malformed,
	// so the caller can still attempt cleanup on the error path.
	p.mongoCursor = cursor
	batchName := "nextBatch"
	if first {
		batchName = "firstBatch"
	}
	array, valid := raw.Lookup("cursor", batchName).ArrayOK()
	if !valid {
		return nil, errors.New("Native Mongo batch missing")
	}
	batch, err := array.Values()
	if err != nil || len(batch) > nativePageSize {
		return nil, fmt.Errorf("invalid Native Mongo batch size=%d: %v", len(batch), err)
	}
	documents := make([]*weir.Document, 0, len(batch))
	for _, value := range batch {
		data, valid := value.DocumentOK()
		if !valid {
			return nil, errors.New("Native Mongo batch item is not a document")
		}
		document := &weir.Document{ContentType: "application/bson", Data: bytes.Clone(data)}
		documents = append(documents, document)
	}
	p.exhausted = cursor == 0
	return documents, nil
}

func (p *nativePager) searchPage(body []byte) ([]*weir.Document, error) {
	var page struct {
		ScrollID string          `json:"_scroll_id"`
		Error    json.RawMessage `json:"error"`
		TimedOut bool            `json:"timed_out"`
		Shards   struct {
			Failed int `json:"failed"`
		} `json:"_shards"`
		Hits *struct {
			Hits []struct {
				Source json.RawMessage `json:"_source"`
			} `json:"hits"`
		} `json:"hits"`
	}
	if err := json.Unmarshal(body, &page); err != nil {
		return nil, err
	}
	if page.ScrollID != "" {
		p.scrollID = page.ScrollID
	}
	if page.ScrollID == "" || page.Hits == nil || page.Hits.Hits == nil || len(page.Error) != 0 || page.TimedOut || page.Shards.Failed != 0 || len(page.Hits.Hits) > nativePageSize {
		return nil, fmt.Errorf("invalid or partial Native Search page: %s", body)
	}
	documents := make([]*weir.Document, 0, len(page.Hits.Hits))
	for _, hit := range page.Hits.Hits {
		if len(hit.Source) == 0 || hit.Source[0] != '{' {
			return nil, errors.New("Native Search hit has no source object")
		}
		document := &weir.Document{ContentType: "application/json", Data: bytes.Clone(hit.Source)}
		documents = append(documents, document)
	}
	p.exhausted = len(documents) == 0
	return documents, nil
}

func (p *nativePager) close() error {
	// Cursor ownership survives cancellation of the query/test context.
	ctx, cancel := context.WithTimeout(context.Background(), rpcTimeout)
	defer cancel()
	if p.closed {
		return nil
	}
	if p.store == "mongo" {
		if p.mongoCursor == 0 {
			p.closed = true
			return nil
		}
		command := bson.D{{Key: "killCursors", Value: p.collection}, {Key: "cursors", Value: bson.A{p.mongoCursor}}}
		request, err := nativeMongoRequest(p.database, command)
		if err != nil {
			return err
		}
		_, body, err := p.exchange(ctx, request)
		if err != nil {
			return err
		}
		raw, err := mongoBusinessOK(body)
		if err != nil {
			return err
		}
		acknowledged := false
		for _, field := range []string{"cursorsKilled", "cursorsNotFound"} {
			array, valid := raw.Lookup(field).ArrayOK()
			if !valid {
				return errors.New("Mongo cursor cleanup acknowledgement missing")
			}
			values, err := array.Values()
			if err != nil {
				return err
			}
			for _, value := range values {
				if id, valid := value.Int64OK(); valid && id == p.mongoCursor {
					acknowledged = true
					if field == "cursorsKilled" {
						p.reclaimed = true
					}
				}
			}
		}
		if !acknowledged {
			return fmt.Errorf("Mongo cursor not reclaimed: %s", raw)
		}
	} else if p.scrollID != "" {
		cleanupBody := map[string]any{"scroll_id": []string{p.scrollID}}
		query := nativeHTTPQuery{method: http.MethodDelete, path: "/_search/scroll", body: cleanupBody}
		request, err := nativeJSONRequest(query)
		if err != nil {
			return err
		}
		head, body, err := p.exchange(ctx, request)
		if err != nil {
			return err
		}
		metadata, err := weir.ParseHTTPNativeResponse(head)
		if err != nil {
			return err
		}
		var cleared struct {
			Succeeded bool `json:"succeeded"`
			Freed     int  `json:"num_freed"`
		}
		if json.Unmarshal(body, &cleared) != nil || !cleared.Succeeded || (!p.exhausted && cleared.Freed < 1) || (metadata.StatusCode != http.StatusOK && !(p.exhausted && metadata.StatusCode == http.StatusNotFound)) {
			return fmt.Errorf("Search scroll not reclaimed: status=%d body=%s", metadata.StatusCode, body)
		}
		p.reclaimed = cleared.Freed > 0
	}
	p.closed = true
	return nil
}

type nativeQuery struct {
	mongoFilter, mongoProjection bson.D
	searchQuery, searchSource    any
}

func (b *backendData) nativePager(invoke nativeInvoke, query nativeQuery) (*nativePager, error) {
	pager := &nativePager{store: b.name, collection: b.collection, invoke: invoke}
	var err error
	if b.name == "mongo" {
		pager.database, pager.collection = b.mongo.Database().Name(), b.mongo.Name()
		command := bson.D{{Key: "find", Value: pager.collection}, {Key: "batchSize", Value: nativePageSize}}
		if query.mongoFilter != nil {
			filter := bson.E{Key: "filter", Value: query.mongoFilter}
			command = append(command, filter)
		}
		if query.mongoProjection != nil {
			projection := bson.E{Key: "projection", Value: query.mongoProjection}
			command = append(command, projection)
		}
		pager.open, err = nativeMongoRequest(pager.database, command)
	} else {
		body := map[string]any{"size": nativePageSize, "sort": []string{"_doc"}}
		if query.searchQuery != nil {
			body["query"] = query.searchQuery
		}
		if query.searchSource != nil {
			body["_source"] = query.searchSource
		}
		request := nativeHTTPQuery{resource: b.collection, method: http.MethodPost, path: "/_search?scroll=1m", body: body}
		pager.open, err = nativeJSONRequest(request)
	}
	return pager, err
}

func cleanupNativePager(t *testing.T, pager *nativePager) {
	t.Helper()
	t.Cleanup(func() {
		if err := pager.close(); err != nil {
			t.Error("caller-owned Native cursor cleanup:", err)
		}
	})
}

func collectNativePages(t *testing.T, ctx context.Context, pager *nativePager) []*weir.Document {
	t.Helper()
	cleanupNativePager(t, pager)
	var documents []*weir.Document
	for !pager.exhausted {
		page, err := pager.next(ctx)
		if err != nil {
			t.Fatal("Native pagination:", err)
		}
		documents = append(documents, page...)
	}
	if err := pager.close(); err != nil {
		t.Fatal("Native pagination cleanup:", err)
	}
	return documents
}
