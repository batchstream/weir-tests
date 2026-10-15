package benchmark

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestCrawlerDomainOraclePreservesHistoriesAndReconcilesOffers(t *testing.T) {
	var current map[string]any
	for sequence := range 8 {
		mutations := crawlerPage(0, sequence*2, 1)
		for _, mutation := range mutations {
			if !mutation.Product {
				continue
			}
			if current == nil {
				current = crawlerDocument(mutation.ID, true)
			}
			current = crawlerMerge(current, mutation)
			if current["first_found_at"] != "2026-01-01T00:00:00Z" || current["last_found_at"] != crawlerObserved || current["brand"] != "EXAMPLE" {
				t.Fatal("observation or normalization changed", current)
			}
			if len(current["stocks"].([]any)) != 20 || len(current["solds"].([]any)) != 20 || len(current["comments"].([]any)) != 20 {
				t.Fatal("history caps changed")
			}
			offers := current["offers"].([]any)
			if len(offers) != 3 || offers[2].(map[string]any)["expiry_detected_at"] != crawlerObserved {
				t.Fatal("reconciliation dropped retained expired offer", offers)
			}
		}
	}
	initial := crawlerDocument("shared-seller", false)
	mutation := crawlerMutation{ID: "shared-seller", Base: true, Document: map[string]any{"name": "placeholder"}, ObservedAt: crawlerObserved}
	actual := crawlerMerge(initial, mutation)
	if !reflect.DeepEqual(initial, actual) {
		t.Fatal("base seller changed")
	}
	actual["name"] = "mutated"
	if initial["name"] == "mutated" {
		t.Fatal("oracle aliased current document")
	}
}

func TestCrawlerOpenLoopKeepsArrivalsIndependentAndCountsRejectedPages(t *testing.T) {
	handler := http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		time.Sleep(30 * time.Millisecond)
		var lines []json.RawMessage
		decoder := json.NewDecoder(request.Body)
		for decoder.More() {
			var line json.RawMessage
			if err := decoder.Decode(&line); err != nil {
				break
			}
			lines = append(lines, line)
		}
		items := make([]any, len(lines)/2)
		for index := range items {
			shards := map[string]any{"successful": 1, "failed": 0}
			update := map[string]any{"status": 200, "result": "updated", "_shards": shards}
			item := map[string]any{"update": update}
			items[index] = item
		}
		reply := map[string]any{"errors": false, "items": items}
		_ = json.NewEncoder(writer).Encode(reply)
	})
	server := httptest.NewServer(handler)
	defer server.Close()
	config := crawlerClientConfig{Path: "direct", SearchURL: server.URL, Namespace: "fixture", Threads: 2, PageSize: 1, Processes: 1, QueuePages: 1, TimeoutNS: int64(time.Second)}
	client, err := newCrawlerClient(t.Context(), config)
	if err != nil {
		t.Fatal(err)
	}
	defer client.close()
	phase := crawlerPhase{Phase: "timed", StartNS: time.Now().Add(10 * time.Millisecond).UnixNano(), DurationNS: int64(150 * time.Millisecond), ProductsPerSecond: 1000}
	started := time.Now()
	results := client.measure(context.Background(), phase)
	var offered, rejected, completed uint64
	for _, result := range results {
		offered += result.Offered
		rejected += result.Rejected
		completed += result.Completed
		if result.Failed != 0 || result.Latency.Samples != result.Completed || result.Latency.P95NS < result.ServiceLatency.P50NS {
			t.Fatal("completion or planned latency invalid", result)
		}
		if result.QueuePeak > 1 {
			t.Fatal("unbounded queue")
		}
	}
	if offered != 150 || rejected == 0 || completed == 0 || offered != rejected+completed || time.Since(started) > time.Second {
		t.Fatal("generator became closed-loop or lost admissions", offered, rejected, completed, time.Since(started))
	}
}

func TestCrawlerSourcesRequireOnlyPlainDocumentReturns(t *testing.T) {
	for name, source := range map[string]string{"common": crawlerCommon, "product": crawlerProduct, "offer": crawlerOffer} {
		if strings.Contains(source, "weir.keep") || strings.Contains(source, "sink.v1") {
			t.Fatal("unexpected legacy interface or action helper", name)
		}
		if len(crawlerSources()[name]) != 64 {
			t.Fatal("missing frozen source digest", name)
		}
	}
	if strings.Contains(crawlerProduct, "return reconcile(current, incoming)") {
		t.Fatal("product still skips the merge after reconciliation")
	}
}

func TestCrawlerDatabaseTransportDoesNotIntroduceCompression(t *testing.T) {
	handler := http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if value := request.Header.Get("Accept-Encoding"); value != "" {
			t.Errorf("database compression was introduced: %s", value)
		}
		_, _ = writer.Write([]byte(`{}`))
	})
	database := httptest.NewServer(handler)
	defer database.Close()
	observer, err := startCrawlerObserver(database.URL)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = observer.close(t.Context()) }()
	config := crawlerClientConfig{Path: "direct", SearchURL: observer.URL, Threads: 1}
	client, err := newCrawlerClient(t.Context(), config)
	if err != nil {
		t.Fatal(err)
	}
	defer client.close()
	request := crawlerHTTPRequest{Method: http.MethodPost, URL: observer.URL + "/_bulk", Body: []byte(`{}`)}
	if _, err := crawlerHTTP(t.Context(), client.http, request); err != nil {
		t.Fatal(err)
	}
}
