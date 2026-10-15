package benchmark

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"sync/atomic"
	"testing"
	"time"

	weir "github.com/batchstream/weir-go"
)

func TestPartialUpdateApplicationAcknowledgedMatchesNativeAndPreservesPadding(t *testing.T) {
	mutation := partialUpdateMutation(1, 2)
	var calls atomic.Int32
	handler := http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		calls.Add(1)
		if request.URL.Query().Get("refresh") != "false" || request.URL.Query().Get("retry_on_conflict") != "0" || request.URL.Query().Get("wait_for_active_shards") != "1" {
			t.Error("application acknowledgement changed retry or durability policy", request.URL)
		}
		var payload map[string]any
		if err := json.NewDecoder(request.Body).Decode(&payload); err != nil || !reflect.DeepEqual(payload["doc"], mutation.Document) {
			t.Error("native partial payload differs", payload, err)
		}
		shards := map[string]any{"successful": 1, "failed": 0}
		reply := map[string]any{"_id": mutation.ID, "_index": "fixture-products", "result": "updated", "_shards": shards}
		_ = json.NewEncoder(writer).Encode(reply)
	})
	server := httptest.NewServer(handler)
	defer server.Close()
	config := crawlerClientConfig{InputShape: "single", Visibility: "application_acknowledged", SeedPaddingBytes: 8192, Path: "direct", Profile: "partial-update", SearchURL: server.URL, Namespace: "fixture", Threads: 1, PageSize: 1}
	client, err := newCrawlerClient(t.Context(), config)
	if err != nil {
		t.Fatal(err)
	}
	defer client.close()
	mutations := []crawlerMutation{mutation}
	var ledger crawlerApplicationLedger
	unknown, err := client.execute(t.Context(), mutations, &ledger)
	if err != nil || unknown || calls.Load() != 1 {
		t.Fatal(unknown, err, calls.Load())
	}
	requests, err := crawlerWeirRequests(config, mutations)
	if err != nil || len(requests) != 1 {
		t.Fatal(requests, err)
	}
	if requests[0].Options.Visibility != weir.MutationApplicationAcknowledged || requests[0].Lua != nil {
		t.Fatal("Weir did not use the same application completion boundary", requests[0])
	}
	var payload map[string]any
	if err := json.Unmarshal(requests[0].BackendExpression.Data, &payload); err != nil || !reflect.DeepEqual(payload["doc"], mutation.Document) {
		t.Fatal("Weir patch differs from the native patch", payload, err)
	}
	before := crawlerSeedDocument(mutation.ID, true, config.SeedPaddingBytes)
	after := partialUpdateMerge(before, mutation)
	if len(after["benchmark_padding"].(string)) != 8192 || after["benchmark_padding"] != before["benchmark_padding"] || !reflect.DeepEqual(after["stocks"], before["stocks"]) {
		t.Fatal("partial update did not preserve the padded source and unrelated fields")
	}
}

func TestCrawlerPostflightRefreshPrecedesFullFieldQueryOnlyForApplicationAcknowledged(t *testing.T) {
	for _, refresh := range []bool{false, true} {
		var calls []string
		id := crawlerID(0, 0, "product")
		document := crawlerSeedDocument(id, true, 8192)
		handler := http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
			calls = append(calls, request.URL.Path)
			if request.URL.Path == "/fixture-products,fixture-offers/_refresh" {
				shards := map[string]any{"successful": 2, "failed": 0}
				reply := map[string]any{"_shards": shards}
				_ = json.NewEncoder(writer).Encode(reply)
				return
			}
			total := map[string]any{"value": 1}
			hit := map[string]any{"_id": id, "_index": "fixture-products", "_source": document}
			hits := map[string]any{"total": total, "hits": []any{hit}}
			reply := map[string]any{"hits": hits}
			_ = json.NewEncoder(writer).Encode(reply)
		})
		server := httptest.NewServer(handler)
		expected := map[string]map[string]any{id: document}
		opts := crawlerVerifyOptions{URL: server.URL, Namespace: "fixture", Expected: expected, Refresh: refresh}
		err := verifyCrawler(t.Context(), server.Client(), opts)
		server.Close()
		if err != nil {
			t.Fatal(err)
		}
		want := []string{"/fixture-products,fixture-offers/_search"}
		if refresh {
			want = append([]string{"/fixture-products,fixture-offers/_refresh"}, want...)
		}
		if !reflect.DeepEqual(calls, want) {
			t.Fatal("query-visible completion was weakened or postflight refresh was missing", calls, want)
		}
	}
}

func TestCrawlerPostflightRejectsPartialRefresh(t *testing.T) {
	handler := http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path != "/fixture-products,fixture-offers/_refresh" {
			t.Error("query was attempted despite a failed refresh", request.URL)
		}
		_, _ = writer.Write([]byte(`{"_shards":{"successful":1,"failed":1}}`))
	})
	server := httptest.NewServer(handler)
	defer server.Close()
	opts := crawlerVerifyOptions{URL: server.URL, Namespace: "fixture", Refresh: true}
	if err := verifyCrawler(t.Context(), server.Client(), opts); err == nil {
		t.Fatal("partial refresh acknowledgement was accepted")
	}
}

func TestCrawlerExpiredPlannedDeadlinesDoNotDispatchWrites(t *testing.T) {
	var calls atomic.Int32
	handler := http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) { calls.Add(1) })
	server := httptest.NewServer(handler)
	defer server.Close()
	config := crawlerClientConfig{InputShape: "single", Path: "direct", Profile: "partial-update", SearchURL: server.URL, Namespace: "fixture", Threads: 1, PageSize: 1, Processes: 1, QueuePages: 1, TimeoutNS: int64(time.Millisecond)}
	client, err := newCrawlerClient(t.Context(), config)
	if err != nil {
		t.Fatal(err)
	}
	defer client.close()
	phase := crawlerPhase{Phase: "timed", StartNS: time.Now().Add(-time.Second).UnixNano(), DurationNS: int64(100 * time.Millisecond), ProductsPerSecond: 10}
	results := client.measure(t.Context(), phase)
	result := results[0]
	if result.Offered != 1 || result.TimedOut != 1 || result.Failed != 1 || result.Completed != 0 || calls.Load() != 0 || result.ApplicationRequests.Requests != 0 || len(result.AppliedPages) != 0 {
		t.Fatal("planned deadline was reset at dispatch or unacknowledged work entered the oracle", result, calls.Load())
	}
}

func TestCrawlerCanceledArrivalWaitClosesWorkers(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	config := crawlerClientConfig{Threads: 1, PageSize: 1, Processes: 1, QueuePages: 1, TimeoutNS: int64(time.Second)}
	client := &crawlerClient{config: config}
	phase := crawlerPhase{Phase: "timed", StartNS: time.Now().Add(time.Hour).UnixNano(), DurationNS: int64(time.Second), ProductsPerSecond: 10}
	results := client.measure(ctx, phase)
	if len(results) != 1 || results[0].Offered != 0 || results[0].Failed != 0 {
		t.Fatal("canceled arrival wait generated work", results)
	}
}
