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
	fmt.Fprintf(&text, "# Database saturation comparison\n\nBackend **%s**. Started %s.\n\n%s.\n\nWarmup %s and measurement %s per path/stage, %d paired rounds, requested writes %d%%. Latency is the end-to-end batch completion latency, assigned to every logical operation in the batch.\n\n", r.Parameters.Dataset.Backend, r.Started, r.Method, r.Parameters.Warmup, r.Parameters.Duration, r.Parameters.Rounds, r.Parameters.WritePercent)
	text.WriteString("| Batch | Workers | Round | Path | Read | Write | Success | Errors/unknown | Ops/s | p95 batch ms | DB CPU budget % | Full CPU time % | Sampling coverage % | Verified |\n| ---: | ---: | ---: | --- | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | --- |\n")
	for _, pair := range r.Pairs {
		for _, result := range []SaturationResult{pair.Direct, pair.Weir} {
			fmt.Fprintf(&text, "| %d | %d | %d | %s | %d | %d | %d | %d | %.1f | %.3f | %.1f | %.1f | %.1f | %t |\n", pair.BatchSize, pair.Concurrency, pair.Round, result.Path, result.Reads, result.Writes, result.Succeeded, result.Errors+result.Indeterminate, result.OperationsPerSec, float64(result.Latency.P95NS)/1e6, result.Resources.MeanCPUPercent, result.Resources.FullCPUFraction*100, result.Resources.Coverage*100, result.Verified)
		}
	}
	text.WriteString("\nMeasured Weir adapter and unary RPC evidence (warmup, reset and postflight excluded):\n\n| Batch | Workers | Round | Adapter invocations | Mean adapter batch | Read/Mutate RPCs | Queue mean ms | Adapter mean ms | Rejections | Backend concurrency limit |\n| ---: | ---: | ---: | ---: | ---: | --- | ---: | ---: | ---: | ---: |\n")
	var metricWarnings []string
	for _, pair := range r.Pairs {
		metrics := pair.Weir.ServerMetrics
		if metrics == nil {
			fmt.Fprintf(&text, "| %d | %d | %d | unavailable | unavailable | unavailable | unavailable | unavailable | unavailable | unavailable |\n", pair.BatchSize, pair.Concurrency, pair.Round)
			continue
		}
		fmt.Fprintf(&text, "| %d | %d | %d | %s | %s | %s/%s | %s | %s | %s | %s |\n", pair.BatchSize, pair.Concurrency, pair.Round,
			metricCounter(metrics, "weir_store_executions_total"), metricMean(metrics.AdapterBatchAverage, 1), metricCounter(metrics, "weir_rpc_completions_total:read"), metricCounter(metrics, "weir_rpc_completions_total:mutate"),
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
