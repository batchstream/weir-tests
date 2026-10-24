package benchmark

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	weir "github.com/batchstream/weir-go"
	"github.com/batchstream/weir-tests/internal/observe"
)

func TestPartialUpdateNativeAndWeirUseIdenticalSingleDocWithoutLua(t *testing.T) {
	mutation := partialUpdateMutation(7, 4)
	var calls atomic.Int32
	handler := http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		calls.Add(1)
		if request.Method != http.MethodPost || request.URL.Path != "/fixture-products/_update/"+mutation.ID || request.URL.Query().Get("retry_on_conflict") != "0" || request.URL.Query().Get("refresh") != "wait_for" || request.URL.Query().Get("wait_for_active_shards") != "1" {
			t.Error("partial control changed endpoint, retries, visibility, or durability", request.URL)
		}
		var payload map[string]any
		if err := json.NewDecoder(request.Body).Decode(&payload); err != nil || len(payload) != 1 || !reflect.DeepEqual(payload["doc"], mutation.Document) {
			t.Error("native partial document differs", payload, err)
		}
		shards := map[string]any{"successful": 1, "failed": 0}
		reply := map[string]any{"_id": mutation.ID, "_index": "fixture-products", "result": "updated", "_shards": shards}
		_ = json.NewEncoder(writer).Encode(reply)
	})
	server := httptest.NewServer(handler)
	defer server.Close()
	config := crawlerClientConfig{InputShape: "single", Path: "direct", Profile: "partial-update", SearchURL: server.URL, Namespace: "fixture", Threads: 1, PageSize: 1}
	client, err := newCrawlerClient(t.Context(), config)
	if err != nil {
		t.Fatal(err)
	}
	defer client.close()
	var ledger crawlerApplicationLedger
	mutations := []crawlerMutation{mutation}
	unknown, err := client.execute(t.Context(), mutations, &ledger)
	counts := ledger.snapshot()
	if err != nil || unknown || calls.Load() != 1 || counts.Requests != 1 || counts.Records != 1 || counts.MaximumRecords != 1 {
		t.Fatal("mechanism generated causal/seller calls or a native batch", unknown, err, calls.Load(), counts)
	}
	requests, err := crawlerWeirRequests(config, mutations)
	if err != nil || len(requests) != 1 {
		t.Fatal(requests, err)
	}
	request := requests[0]
	if request.Lua != nil || request.BackendExpression == nil || request.BackendExpression.ContentType != "application/vnd.weir.search-update.v1+json" || request.Action != weir.MutationAtomicTransform || request.Options.Visibility != weir.MutationQueryVisible {
		t.Fatal("mechanism included Lua or changed completion semantics", request)
	}
	var payload map[string]any
	if err := json.Unmarshal(request.BackendExpression.Data, &payload); err != nil || len(payload) != 1 || !reflect.DeepEqual(payload["doc"], mutation.Document) {
		t.Fatal("Weir partial document differs", payload, err)
	}
}

func TestPartialUpdateChangesEveryRepeatedKeyAndPreservesOtherFields(t *testing.T) {
	seen := map[string]int{}
	for worker := range 4 {
		current := map[string]map[string]any{}
		for page := range 8 {
			mutation := partialUpdateMutation(worker, page)
			if old, ok := seen[mutation.ID]; ok && old != worker {
				t.Fatal("workers share a mutation key", mutation.ID)
			}
			seen[mutation.ID] = worker
			before := current[mutation.ID]
			if before == nil {
				before = crawlerDocument(mutation.ID, true)
			}
			after := partialUpdateMerge(before, mutation)
			if reflect.DeepEqual(before, after) || after["batch_sequence"] != float64(page+1) || !reflect.DeepEqual(after["stocks"], before["stocks"]) || !reflect.DeepEqual(after["offers"], before["offers"]) || after["last_found_at"] != before["last_found_at"] {
				t.Fatal("mechanism became a noop or changed unrelated fields")
			}
			current[mutation.ID] = after
		}
	}
	if len(seen) != 8 {
		t.Fatal("mechanism did not use two disjoint keys per worker")
	}
}

func TestPartialUpdateRejectsPageAndMixedProfilesBeforeFixtures(t *testing.T) {
	for _, invalid := range []string{"page", "mixed", "negative-wait", "large-wait", "large-adapter-cap"} {
		opts := CrawlerOptions{InputShape: "single", Profiles: []string{"partial-update"}, Processes: 1, PageSize: 1, QueuePages: 1, Rounds: 1, Concurrency: []int{1}, Rates: []int{1}, Warmup: time.Second, Duration: time.Second, Timeout: time.Second}
		switch invalid {
		case "page":
			opts.InputShape = "page"
		case "mixed":
			opts.Profiles = []string{"partial-update", "two-step"}
		case "large-adapter-cap":
			opts.AfterMaxInFlight = 4097
		case "negative-wait":
			opts.AfterBatchMaxWait = -time.Millisecond
		case "large-wait":
			opts.AfterBatchMaxWait = time.Second + time.Nanosecond
		}
		_, err := RunCrawler(t.Context(), opts)
		if err == nil || strings.Contains(err.Error(), "no such file") {
			t.Fatal("invalid mechanism options reached binary/fixture access", invalid, err)
		}
	}
}

func TestPartialUpdateAuditReconstructsAllSeededFieldsAndWait(t *testing.T) {
	digest, afterDigest := strings.Repeat("2", 64), strings.Repeat("4", 64)
	before := crawlerServer{SHA256: digest}
	after := crawlerServer{SHA256: afterDigest}
	servers := map[string]crawlerServer{"before": before, "after": after}
	opts := CrawlerOptions{InputShape: "single", HTTPObserver: "disabled", Profiles: []string{"partial-update"}, Modes: []string{"fixed-rate"}, Paths: []string{"direct", "before", "after"}, Concurrency: []int{1}, Rates: []int{1}, Processes: 1, PageSize: 1, QueuePages: 1, Rounds: 1, CPUs: 1, AfterBatchMaxWait: 20 * time.Millisecond, Visibility: "application_acknowledged", SeedPaddingBytes: 8192, AfterBatchMaxOperations: 64, AfterBatchMaxBytes: 65536, AfterMaxInFlight: 4}
	opts.Duration = time.Second
	expected := map[string]map[string]any{"shared-seller": crawlerDocument("shared-seller", false)}
	for record := range 2 {
		for _, suffix := range []string{"product", "seller-0", "seller-1"} {
			id := crawlerID(0, record, suffix)
			expected[id] = crawlerSeedDocument(id, suffix == "product", opts.SeedPaddingBytes)
		}
	}
	mutation := partialUpdateMutation(0, 2)
	expected[mutation.ID] = partialUpdateMerge(expected[mutation.ID], mutation)
	expectedRaw, _ := json.Marshal(expected)
	sum := sha256.Sum256(expectedRaw)
	var latency histogram
	latency.add(time.Millisecond)
	counts := crawlerApplicationCounts{Requests: 1, Records: 1, MinimumRecords: 1, MaximumRecords: 1, RecordsPerRequest: map[int]uint64{1: 1}}
	settled := crawlerSettling{Settled: true, RequiredQuietNS: int64(2 * time.Second)}
	quota := &DatabaseCPUQuota{RequestedCPUs: 1, ObservedCPUs: 1}
	report := &CrawlerReport{Schema: crawlerReportSchema, Parameters: opts, Servers: servers, Sources: partialUpdateSources()}
	for index, path := range []string{"direct", "before", "after"} {
		worker := crawlerWorkerResult{PID: index + 100, Worker: 0, Offered: 1, Completed: 1, WithinWindow: 1, AppliedPages: []int{2}, RequestedMutations: 1, ApplicationRequests: counts, Latency: latency.summary(), ServiceLatency: latency.summary(), Buckets: latency.buckets, ServiceBuckets: latency.buckets}
		worker.TimeBuckets = newTimeBuckets(int64(opts.Duration), true)
		worker.TimeBuckets[0].Offered = 1
		worker.TimeBuckets[0].Completed = 1
		worker.TimeBuckets[0].Latency = latency.summary()
		worker.TimeBuckets[0].Buckets = latency.buckets
		finishTimeBuckets(worker.TimeBuckets)
		pool := 0
		if path == "direct" {
			pool = 3
		}
		reply := crawlerReply{Visibility: opts.Visibility, SeedPaddingBytes: opts.SeedPaddingBytes, Phase: "timed", PID: worker.PID, Workers: []crawlerWorkerResult{worker}, InputShape: "single", NativePoolLimit: pool, ApplicationRequests: counts}
		stage := crawlerStage{Profile: "partial-update", InputShape: "single", Path: path, Mode: "fixed-rate", Round: 1, Concurrency: 1, ProductsPerSecond: 1, Offered: 1, Completed: 1, WithinWindow: 1, ElapsedNS: int64(time.Second), CompletedProductsPerSecond: 1, ServerSHA256: digest, CPUQuota: quota, BeforeWarmupSettling: settled, BeforeMeasurementSettling: settled, Verified: true, ClientsExited: true, Clients: []crawlerReply{reply}, VerifiedDocuments: len(expected), ExpectedDocumentsSHA256: hex.EncodeToString(sum[:]), ApplicationRequests: counts, RequestedMutations: 1, ScheduledToVisible: latency.summary(), DispatchToVisible: latency.summary()}
		stage.ServerConfiguration = crawlerTestConfiguration(t, opts, path)
		if path == "after" {
			stage.BatchMaxWaitNS = int64(opts.AfterBatchMaxWait)
			stage.ServerSHA256 = afterDigest
		}
		inFlight := 0
		if path == "after" {
			inFlight = opts.AfterMaxInFlight
		}
		capMetric := observe.Metric{Name: "weir_store_active_executions_limit", Labels: map[string]string{"store": "search"}, Value: liveExecutionLimit(inFlight)}
		snapshot := observe.Snapshot{Metrics: []observe.Metric{capMetric}}
		stage.ServerBefore, stage.ServerAfter = snapshot, snapshot
		report.Stages = append(report.Stages, stage)
	}
	if err := AuditCrawlerReport(report); err != nil {
		t.Fatal("independent mechanism oracle rejected valid complete fields", err)
	}
	report.Stages[2].BatchMaxWaitNS = 0
	if err := AuditCrawlerReport(report); err == nil {
		t.Fatal("audit accepted an after path with the wrong collection window")
	}
	report.Stages[2].BatchMaxWaitNS = int64(opts.AfterBatchMaxWait)
	for _, field := range []string{"InputShape", "HTTPObserver", "Visibility", "Paths", "Modes"} {
		missing := *report
		missing.Parameters = opts
		switch field {
		case "InputShape":
			missing.Parameters.InputShape = ""
		case "HTTPObserver":
			missing.Parameters.HTTPObserver = ""
		case "Visibility":
			missing.Parameters.Visibility = ""
		case "Paths":
			missing.Parameters.Paths = nil
		case "Modes":
			missing.Parameters.Modes = nil
		}
		if err := AuditCrawlerReport(&missing); err == nil {
			t.Fatal("audit filled a missing report parameter", field)
		}
	}
	report.Stages[0].ExpectedDocumentsSHA256 = strings.Repeat("0", 64)
	if err := AuditCrawlerReport(report); err == nil {
		t.Fatal("audit accepted incorrect untouched/updated fields")
	}
}
