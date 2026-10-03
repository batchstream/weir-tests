package benchmark

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

func (r *Report) Write(jsonPath, markdownPath string) error {
	if r == nil || jsonPath == "" || markdownPath == "" || filepath.Clean(jsonPath) == filepath.Clean(markdownPath) {
		return fmt.Errorf("report requires distinct JSON and Markdown paths")
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

func writeAtomic(path string, data []byte) error {
	directory := filepath.Dir(path)
	if err := os.MkdirAll(directory, 0755); err != nil {
		return err
	}
	file, err := os.CreateTemp(directory, ".weir-report-*")
	if err != nil {
		return err
	}
	defer os.Remove(file.Name())
	if _, err := file.Write(data); err != nil {
		file.Close()
		return err
	}
	if err := file.Chmod(0644); err != nil {
		file.Close()
		return err
	}
	if err := file.Close(); err != nil {
		return err
	}
	return os.Rename(file.Name(), path)
}

func (r *Report) Markdown() string {
	var text strings.Builder
	fmt.Fprintf(&text, "# Weir throughput comparison\n\nBackend: **%s**. Started: %s.\n\n", r.Parameters.Dataset.Backend, r.Started)
	fmt.Fprintf(&text, "%s.\n\n", r.Method)
	fmt.Fprintf(&text, "Each path executes %d operations (%d reads, %d writes), %d workers, %d pre-created records and %d padding bytes per document. Warmup: %d operations; paired rounds: %d.\n\n", r.Parameters.Operations, r.Parameters.PlannedReads, r.Parameters.PlannedWrites, r.Parameters.Dataset.Concurrency, r.Parameters.Dataset.Records, r.Parameters.Dataset.PayloadBytes, r.Parameters.WarmupOperations, r.Parameters.Rounds)
	text.WriteString("| Round | Order | Path | Success | Errors | Indeterminate | Not attempted | Ops/s | p50 upper ms | p95 upper ms | p99 upper ms | Verified |\n| --- | --- | --- | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | --- |\n")
	for _, pair := range r.Pairs {
		for _, result := range []Result{pair.Direct, pair.Weir} {
			fmt.Fprintf(&text, "| %d | %s | %s | %d | %d | %d | %d | %.1f | %.3f | %.3f | %.3f | %t |\n", pair.Round, strings.Join(pair.Order, " / "), result.Path, result.Succeeded, result.Errors, result.Indeterminate, result.NotAttempted, result.OperationsPerSec, float64(result.Latency.P50NS)/1e6, float64(result.Latency.P95NS)/1e6, float64(result.Latency.P99NS)/1e6, result.Verified)
		}
	}
	if r.Successful() {
		fmt.Fprintf(&text, "\nPooled throughput across all %d verified pairs: direct **%.1f ops/s**, Weir **%.1f ops/s**; Weir/direct **%.4f**, Weir throughput change **%+.2f%%**.\n", r.Aggregate.QualifiedPairs, r.Aggregate.DirectOpsPerSec, r.Aggregate.WeirOpsPerSec, *r.Aggregate.WeirDirectRatio, *r.Aggregate.WeirDeltaPercent)
	} else {
		text.WriteString("\n**No aggregate comparison:** at least one pair is incomplete, failed, indeterminate, or unverified.\n")
	}
	text.WriteString("\nLatency includes every attempted operation and payload validation. Quantiles are approximate inclusive upper bounds from a fixed logarithmic histogram. Counts and elapsed wall time include all worker joins. Logical document byte counts exclude wire framing, metadata and mutation acknowledgements; they are not network bandwidth measurements.\n\n")
	text.WriteString("The JSON report contains protocol, backend version, write/retry policy, byte counts, bounded error examples, and supplied fixture provenance. Results characterize the recorded workload and environment; they do not establish a general Weir overhead figure.\n")
	if r.Incomplete != "" {
		fmt.Fprintf(&text, "\nIncomplete: %s\n", strings.ReplaceAll(r.Incomplete, "\n", " "))
	}
	return text.String()
}
