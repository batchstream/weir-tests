package benchmark

import (
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
)

func (r *SaturationReport) Write(jsonPath, markdownPath string) error {
	if r == nil || jsonPath == "" || markdownPath == "" || filepath.Clean(jsonPath) == filepath.Clean(markdownPath) {
		return errors.New("report requires distinct JSON and Markdown paths")
	}
	raw, err := json.MarshalIndent(r, "", "  ")
	if err != nil {
		return err
	}
	if err := writeAtomic(jsonPath, append(raw, '\n')); err != nil {
		return err
	}
	return writeAtomic(markdownPath, []byte(r.Markdown()))
}

func (r *SaturationReport) Markdown() string {
	var text strings.Builder
	fmt.Fprintf(&text, "# Database saturation comparison\n\nBackend **%s**. Started %s.\n\n%s.\n\nWarmup %s and measurement %s per path/stage, %d paired rounds, requested writes %d%%.\n\n", r.Parameters.Dataset.Backend, r.Started, r.Method, r.Parameters.Warmup, r.Parameters.Duration, r.Parameters.Rounds, r.Parameters.WritePercent)
	if r.Parameters.OperationTimeout != "" {
		fmt.Fprintf(&text, "Complete business-request budget for both paths: %s.\n\n", r.Parameters.OperationTimeout)
	}
	if timeout := r.Provenance["weir_backend_timeout"]; timeout != "" {
		fmt.Fprintf(&text, "Configured Weir Store backend budget: %s; the active caller deadline also applies.\n\n", timeout)
	}
	if r.Parameters.ClientProcesses > 0 {
		fmt.Fprintf(&text, "%d independent OS client processes. Each worker sends one business request containing one record, waits for its acknowledged result, and validates it. p50/p95/p99 describe individual requests; histograms are pooled rather than averaging process quantiles.\n\n", r.Parameters.ClientProcesses)
	} else {
		text.WriteString("Supplemental bulk-versus-bulk overhead profile. Batch completion latency is assigned to each record; this experiment does not measure cross-RPC aggregation of independent requests.\n\n")
	}
	text.WriteString("| Batch | Workers | Round | Path | Read | Write | Success | Errors/unknown | Ops/s | p95 call ms | DB CPU budget % | Full CPU time % | Sampling coverage % | Verified |\n| ---: | ---: | ---: | --- | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | --- |\n")
	for _, pair := range r.Pairs {
		for _, result := range []SaturationResult{pair.Direct, pair.Weir} {
			fmt.Fprintf(&text, "| %d | %d | %d | %s | %d | %d | %d | %d | %.1f | %.3f | %.1f | %.1f | %.1f | %t |\n", pair.BatchSize, pair.Concurrency, pair.Round, result.Path, result.Reads, result.Writes, result.Succeeded, result.Errors+result.Indeterminate, result.OperationsPerSec, float64(result.Latency.P95NS)/1e6, result.Resources.MeanCPUPercent, result.Resources.FullCPUFraction*100, result.Resources.Coverage*100, result.Verified)
		}
	}
	text.WriteString("\nMeasured Weir adapter and streaming RPC evidence (warmup, reset and postflight excluded):\n\n| Batch | Workers | Round | Adapter invocations | Mean adapter batch | Execute RPCs | Queue mean ms | Adapter mean ms | Rejections | Backend concurrency limit |\n| ---: | ---: | ---: | ---: | ---: | --- | ---: | ---: | ---: | ---: |\n")
	var metricWarnings []string
	for _, pair := range r.Pairs {
		metrics := pair.Weir.ServerMetrics
		if metrics == nil {
			fmt.Fprintf(&text, "| %d | %d | %d | unavailable | unavailable | unavailable | unavailable | unavailable | unavailable | unavailable |\n", pair.BatchSize, pair.Concurrency, pair.Round)
			continue
		}
		fmt.Fprintf(&text, "| %d | %d | %d | %s | %s | %s | %s | %s | %s | %s |\n", pair.BatchSize, pair.Concurrency, pair.Round,
			metricCounter(metrics, "weir_store_executions_total"), metricMean(metrics.AdapterBatchAverage, 1), metricCounter(metrics, "weir_rpc_completions_total:execute"),
			metricMean(metrics.QueueWaitMeanSeconds, 1000), metricMean(metrics.ExecutionMeanSeconds, 1000), metricCounter(metrics, "weir_store_rejections_total"), metricMean(metrics.ConcurrencyLimit, 1))
		if metrics.Unavailable != "" {
			warning := fmt.Sprintf("Metrics unavailable (batch %d, workers %d, round %d): %s.", pair.BatchSize, pair.Concurrency, pair.Round, strings.ReplaceAll(metrics.Unavailable, "\n", " "))
			metricWarnings = append(metricWarnings, warning)
		}
	}
	for _, warning := range metricWarnings {
		fmt.Fprintf(&text, "\n%s\n", warning)
	}
	for _, comparison := range r.Comparisons {
		if comparison.Ratio == nil {
			fmt.Fprintf(&text, "\n**Batch %d: database-full-load throughput comparison unavailable.** %s.\n", comparison.BatchSize, comparison.Unavailable)
		} else {
			fmt.Fprintf(&text, "\n**Batch %d at demonstrated database CPU saturation:** direct %.1f logical ops/s (%d workers), Weir %.1f logical ops/s (%d workers), Weir/direct %.4f, change %+.2f%%.\n", comparison.BatchSize, comparison.Direct.OperationsPerSec, comparison.Direct.Concurrency, comparison.Weir.OperationsPerSec, comparison.Weir.Concurrency, *comparison.Ratio, *comparison.DeltaPercent)
		}
	}
	if len(r.BusinessComparisons) > 0 {
		if r.BusinessPeaks != nil {
			p := r.BusinessPeaks
			fmt.Fprintf(&text, "\nBest verified business QPS within this ladder: direct %.1f (%d workers), Weir %.1f (%d workers), observed change %+.2f%%. This is a measured business-throughput comparison, not a maximum-database-capacity claim.\n", p.Direct.OperationsPerSec, p.Direct.Concurrency, p.Weir.OperationsPerSec, p.Weir.Concurrency, p.DeltaPercent)
		}
		text.WriteString("\nObserved single-request business performance at matched client concurrency (independent of the database-capacity qualification):\n\n| Workers | Direct QPS | Weir QPS | Change | Direct p50/p95/p99 ms | Weir p50/p95/p99 ms |\n| ---: | ---: | ---: | ---: | --- | --- |\n")
		for _, comparison := range r.BusinessComparisons {
			d, w := comparison.DirectLatency, comparison.WeirLatency
			fmt.Fprintf(&text, "| %d | %.1f | %.1f | %+.2f%% | %.3f / %.3f / %.3f | %.3f / %.3f / %.3f |\n", comparison.Concurrency, comparison.DirectOps, comparison.WeirOps, comparison.DeltaPercent, float64(d.P50NS)/1e6, float64(d.P95NS)/1e6, float64(d.P99NS)/1e6, float64(w.P50NS)/1e6, float64(w.P95NS)/1e6, float64(w.P99NS)/1e6)
		}
		text.WriteString("\nThese QPS and latency observations apply to this hardware and measured concurrency range. A business-throughput improvement can exist without demonstrating the maximum database capacity. An underfull database or missing next-level plateau keeps the database-capacity comparison unavailable.\n")
	}
	if r.Parameters.ClientProcesses > 0 {
		text.WriteString("\nAggregation evidence: the JSON records one business request per record, Execute RPC counts, adapter invocation counts, average adapter batch and its cumulative histogram, and RPCs per adapter invocation. An adapter invocation is not necessarily one physical database wire command. MongoDB physical find/update/bulkWrite/getMore/killCursors/commitTransaction/abortTransaction deltas come from owned serverStatus at timed boundaries; missing counters remain unavailable. Elasticsearch has no equivalent physical HTTP-command counter. Every child PID, configured native pool limit, final revision ranges, exit result and sampled CPU/RSS is retained; coordinator sampling CPU is excluded from business-client CPU.\n")
	}
	text.WriteString("\nThe JSON retains before/after raw public Weir metrics and their observed deltas, every resource sample, monitor failures, raw cumulative database/container I/O and network counters, CPU quota/host core denominator, and client/Weir cumulative CPU. Docker CPU uses cumulative Engine usage and read timestamps across each entire sampling interval, never CLI instantaneous percentages. Both interval endpoints must succeed; missing intervals reduce full-run coverage. Samples include monitoring traffic and background database work. Native ps CPU has platform-dependent clock resolution. Native databases share the host CPU budget with the client and Weir. Disk/network utilization or capacity is not inferred from byte counters. A plateau without sustained database CPU saturation is not a database-full-load result. Startup/discovery/reset/postflight and warmup are excluded from throughput; all in-flight completion and validation is included.\n")
	if r.Incomplete != "" {
		fmt.Fprintf(&text, "\nIncomplete: %s\n", strings.ReplaceAll(r.Incomplete, "\n", " "))
	}
	return text.String()
}

func (r *SaturationReport) Verified() bool {
	if r == nil || r.Incomplete != "" || len(r.Pairs) != len(r.Parameters.Concurrency)*len(r.Parameters.BatchSizes)*r.Parameters.Rounds {
		return false
	}
	for _, pair := range r.Pairs {
		if !pair.Direct.complete() || !pair.Weir.complete() || !pair.Direct.Verified || !pair.Weir.Verified {
			return false
		}
	}
	return true
}

func metricCounter(metrics *ServerMetrics, name string) string {
	value, found := metrics.Deltas[name]
	if !found {
		return "unavailable"
	}
	return fmt.Sprintf("%.0f", value)
}

func metricMean(value *float64, scale float64) string {
	if value == nil {
		return "unavailable"
	}
	return fmt.Sprintf("%.3f", *value*scale)
}
