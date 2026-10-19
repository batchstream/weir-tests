//go:build integration

package integration_test

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	weir "github.com/batchstream/weir-go"
	"go.mongodb.org/mongo-driver/v2/bson"
)

// These tests exercise only consumer parsing, budgets and cleanup. They are not
// evidence of real MongoDB/Search behavior or Store routing.
func nativeTestReply(opts weir.NativeOptions, body []byte, status int) (*weir.NativeResult, error) {
	head := &weir.NativeResponse{}
	if status != 0 {
		metadata := &weir.Document{ContentType: "application/http", Data: fmt.Appendf(nil, "HTTP/1.1 %d %s\r\n\r\n", status, http.StatusText(status))}
		head.Metadata = metadata
	}
	if len(body) != 0 {
		if err := opts.Consume(context.Background(), head, body); err != nil {
			return nil, err
		}
	}
	result := &weir.NativeResult{Response: head, Completion: weir.NativeResponseComplete}
	return result, nil
}

func mongoTestPage(t *testing.T, id int64, batch string) []byte {
	t.Helper()
	documents := bson.A{}
	if batch != "empty" {
		document := bson.D{{Key: "n", Value: int64(1)}}
		documents = append(documents, document)
	}
	field := "nextBatch"
	if batch == "first" {
		field = "firstBatch"
	}
	cursor := bson.D{{Key: "id", Value: id}, {Key: field, Value: documents}}
	command := bson.D{{Key: "ok", Value: float64(1)}, {Key: "cursor", Value: cursor}}
	body, err := bson.Marshal(command)
	if err != nil {
		t.Fatal(err)
	}
	return body
}

func testNativeHTTPRequest(t *testing.T, request *weir.NativeRequest) (*http.Request, map[string]any) {
	t.Helper()
	parsed, err := http.ReadRequest(bufio.NewReader(bytes.NewReader(request.Request.Data)))
	if err != nil {
		t.Fatal(err)
	}
	defer parsed.Body.Close()
	body, err := io.ReadAll(parsed.Body)
	if err != nil {
		t.Fatal(err)
	}
	var value map[string]any
	if len(body) != 0 && json.Unmarshal(body, &value) != nil {
		t.Fatal("invalid Native request JSON", string(body))
	}
	return parsed, value
}

func TestNativePaginationMongoPages(t *testing.T) {
	calls := 0
	invoke := func(_ context.Context, opts weir.NativeOptions) (*weir.NativeResult, error) {
		command := bson.Raw(opts.Request.Request.Data)
		if opts.StoreName != "mongo" || opts.Request.Resource != "owned" {
			t.Fatal("Mongo Native routing changed", opts)
		}
		batch := "next"
		cursor := int64(17)
		if calls == 0 {
			batch = "first"
		} else if command.Lookup("getMore").Int64() != 17 || command.Lookup("collection").StringValue() != "records" || command.Lookup("batchSize").AsInt64() != nativePageSize {
			t.Fatal("wrong Mongo continuation", command)
		}
		if calls == 2 {
			batch, cursor = "empty", 0
		}
		calls++
		return nativeTestReply(opts, mongoTestPage(t, cursor, batch), 0)
	}
	command := bson.D{{Key: "find", Value: "records"}, {Key: "batchSize", Value: nativePageSize}}
	open, err := nativeMongoRequest("owned", command)
	if err != nil {
		t.Fatal(err)
	}
	pager := &nativePager{store: "mongo", database: "owned", collection: "records", open: open, invoke: invoke}
	count := 0
	for !pager.exhausted {
		page, err := pager.next(t.Context())
		if err != nil {
			t.Fatal(err)
		}
		count += len(page)
	}
	if count != 2 || calls != 3 || pager.pages != 3 || pager.mongoCursor != 0 {
		t.Fatal("Mongo page assembly mismatch", count, calls, pager)
	}
	if _, err := pager.next(t.Context()); err == nil || calls != 3 {
		t.Fatal("finished pager made another call")
	}
	if err := pager.close(); err != nil || calls != 3 {
		t.Fatal("exhausted Mongo cursor needed no kill", err, calls)
	}
}

func TestNativePaginationSearchRotatingScrollAndCleanup(t *testing.T) {
	calls := 0
	invoke := func(ctx context.Context, opts weir.NativeOptions) (*weir.NativeResult, error) {
		request, body := testNativeHTTPRequest(t, opts.Request)
		var response string
		switch calls {
		case 0:
			if opts.Request.Resource != "owned" || request.URL.RequestURI() != "/_search?scroll=1m" {
				t.Fatal("wrong Search opening request")
			}
			response = `{"_scroll_id":"a","hits":{"hits":[{"_source":{"n":1}}]}}`
		case 1, 2:
			want := []string{"", "a", "b"}[calls]
			if opts.Request.Resource != "" || request.Method != http.MethodPost || request.URL.Path != "/_search/scroll" || body["scroll_id"] != want {
				t.Fatal("wrong backend-owned scroll continuation", body)
			}
			response = `{"_scroll_id":"b","hits":{"hits":[{"_source":{"n":2}}]}}`
			if calls == 2 {
				response = `{"_scroll_id":"c","hits":{"hits":[]}}`
			}
		case 3:
			ids, valid := body["scroll_id"].([]any)
			deadline, bounded := ctx.Deadline()
			if ctx.Err() != nil || !bounded || time.Until(deadline) > rpcTimeout || request.Method != http.MethodDelete || !valid || len(ids) != 1 || ids[0] != "c" {
				t.Fatal("cleanup did not use latest cursor and independent deadline", body, ctx.Err())
			}
			response = `{"succeeded":true,"num_freed":0}`
		default:
			t.Fatal("extra Search call", calls)
		}
		calls++
		return nativeTestReply(opts, []byte(response), http.StatusOK)
	}
	query := nativeHTTPQuery{resource: "owned", method: http.MethodPost, path: "/_search?scroll=1m"}
	open, err := nativeJSONRequest(query)
	if err != nil {
		t.Fatal(err)
	}
	pager := &nativePager{store: "search", open: open, invoke: invoke}
	count := 0
	for !pager.exhausted {
		page, err := pager.next(t.Context())
		if err != nil {
			t.Fatal(err)
		}
		count += len(page)
	}
	if count != 2 || calls != 3 || pager.scrollID != "c" {
		t.Fatal("Search pages or scroll rotation lost", count, calls, pager)
	}
	if err := pager.close(); err != nil {
		t.Fatal(err)
	}
	if err := pager.close(); err != nil || calls != 4 {
		t.Fatal("cleanup was not idempotent", err, calls)
	}
}

func TestNativePaginationCleanupAfterCanceledQuery(t *testing.T) {
	for _, store := range []string{"mongo", "search"} {
		t.Run(store, func(t *testing.T) {
			calls := 0
			invoke := func(ctx context.Context, opts weir.NativeOptions) (*weir.NativeResult, error) {
				if calls == 0 {
					calls++
					return nil, ctx.Err()
				}
				if ctx.Err() != nil {
					t.Fatal("cleanup inherited canceled query context")
				}
				if deadline, ok := ctx.Deadline(); !ok || time.Until(deadline) > rpcTimeout {
					t.Fatal("cleanup has no short deadline")
				}
				calls++
				if store == "search" {
					return nativeTestReply(opts, []byte(`{"succeeded":true,"num_freed":1}`), http.StatusOK)
				}
				raw := bson.Raw(opts.Request.Request.Data)
				if raw.Lookup("killCursors").StringValue() != "records" {
					t.Fatal("wrong Mongo cleanup", raw)
				}
				command := bson.D{{Key: "ok", Value: float64(1)}, {Key: "cursorsKilled", Value: bson.A{int64(17)}}, {Key: "cursorsNotFound", Value: bson.A{}}}
				body, err := bson.Marshal(command)
				if err != nil {
					t.Fatal(err)
				}
				return nativeTestReply(opts, body, 0)
			}
			pager := &nativePager{store: store, database: "owned", collection: "records", mongoCursor: 17, scrollID: "live", started: true, invoke: invoke}
			ctx, cancel := context.WithCancel(t.Context())
			cancel()
			if _, err := pager.next(ctx); !errors.Is(err, context.Canceled) {
				t.Fatal("canceled query did not fail", err)
			}
			if err := pager.close(); err != nil || calls != 2 || !pager.closed {
				t.Fatal("cursor cleanup after cancellation", err, calls)
			}
		})
	}
}

func TestNativePaginationBounds(t *testing.T) {
	t.Run("pages_reserve_cleanup", func(t *testing.T) {
		calls := 0
		invoke := func(_ context.Context, opts weir.NativeOptions) (*weir.NativeResult, error) {
			calls++
			body := `{"_scroll_id":"live","hits":{"hits":[{"_source":{"n":1}}]}}`
			if calls > nativeMaxPages {
				body = `{"succeeded":true,"num_freed":1}`
			}
			return nativeTestReply(opts, []byte(body), http.StatusOK)
		}
		pager := &nativePager{store: "search", invoke: invoke}
		for range nativeMaxPages {
			if _, err := pager.next(t.Context()); err != nil {
				t.Fatal(err)
			}
		}
		if _, err := pager.next(t.Context()); err == nil || calls != nativeMaxPages {
			t.Fatal("unbounded pagination", err, calls)
		}
		if err := pager.close(); err != nil || calls != nativeMaxPages+1 {
			t.Fatal("page bound prevented cleanup", err, calls)
		}
	})
	for _, bound := range []string{"body", "total", "calls"} {
		t.Run(bound, func(t *testing.T) {
			calls := 0
			invoke := func(_ context.Context, opts weir.NativeOptions) (*weir.NativeResult, error) {
				calls++
				body := []byte("xx")
				if bound == "body" {
					body = bytes.Repeat([]byte("x"), nativeMaxBody+1)
				}
				return nativeTestReply(opts, body, http.StatusOK)
			}
			pager := &nativePager{store: "search", invoke: invoke}
			if bound == "total" {
				pager.bytes = nativeMaxBytes - 1
			}
			if bound == "calls" {
				pager.calls = nativeMaxCalls
			}
			_, body, err := pager.exchange(t.Context(), nil)
			if err == nil || body != nil || bound == "calls" && calls != 0 {
				t.Fatal("Native bound accepted data", err, len(body), calls)
			}
		})
	}
}

func TestNativePaginationMalformedPagesAndBusinessErrors(t *testing.T) {
	for _, body := range []string{
		`{`,
		`{"_scroll_id":"live","hits":{}}`,
		`{"_scroll_id":"live","hits":{"hits":[{}]}}`,
		`{"_scroll_id":"live","timed_out":true,"hits":{"hits":[]}}`,
		`{"_scroll_id":"live","_shards":{"failed":1},"hits":{"hits":[]}}`,
		`{"_scroll_id":"live","hits":{"hits":[{"_source":{}},{"_source":{}}]}}`,
	} {
		pager := &nativePager{store: "search"}
		if _, err := pager.searchPage([]byte(body)); err == nil || pager.exhausted {
			t.Fatal("malformed or partial Search page accepted", body, err)
		}
		if strings.Contains(body, "live") && pager.scrollID != "live" {
			t.Fatal("failed page discarded cleanup identity")
		}
	}
	cursor := bson.D{{Key: "id", Value: int64(17)}, {Key: "firstBatch", Value: bson.A{42}}}
	command := bson.D{{Key: "ok", Value: float64(1)}, {Key: "cursor", Value: cursor}}
	body, err := bson.Marshal(command)
	if err != nil {
		t.Fatal(err)
	}
	pager := &nativePager{store: "mongo"}
	if _, err := pager.mongoPage(body, true); err == nil || pager.mongoCursor != 17 || pager.exhausted {
		t.Fatal("malformed Mongo page lost cleanup identity", err, pager)
	}
	invoke := func(_ context.Context, opts weir.NativeOptions) (*weir.NativeResult, error) {
		return nativeTestReply(opts, []byte(`{"error":{"type":"backend_failure"}}`), http.StatusInternalServerError)
	}
	pager = &nativePager{store: "search", invoke: invoke}
	head, _, err := pager.exchange(t.Context(), nil)
	if err != nil || head == nil {
		t.Fatal("complete HTTP 500 was incorrectly a transport failure", err)
	}
	if _, err := pager.next(t.Context()); err == nil || !strings.Contains(err.Error(), "business status=500") {
		t.Fatal("HTTP 500 was incorrectly a successful query", err)
	}
}

func TestNativePaginationIncompleteTransferAndCleanupFailure(t *testing.T) {
	invoke := func(_ context.Context, _ weir.NativeOptions) (*weir.NativeResult, error) {
		head := &weir.NativeResponse{}
		result := &weir.NativeResult{Response: head, Completion: weir.NativeResponseIncomplete}
		return result, nil
	}
	pager := &nativePager{store: "search", invoke: invoke}
	if _, _, err := pager.exchange(t.Context(), nil); err == nil {
		t.Fatal("partial transfer accepted")
	}
	pager.scrollID = "live"
	pager.invoke = func(_ context.Context, opts weir.NativeOptions) (*weir.NativeResult, error) {
		return nativeTestReply(opts, []byte(`{"succeeded":true,"num_freed":0}`), http.StatusOK)
	}
	if err := pager.close(); err == nil || pager.closed {
		t.Fatal("early-stop cleanup claimed reclamation without acknowledgement", err)
	}
}
