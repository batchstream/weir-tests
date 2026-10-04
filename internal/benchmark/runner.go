// Package benchmark measures matched, finite logical database workloads.
package benchmark

import (
	"context"
	"errors"
	"fmt"
	"math"
	"math/bits"
	"runtime"
	"sync"
	"time"

	"github.com/batchstream/weir-tests/internal/workload"
)

type Options struct {
	Dataset          *workload.Dataset
	Paths            *workload.Paths
	Operations       int
	WarmupOperations int
	WritePercent     int
	Rounds           int
	OperationTimeout time.Duration
	Provenance       map[string]string
}

type Parameters struct {
	Dataset          workload.Config `json:"dataset"`
	Operations       int             `json:"operations_per_path_per_round"`
	WarmupOperations int             `json:"warmup_operations_per_path"`
	WritePercent     int             `json:"requested_write_percent"`
	PlannedReads     int             `json:"actual_planned_reads"`
	PlannedWrites    int             `json:"actual_planned_writes"`
	Rounds           int             `json:"paired_rounds"`
	OperationTimeout string          `json:"operation_timeout"`
}

type Latency struct {
	Samples uint64 `json:"samples"`
	P50NS   int64  `json:"p50_upper_bound_ns"`
	P95NS   int64  `json:"p95_upper_bound_ns"`
	P99NS   int64  `json:"p99_upper_bound_ns"`
	MaxNS   int64  `json:"max_observed_ns"`
	Method  string `json:"method"`
}

type Result struct {
	Timeouts          uint64            `json:"business_operation_timeouts"`
	Path              string            `json:"path"`
	Planned           uint64            `json:"planned"`
	Attempted         uint64            `json:"attempted"`
	Succeeded         uint64            `json:"succeeded"`
	Errors            uint64            `json:"errors"`
	Indeterminate     uint64            `json:"indeterminate"`
	NotAttempted      uint64            `json:"not_attempted"`
	AppliedWithError  uint64            `json:"applied_with_error"`
	ElapsedNS         int64             `json:"elapsed_ns"`
	OperationsPerSec  float64           `json:"successful_operations_per_second"`
	RequestBytes      uint64            `json:"logical_request_document_bytes"`
	ResponseBytes     uint64            `json:"logical_response_document_bytes"`
	Verified          bool              `json:"verified"`
	VerificationError string            `json:"verification_error,omitempty"`
	ErrorExamples     []string          `json:"error_examples,omitempty"`
	Latency           Latency           `json:"latency"`
	Evidence          workload.Evidence `json:"evidence"`
}

type Pair struct {
	Round            int      `json:"round"`
	Order            []string `json:"order"`
	Direct           Result   `json:"direct"`
	Weir             Result   `json:"weir"`
	WeirDirectRatio  *float64 `json:"weir_direct_throughput_ratio,omitempty"`
	WeirDeltaPercent *float64 `json:"weir_throughput_delta_percent,omitempty"`
}

type Aggregate struct {
	QualifiedPairs   int      `json:"qualified_pairs"`
	DirectOpsPerSec  float64  `json:"direct_successful_operations_per_second"`
	WeirOpsPerSec    float64  `json:"weir_successful_operations_per_second"`
	WeirDirectRatio  *float64 `json:"weir_direct_throughput_ratio,omitempty"`
	WeirDeltaPercent *float64 `json:"weir_throughput_delta_percent,omitempty"`
}

type Report struct {
	Schema     int               `json:"schema"`
	Started    string            `json:"started_utc"`
	Parameters Parameters        `json:"parameters"`
	Provenance map[string]string `json:"provenance"`
	Pairs      []Pair            `json:"pairs"`
	Aggregate  Aggregate         `json:"aggregate"`
	Method     string            `json:"method"`
	Incomplete string            `json:"incomplete,omitempty"`
}

func Run(ctx context.Context, options Options) (*Report, error) {
	if options.Dataset == nil || options.Paths == nil || options.Rounds < 1 || options.Rounds > 100 || options.OperationTimeout <= 0 || options.OperationTimeout > time.Minute || options.WarmupOperations < 0 {
		return nil, errors.New("invalid benchmark options")
	}
	planOptions := workload.PlanOptions{Operations: options.Operations, WritePercent: options.WritePercent}
	plan, err := options.Dataset.Plan(planOptions)
	if err != nil {
		return nil, err
	}
	var warmup *workload.Plan
	if options.WarmupOperations > 0 {
		warmupOptions := workload.PlanOptions{Operations: options.WarmupOperations, WritePercent: options.WritePercent}
		warmup, err = options.Dataset.Plan(warmupOptions)
		if err != nil {
			return nil, err
		}
	}
	parameters := Parameters{Dataset: options.Dataset.Config, Operations: plan.Count, PlannedReads: plan.Reads, PlannedWrites: plan.Writes, WarmupOperations: options.WarmupOperations, WritePercent: options.WritePercent, Rounds: options.Rounds, OperationTimeout: options.OperationTimeout.String()}
	provenance := make(map[string]string, len(options.Provenance)+4)
	for key, value := range options.Provenance {
		provenance[key] = value
	}
	provenance["go"] = runtime.Version()
	provenance["client_os_arch"] = runtime.GOOS + "/" + runtime.GOARCH
	provenance["gomaxprocs"] = fmt.Sprint(runtime.GOMAXPROCS(0))
	provenance["client_cpu_count"] = fmt.Sprint(runtime.NumCPU())
	report := &Report{Schema: 1, Started: time.Now().UTC().Format(time.RFC3339Nano), Parameters: parameters, Provenance: provenance, Method: "fixed identical operation plan; partitioned IDs; no client bulk; alternating AB/BA; setup, warmup, reset and independent postflight excluded; elapsed includes worker start barrier and all joins; latency includes payload validation; throughput counts only successful logical operations"}
	for round := range options.Rounds {
		order := pairOrder(round)
		pair := Pair{Round: round + 1, Order: order}
		for _, name := range order {
			if err := options.Paths.Prepare(ctx, 1); err != nil {
				report.Incomplete = "prepare: " + err.Error()
				return report, err
			}
			executor := options.Paths.Direct
			if name == "weir" {
				executor = options.Paths.Weir
			}
			if warmup != nil {
				result := measure(ctx, executor, warmup, options.OperationTimeout)
				if !result.complete() {
					report.Incomplete = fmt.Sprintf("warmup failed for %s: success=%d errors=%d indeterminate=%d unattempted=%d examples=%v", name, result.Succeeded, result.Errors, result.Indeterminate, result.NotAttempted, result.ErrorExamples)
					return report, errors.New(report.Incomplete)
				}
				if err := options.Paths.Verify(ctx, warmup, 1); err != nil {
					report.Incomplete = "warmup verification: " + err.Error()
					return report, err
				}
				if err := options.Paths.Prepare(ctx, 1); err != nil {
					report.Incomplete = "reset after warmup: " + err.Error()
					return report, err
				}
			}
			result := measure(ctx, executor, plan, options.OperationTimeout)
			if err := options.Paths.Verify(ctx, plan, 1); err != nil {
				result.VerificationError = err.Error()
			} else {
				result.Verified = true
			}
			if name == "direct" {
				pair.Direct = result
			} else {
				pair.Weir = result
			}
			if err := ctx.Err(); err != nil {
				report.Incomplete = err.Error()
				report.Pairs = append(report.Pairs, pair)
				report.aggregate()
				return report, err
			}
		}
		pair.qualify()
		report.Pairs = append(report.Pairs, pair)
	}
	report.aggregate()
	return report, nil
}

func pairOrder(round int) []string {
	if round%2 == 0 {
		return []string{"direct", "weir"}
	}
	return []string{"weir", "direct"}
}

type workerResult struct {
	result Result
	hist   histogram
}

func measure(ctx context.Context, executor workload.Executor, plan *workload.Plan, timeout time.Duration) Result {
	workers := make([]workerResult, len(plan.Workers))
	start := make(chan struct{})
	var ready, joined sync.WaitGroup
	ready.Add(len(workers))
	joined.Add(len(workers))
	for index, operations := range plan.Workers {
		go func() {
			defer joined.Done()
			ready.Done()
			<-start
			worker := &workers[index]
			for _, operation := range operations {
				if ctx.Err() != nil {
					break
				}
				operationCtx, cancel := context.WithTimeout(ctx, timeout)
				began := time.Now()
				outcome := executor.Execute(operationCtx, operation)
				worker.hist.add(time.Since(began))
				cancel()
				worker.result.Attempted++
				worker.result.RequestBytes += outcome.RequestBytes
				worker.result.ResponseBytes += outcome.ResponseBytes
				switch outcome.Status {
				case workload.Success:
					worker.result.Succeeded++
				case workload.Indeterminate:
					worker.result.Indeterminate++
				default:
					worker.result.Errors++
				}
				if outcome.Status != workload.Success {
					if outcome.Applied {
						worker.result.AppliedWithError++
					}
					if len(worker.result.ErrorExamples) < 3 {
						worker.result.ErrorExamples = append(worker.result.ErrorExamples, outcome.Error)
					}
				}
			}
		}()
	}
	ready.Wait()
	started := time.Now()
	close(start)
	joined.Wait()
	elapsed := time.Since(started)
	result := Result{Path: executor.Name(), Planned: uint64(plan.Count), ElapsedNS: elapsed.Nanoseconds(), Evidence: executor.Evidence()}
	var hist histogram
	for _, worker := range workers {
		result.Attempted += worker.result.Attempted
		result.Succeeded += worker.result.Succeeded
		result.Errors += worker.result.Errors
		result.Indeterminate += worker.result.Indeterminate
		result.AppliedWithError += worker.result.AppliedWithError
		result.RequestBytes += worker.result.RequestBytes
		result.ResponseBytes += worker.result.ResponseBytes
		for _, example := range worker.result.ErrorExamples {
			if len(result.ErrorExamples) < 5 {
				result.ErrorExamples = append(result.ErrorExamples, example)
			}
		}
		hist.merge(&worker.hist)
	}
	result.NotAttempted = result.Planned - result.Attempted
	if result.ElapsedNS > 0 {
		result.OperationsPerSec = float64(result.Succeeded) / elapsed.Seconds()
	}
	result.Latency = hist.summary()
	return result
}

func (r Result) complete() bool {
	return r.Planned > 0 && r.Attempted == r.Planned && r.Succeeded == r.Planned && r.Errors == 0 && r.Indeterminate == 0 && r.NotAttempted == 0 && r.ElapsedNS > 0
}

func (p *Pair) qualify() {
	p.WeirDirectRatio, p.WeirDeltaPercent = nil, nil
	if !p.Direct.complete() || !p.Weir.complete() || !p.Direct.Verified || !p.Weir.Verified || p.Direct.Planned != p.Weir.Planned {
		return
	}
	ratio := p.Weir.OperationsPerSec / p.Direct.OperationsPerSec
	delta := (ratio - 1) * 100
	p.WeirDirectRatio, p.WeirDeltaPercent = &ratio, &delta
}

func (r *Report) aggregate() {
	var directCount, weirCount uint64
	var directNS, weirNS int64
	var qualified int
	for _, pair := range r.Pairs {
		if pair.WeirDirectRatio == nil {
			continue
		}
		qualified++
		directCount += pair.Direct.Succeeded
		weirCount += pair.Weir.Succeeded
		directNS += pair.Direct.ElapsedNS
		weirNS += pair.Weir.ElapsedNS
	}
	aggregate := Aggregate{QualifiedPairs: qualified}
	if qualified == len(r.Pairs) && qualified == r.Parameters.Rounds && r.Incomplete == "" && directNS > 0 && weirNS > 0 {
		aggregate.DirectOpsPerSec = float64(directCount) * 1e9 / float64(directNS)
		aggregate.WeirOpsPerSec = float64(weirCount) * 1e9 / float64(weirNS)
		ratio := aggregate.WeirOpsPerSec / aggregate.DirectOpsPerSec
		delta := (ratio - 1) * 100
		aggregate.WeirDirectRatio, aggregate.WeirDeltaPercent = &ratio, &delta
	}
	r.Aggregate = aggregate
}

func (r *Report) Successful() bool {
	return r != nil && r.Aggregate.WeirDirectRatio != nil && r.Incomplete == ""
}

// Eight subdivisions per power of two bound approximation to at most 12.5%
// above the observed sample (plus microsecond rounding). Memory is fixed.
type histogram struct {
	buckets [256]uint64
	count   uint64
	max     int64
}

func (h *histogram) add(duration time.Duration) {
	ns := max(int64(duration), 0)
	us := uint64(ns / 1000)
	if ns%1000 != 0 {
		us++
	}
	index := 0
	if us > 0 {
		exponent := bits.Len64(us) - 1
		base := uint64(1) << exponent
		index = min(255, 1+exponent*8+int((us-base)*8/base))
	}
	h.buckets[index]++
	h.count++
	h.max = max(h.max, ns)
}

func (h *histogram) merge(other *histogram) {
	for index, count := range other.buckets {
		h.buckets[index] += count
	}
	h.count += other.count
	h.max = max(h.max, other.max)
}

func (h *histogram) quantile(percent uint64) int64 {
	if h.count == 0 {
		return 0
	}
	rank := (h.count*percent + 99) / 100
	var observed uint64
	for index, count := range h.buckets {
		observed += count
		if observed < rank {
			continue
		}
		if index == 0 {
			return 0
		}
		if index == 255 {
			return h.max
		}
		exponent, fraction := (index-1)/8, (index-1)%8
		base := uint64(1) << exponent
		upper := base + ((uint64(fraction+1)*base + 7) / 8) - 1
		if upper > math.MaxInt64/1000 {
			return h.max
		}
		return max(int64(upper)*1000, 1000)
	}
	return h.max
}

func (h *histogram) summary() Latency {
	latency := Latency{Samples: h.count, P50NS: h.quantile(50), P95NS: h.quantile(95), P99NS: h.quantile(99), MaxNS: h.max, Method: "256 fixed logarithmic buckets, 8 subdivisions/power-of-two, microsecond rounding; quantiles are inclusive bucket upper bounds (<=12.5% plus 1us); final overflow bucket uses observed maximum"}
	return latency
}
