package benchmark

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func crawlerSettleReply(indexing, merging, flush uint64) map[string]any {
	indices := map[string]any{}
	for _, product := range []bool{true, false} {
		index := map[string]any{"index_current": indexing}
		merge := map[string]any{"current": merging, "total": 3, "total_time_in_millis": 5, "total_docs": 20, "total_size_in_bytes": 100}
		flushing := map[string]any{"total": flush, "total_time_in_millis": 2}
		primaries := map[string]any{"indexing": index, "merges": merge, "flush": flushing}
		stats := map[string]any{"primaries": primaries}
		indices[crawlerIndex("fixture", product)] = stats
	}
	shards := map[string]any{"failed": 0}
	result := map[string]any{"_shards": shards, "indices": indices}
	return result
}

func TestCrawlerSettlingRestartsQuietWindowAfterBusyAndFlushChange(t *testing.T) {
	var requests atomic.Int64
	handler := http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		sequence := requests.Add(1)
		var indexing, merging, flush uint64
		if sequence == 1 {
			indexing = 1
		}
		if sequence == 2 {
			merging = 1
		}
		if sequence >= 4 {
			flush = 1
		}
		reply := crawlerSettleReply(indexing, merging, flush)
		_ = json.NewEncoder(writer).Encode(reply)
	})
	server := httptest.NewServer(handler)
	defer server.Close()
	client := server.Client()
	opts := crawlerSettleOptions{URL: server.URL, Namespace: "fixture", Timeout: time.Second, Quiet: 20 * time.Millisecond, Poll: 5 * time.Millisecond}
	result, err := waitCrawlerSettled(t.Context(), client, opts)
	if err != nil || !result.Settled || len(result.Samples) < 5 {
		t.Fatal("settling ignored busy work or unstable cumulative flush counters", result, err)
	}
	changed := result.Samples[3].ElapsedNS
	last := result.Samples[len(result.Samples)-1].ElapsedNS
	if last-changed < int64(opts.Quiet) || result.WaitNS < last {
		t.Fatal("quiet interval/wait records do not cover the final stable counters", result)
	}
	for _, sample := range result.Samples {
		if sample.At == "" || len(sample.Stats) == 0 || sample.Error != "" {
			t.Fatal("raw timestamped settling sample missing", sample)
		}
	}
}

func TestCrawlerSettlingHonorsDeadlineAndRetainsBusyEvidence(t *testing.T) {
	handler := http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		reply := crawlerSettleReply(0, 1, 0)
		_ = json.NewEncoder(writer).Encode(reply)
	})
	server := httptest.NewServer(handler)
	defer server.Close()
	client := server.Client()
	opts := crawlerSettleOptions{URL: server.URL, Namespace: "fixture", Timeout: 30 * time.Millisecond, Quiet: 20 * time.Millisecond, Poll: 5 * time.Millisecond}
	result, err := waitCrawlerSettled(t.Context(), client, opts)
	if !errors.Is(err, context.DeadlineExceeded) || result.Settled || len(result.Samples) < 2 || result.WaitNS < int64(opts.Timeout) {
		t.Fatal("busy settling escaped its deadline or lost samples", result, err)
	}
}

func TestCrawlerSettlingRejectsMissingOwnedIndexCounters(t *testing.T) {
	raw := []byte(`{"_shards":{"failed":0},"indices":{"unowned":{"primaries":{}}}}`)
	_, _, err := crawlerIdleCounters(raw, "fixture")
	if err == nil {
		t.Fatal("partial/unowned index statistics were accepted")
	}
}

func TestCrawlerExecutionFollowsProductThenSellerPhases(t *testing.T) {
	for _, profile := range []string{"combined", "two-step"} {
		t.Run(profile, func(t *testing.T) {
			var calls [][]crawlerMutation
			handler := http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
				var mutations []crawlerMutation
				decoder := json.NewDecoder(request.Body)
				for decoder.More() {
					var metadata json.RawMessage
					if err := decoder.Decode(&metadata); err != nil {
						break
					}
					var payload struct {
						Script struct {
							Params crawlerMutation `json:"params"`
						} `json:"script"`
					}
					if err := decoder.Decode(&payload); err != nil {
						t.Error(err)
						return
					}
					mutations = append(mutations, payload.Script.Params)
				}
				calls = append(calls, mutations)
				items := make([]any, len(mutations))
				for index := range items {
					shards := map[string]any{"successful": 1, "failed": 0}
					update := map[string]any{"status": 200, "result": "updated", "_shards": shards}
					item := map[string]any{"update": update}
					items[index] = item
				}
				reply := map[string]any{"items": items, "errors": false}
				_ = json.NewEncoder(writer).Encode(reply)
			})
			server := httptest.NewServer(handler)
			defer server.Close()
			config := crawlerClientConfig{Path: "direct", Profile: profile, SearchURL: server.URL, Namespace: "fixture", Threads: 1, PageSize: 2}
			client, err := newCrawlerClient(t.Context(), config)
			if err != nil {
				t.Fatal(err)
			}
			defer client.close()
			mutations := crawlerPage(0, 1, 2)
			var ledger crawlerApplicationLedger
			unknown, err := client.execute(t.Context(), mutations, &ledger)
			if err != nil || unknown {
				t.Fatal(err, unknown)
			}
			want := 2
			if profile == "two-step" {
				want = 3
			}
			if len(calls) != want {
				t.Fatal("page phase count differs from the crawler rules", len(calls), want)
			}
			for phase, records := range calls {
				for _, mutation := range records {
					if phase == len(calls)-1 {
						if mutation.Product || !strings.Contains(mutation.ID, "seller") {
							t.Fatal("seller phase contains a product", mutation)
						}
					} else if !mutation.Product {
						t.Fatal("product phase contains a seller", mutation)
					}
					if profile == "two-step" && phase == 0 && !mutation.ReconcileOnly {
						t.Fatal("two-step profile omitted the separate reconciliation")
					}
				}
			}
		})
	}
}
