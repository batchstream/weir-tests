package benchmark

import (
	"fmt"
	"strings"
	"testing"

	"github.com/batchstream/weir-tests/internal/observe"
)

func metricsSnapshot(t *testing.T, measured bool) observe.Snapshot {
	t.Helper()
	var raw strings.Builder
	values := map[string]float64{
		"weir_store_batch_operations_sum": 3200, "weir_store_batch_operations_count": 100,
		"weir_store_executions_total": 100, "weir_store_records_total": 3200,
		"weir_store_queue_wait_seconds_sum": .1, "weir_store_queue_wait_seconds_count": 100,
		"weir_store_execution_seconds_sum": .5, "weir_store_execution_seconds_count": 100,
		"weir_store_rejections_total": 0,
	}
	for name, value := range values {
		if measured {
			value *= 2
		}
		fmt.Fprintf(&raw, "%s{store=\"mongo\"} %g\n%s{store=\"search\"} 999999\n", name, value, name)
	}
	raw.WriteString("weir_store_concurrency_limit{store=\"mongo\"} 32\nweir_store_concurrency_limit{store=\"search\"} 16\n")
	rpc := 10
	if measured {
		rpc += 100
	}
	fmt.Fprintf(&raw, "weir_rpc_completions_total{method=\"execute\",status=\"ok\"} %d\n", rpc)
	metrics, err := observe.Parse(raw.String())
	if err != nil {
		t.Fatal(err)
	}
	snapshot := observe.Snapshot{Raw: raw.String(), Metrics: metrics}
	return snapshot
}

func TestServerMetricsUseMeasuredDeltaAndReportMissingObservations(t *testing.T) {
	before := metricsSnapshot(t, false)
	after := metricsSnapshot(t, true)
	metrics := serverMetricDelta(before, after, "mongo")
	if metrics.Unavailable != "" || metrics.AdapterBatchAverage == nil || *metrics.AdapterBatchAverage != 32 || metrics.Deltas["weir_store_executions_total"] != 100 || metrics.Deltas["weir_rpc_completions_total:execute"] != 100 {
		t.Fatalf("measured batch did not exclude warmup/other store: %+v", metrics)
	}
	if metrics.QueueWaitMeanSeconds == nil || *metrics.QueueWaitMeanSeconds != .001 || metrics.ExecutionMeanSeconds == nil || *metrics.ExecutionMeanSeconds != .005 {
		t.Fatal("queue/adapter means lost", metrics)
	}
	if metrics.ConcurrencyLimit == nil || *metrics.ConcurrencyLimit != 32 {
		t.Fatal("configured limit became a delta or came from another store", metrics)
	}
	if len(metrics.RPCUnavailable) != 0 {
		t.Fatal("observed stream RPC unexpectedly unavailable")
	}
	pair := SaturationPair{BatchSize: 1, Concurrency: 32, Round: 1}
	pair.Weir.ServerMetrics = metrics
	report := &SaturationReport{Pairs: []SaturationPair{pair}}
	markdown := report.Markdown()
	if !strings.Contains(markdown, "Execute RPCs") || !strings.Contains(markdown, "| 1 | 32 | 1 | 100 | 32.000 | 100 |") || strings.Contains(markdown, "Read/Mutate RPC counts") {
		t.Fatal("human-readable report lost observed Execute counts", markdown)
	}
	after.Error = "scrape failed"
	metrics = serverMetricDelta(before, after, "mongo")
	if metrics.Unavailable == "" || metrics.AdapterBatchAverage != nil {
		t.Fatal("failed monitoring became valid average", metrics)
	}
	after = before
	after.Metrics = nil
	metrics = serverMetricDelta(before, after, "mongo")
	if metrics.Unavailable == "" || metrics.AdapterBatchAverage != nil {
		t.Fatal("missing metrics became zero batching", metrics)
	}
	after = metricsSnapshot(t, true)
	for index := range after.Metrics {
		metric := &after.Metrics[index]
		if metric.Name == "weir_store_concurrency_limit" && metric.Labels["store"] == "mongo" {
			metric.Value = 16
		}
	}
	metrics = serverMetricDelta(before, after, "mongo")
	if metrics.Unavailable == "" || metrics.ConcurrencyLimit != nil || metrics.AdapterBatchAverage != nil {
		t.Fatal("changing concurrency configuration produced comparable evidence", metrics)
	}
}

func TestSingleRequestRPCProofRequiresActiveMethodCounts(t *testing.T) {
	before, after := metricsSnapshot(t, false), metricsSnapshot(t, true)
	metrics := serverMetricDelta(before, after, "mongo")
	metrics.qualifySingleRequests(100, 0)
	if metrics.RPCsPerAdapter == nil || *metrics.RPCsPerAdapter != 1 {
		t.Fatal("read-only stream aggregation evidence lost")
	}
	metrics.qualifySingleRequests(100, 1)
	if metrics.RPCsPerAdapter != nil {
		t.Fatal("mismatched total stream count became valid")
	}
	metrics.qualifySingleRequests(99, 0)
	if metrics.RPCsPerAdapter != nil || metrics.RPCUnavailable["execute"] == "" {
		t.Fatal("extra timed stream completions were treated as single-request proof")
	}
}
