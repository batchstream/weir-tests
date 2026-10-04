package benchmark

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/batchstream/weir-tests/internal/observe"
)

type ServerMetrics struct {
	BatchBuckets         map[string]float64 `json:"timed_adapter_batch_cumulative_buckets,omitempty"`
	RPCsPerAdapter       *float64           `json:"timed_business_rpcs_per_adapter_invocation,omitempty"`
	Before               observe.Snapshot   `json:"before"`
	After                observe.Snapshot   `json:"after"`
	Deltas               map[string]float64 `json:"timed_counter_deltas,omitempty"`
	AdapterBatchAverage  *float64           `json:"timed_adapter_batch_average,omitempty"`
	ConcurrencyLimit     *float64           `json:"configured_backend_concurrency_limit,omitempty"`
	QueueWaitMeanSeconds *float64           `json:"timed_queue_wait_mean_seconds,omitempty"`
	ExecutionMeanSeconds *float64           `json:"timed_adapter_execution_mean_seconds,omitempty"`
	RPCUnavailable       map[string]string  `json:"rpc_observation_unavailable,omitempty"`
	Unavailable          string             `json:"unavailable,omitempty"`
	Scope                string             `json:"scope"`
}

func fetchServerMetrics(ctx context.Context, address string) observe.Snapshot {
	observationCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	return observe.Fetch(observationCtx, address)
}

func serverMetricDelta(before, after observe.Snapshot, store string) *ServerMetrics {
	metrics := &ServerMetrics{Before: before, After: after, Deltas: make(map[string]float64), RPCUnavailable: make(map[string]string), Scope: "only the measured Weir stage, after warmup and before independent postflight; adapter invocation batch size, not a claim that every invocation maps to one physical wire command"}
	labels := map[string]string{"store": store}
	var err error
	for _, name := range []string{"weir_store_batch_operations_sum", "weir_store_batch_operations_count", "weir_store_executions_total", "weir_store_records_total", "weir_store_queue_wait_seconds_sum", "weir_store_queue_wait_seconds_count", "weir_store_execution_seconds_sum", "weir_store_execution_seconds_count", "weir_store_rejections_total"} {
		value, deltaErr := observe.Delta(before, after, name, labels)
		if deltaErr != nil {
			err = errors.Join(err, deltaErr)
			continue
		}
		metrics.Deltas[name] = value
	}
	beforeLimit, foundBefore := before.Sum("weir_store_concurrency_limit", labels)
	afterLimit, foundAfter := after.Sum("weir_store_concurrency_limit", labels)
	if !foundBefore || !foundAfter || beforeLimit < 1 || afterLimit != beforeLimit {
		err = errors.Join(err, errors.New("configured backend concurrency limit unavailable or changed during measurement"))
	} else {
		metrics.ConcurrencyLimit = &afterLimit
	}
	for _, method := range []string{"execute"} {
		methodLabels := map[string]string{"method": method}
		value, deltaErr := observe.Delta(before, after, "weir_rpc_completions_total", methodLabels)
		if deltaErr == nil {
			metrics.Deltas["weir_rpc_completions_total:"+method] = value
		} else {
			metrics.RPCUnavailable[method] = deltaErr.Error()
		}
	}
	if err != nil {
		metrics.Unavailable = err.Error()
		return metrics
	}
	if count := metrics.Deltas["weir_store_batch_operations_count"]; count > 0 {
		average := metrics.Deltas["weir_store_batch_operations_sum"] / count
		metrics.AdapterBatchAverage = &average
		if streams, found := metrics.Deltas["weir_rpc_completions_total:execute"]; found {
			ratio := streams / count
			metrics.RPCsPerAdapter = &ratio
		}
	} else {
		metrics.Unavailable = "no adapter invocation was observed during the measured Weir stage"
	}
	metrics.BatchBuckets = make(map[string]float64)
	for _, metric := range after.Metrics {
		if metric.Name != "weir_store_batch_operations_bucket" || metric.Labels["store"] != store {
			continue
		}
		bucketLabels := map[string]string{"store": store, "le": metric.Labels["le"]}
		value, err := observe.Delta(before, after, metric.Name, bucketLabels)
		if err != nil {
			metrics.Unavailable = fmt.Sprintf("%s; %v", metrics.Unavailable, err)
			continue
		}
		metrics.BatchBuckets[metric.Labels["le"]] = value
	}
	if count := metrics.Deltas["weir_store_queue_wait_seconds_count"]; count > 0 {
		average := metrics.Deltas["weir_store_queue_wait_seconds_sum"] / count
		metrics.QueueWaitMeanSeconds = &average
	}
	if count := metrics.Deltas["weir_store_execution_seconds_count"]; count > 0 {
		average := metrics.Deltas["weir_store_execution_seconds_sum"] / count
		metrics.ExecutionMeanSeconds = &average
	}
	return metrics
}

// Single-request aggregation evidence must match every measured business stream.
func (m *ServerMetrics) qualifySingleRequests(reads, writes uint64) {
	m.RPCsPerAdapter = nil
	if m.Unavailable != "" || m.AdapterBatchAverage == nil {
		return
	}
	observed, found := m.Deltas["weir_rpc_completions_total:execute"]
	if !found {
		return
	}
	if observed != float64(reads+writes) {
		m.RPCUnavailable["execute"] = "timed stream completion count differs from measured single-request calls"
		return
	}
	if count := m.Deltas["weir_store_batch_operations_count"]; count > 0 {
		ratio := observed / count
		m.RPCsPerAdapter = &ratio
	}
}
