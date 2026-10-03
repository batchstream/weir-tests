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
		"weir_store_rejections_total": 0, "weir_store_backpressure_events_total": 0,
	}
	for name, value := range values {
		if measured {
			value *= 2
		}
		fmt.Fprintf(&raw, "%s{store=\"mongo\"} %g\n%s{store=\"search\"} 999999\n", name, value, name)
	}
	rpc := 10
	if measured {
		rpc += 100
	}
	fmt.Fprintf(&raw, "weir_rpc_completions_total{method=\"read\",status=\"ok\"} %d\n", rpc)
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
	if metrics.Unavailable != "" || metrics.AdapterBatchAverage == nil || *metrics.AdapterBatchAverage != 32 || metrics.Deltas["weir_store_executions_total"] != 100 || metrics.Deltas["weir_rpc_completions_total:read"] != 100 {
		t.Fatalf("measured batch did not exclude warmup/other store: %+v", metrics)
	}
	if metrics.QueueWaitMeanSeconds == nil || *metrics.QueueWaitMeanSeconds != .001 || metrics.ExecutionMeanSeconds == nil || *metrics.ExecutionMeanSeconds != .005 {
		t.Fatal("queue/adapter means lost", metrics)
	}
	if metrics.RPCUnavailable["mutate"] == "" {
		t.Fatal("missing optional RPC observation silently disappeared")
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
}
