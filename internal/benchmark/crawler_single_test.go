package benchmark

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	weir "github.com/batchstream/weir-go"
)

func TestCrawlerSingleNativeHasAtomicEndpointAndConcurrentSellerFanout(t *testing.T) {
	var phase, requests, sellers atomic.Int32
	allSellers := make(chan struct{})
	handler := http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		requests.Add(1)
		parts := strings.Split(strings.Trim(request.URL.Path, "/"), "/")
		if len(parts) != 3 || parts[1] != "_update" || request.Method != http.MethodPost {
			t.Error("single input used a bulk or unexpected endpoint", request.URL)
			http.Error(writer, "wrong endpoint", 400)
			return
		}
		if request.URL.Query().Get("retry_on_conflict") != "4" || request.URL.Query().Get("refresh") != "wait_for" || request.URL.Query().Get("wait_for_active_shards") != "1" {
			t.Error("native durability/visibility policy changed", request.URL)
		}
		var payload struct {
			Script struct {
				Params crawlerMutation `json:"params"`
			} `json:"script"`
		}
		if err := json.NewDecoder(request.Body).Decode(&payload); err != nil {
			t.Error(err)
			return
		}
		mutation := payload.Script.Params
		if parts[2] != mutation.ID {
			t.Error("request ID differs from its single input", parts[2], mutation.ID)
		}
		if mutation.Product {
			if mutation.ReconcileOnly {
				if !phase.CompareAndSwap(0, 1) {
					t.Error("reconciliation was not first")
				}
			} else if !phase.CompareAndSwap(1, 2) {
				t.Error("product started before reconciliation acknowledged")
			}
		} else {
			if phase.Load() != 2 {
				t.Error("seller started before product acknowledged")
			}
			if sellers.Add(1) == 3 {
				close(allSellers)
			}
			select {
			case <-allSellers:
			case <-request.Context().Done():
				return
			}
		}
		successful := 1
		result := "updated"
		if mutation.Base {
			successful = 0
			result = "noop"
		}
		shards := map[string]any{"successful": successful, "failed": 0}
		reply := map[string]any{"_id": mutation.ID, "_index": parts[0], "result": result, "_shards": shards}
		_ = json.NewEncoder(writer).Encode(reply)
	})
	server := httptest.NewServer(handler)
	defer server.Close()
	config := crawlerClientConfig{InputShape: "single", Path: "direct", Profile: "two-step", SearchURL: server.URL, Namespace: "fixture", Threads: 1, PageSize: 1}
	client, err := newCrawlerClient(t.Context(), config)
	if err != nil {
		t.Fatal(err)
	}
	defer client.close()
	transport := client.http.Transport.(*http.Transport)
	if transport.MaxConnsPerHost != 3 || transport.MaxIdleConnsPerHost != 3 {
		t.Fatal("native pool cannot support all same-product sellers", transport.MaxConnsPerHost)
	}
	ctx, cancel := context.WithTimeout(t.Context(), 2*time.Second)
	defer cancel()
	mutations := crawlerPage(0, 0, 1)
	var ledger crawlerApplicationLedger
	unknown, err := client.execute(ctx, mutations, &ledger)
	if err != nil || unknown {
		t.Fatal("single-record causal/fanout execution failed", unknown, err)
	}
	counts := ledger.snapshot()
	if requests.Load() != 5 || sellers.Load() != 3 || counts.Requests != 5 || counts.Records != 5 || counts.MinimumRecords != 1 || counts.MaximumRecords != 1 || counts.RecordsPerRequest[1] != 5 {
		t.Fatal("single-record call cardinality or actual ledger differs", requests.Load(), counts)
	}
}

func TestCrawlerSingleMissingAcknowledgementStopsCausalCallsWithoutReplay(t *testing.T) {
	for _, body := range []string{`{}`, `{"_id":"wrong","_index":"fixture-products","result":"updated","_shards":{"successful":1,"failed":0}}`} {
		t.Run(body, func(t *testing.T) {
			var requests atomic.Int32
			handler := http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
				requests.Add(1)
				_, _ = writer.Write([]byte(body))
			})
			server := httptest.NewServer(handler)
			defer server.Close()
			config := crawlerClientConfig{InputShape: "single", Path: "direct", Profile: "two-step", SearchURL: server.URL, Namespace: "fixture", Threads: 1, PageSize: 1}
			client, err := newCrawlerClient(t.Context(), config)
			if err != nil {
				t.Fatal(err)
			}
			defer client.close()
			var ledger crawlerApplicationLedger
			mutations := crawlerPage(0, 0, 1)
			unknown, err := client.execute(t.Context(), mutations, &ledger)
			counts := ledger.snapshot()
			if err == nil || !unknown || requests.Load() != 1 || counts.Requests != 1 || counts.Records != 1 {
				t.Fatal("an uncertain acknowledgement replayed or invented later calls", unknown, err, requests.Load(), counts)
			}
		})
	}
}

func TestCrawlerSingleSDKConstructsExactlyOneRecordAndRejectsBatches(t *testing.T) {
	config := crawlerClientConfig{InputShape: "single", Namespace: "fixture", PageSize: 1}
	mutation := crawlerPage(0, 1, 1)[0]
	one := []crawlerMutation{mutation}
	requests, err := crawlerWeirRequests(config, one)
	if err != nil || len(requests) != 1 {
		t.Fatal("single SDK call was not one record", requests, err)
	}
	request := requests[0]
	if request.Options == nil || request.Options.Visibility != weir.MutationQueryVisible || request.Action != weir.MutationAtomicTransform || request.Lua == nil || request.Lua.Input == nil {
		t.Fatal("single SDK call lost Lua or completion semantics", request)
	}
	var actual crawlerMutation
	if err := json.Unmarshal(request.Lua.Input.Data, &actual); err != nil || !reflect.DeepEqual(actual, mutation) {
		t.Fatal("SDK record content changed", actual, mutation, err)
	}
	batch := []crawlerMutation{mutation, mutation}
	if _, err := crawlerWeirRequests(config, batch); err == nil {
		t.Fatal("single SDK path silently accepted client aggregation")
	}
}

func TestCrawlerApplicationLedgerAuditsActualConcurrentAttempts(t *testing.T) {
	var ledger crawlerApplicationLedger
	done := make(chan struct{}, 3)
	for range 3 {
		go func() {
			for range 100 {
				ledger.record(1)
			}
			done <- struct{}{}
		}()
	}
	for range 3 {
		<-done
	}
	counts := ledger.snapshot()
	if err := auditCrawlerApplicationCounts(counts, "single"); err != nil || counts.Requests != 300 || counts.Records != 300 {
		t.Fatal("concurrent requests lost ledger entries", counts, err)
	}
	var total crawlerApplicationCounts
	mergeCrawlerApplicationCounts(&total, counts)
	mergeCrawlerApplicationCounts(&total, counts)
	if total.Requests != 600 || total.RecordsPerRequest[1] != 600 {
		t.Fatal("child/stage aggregation changed counts", total)
	}
	counts.RecordsPerRequest[2] = 1
	if err := auditCrawlerApplicationCounts(counts, "single"); err == nil {
		t.Fatal("audit accepted hidden client aggregation")
	}
	counts = ledger.snapshot()
	counts.Records++
	if err := auditCrawlerApplicationCounts(counts, "single"); err == nil {
		t.Fatal("audit accepted fabricated submitted records")
	}
}
