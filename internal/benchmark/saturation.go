package benchmark

import (
	"context"
	"errors"
	"fmt"
	"runtime"
	"sync"
	"time"

	"github.com/batchstream/weir-tests/internal/fixture"
	"github.com/batchstream/weir-tests/internal/observe"
	"github.com/batchstream/weir-tests/internal/workload"
)

type SaturationOptions struct {
	Dataset          *workload.Dataset
	Paths            *workload.Paths
	Concurrency      []int
	BatchSizes       []int
	Warmup           time.Duration
	Duration         time.Duration
	Rounds           int
	WritePercent     int
	OperationTimeout time.Duration
	Resources        fixture.ResourceTarget
	CPUThreshold     float64
	Provenance       map[string]string
}

type SaturationParameters struct {
	Dataset      workload.Config `json:"dataset"`
	Concurrency  []int           `json:"concurrency_levels"`
	BatchSizes   []int           `json:"batch_sizes"`
	Warmup       string          `json:"warmup_duration"`
	Duration     string          `json:"measurement_duration"`
	Rounds       int             `json:"paired_rounds"`
	WritePercent int             `json:"requested_write_percent"`
	CPUThreshold float64         `json:"database_cpu_budget_threshold_percent"`
}

type SaturationResult struct {
	Result
	Reads         uint64         `json:"reads"`
	Writes        uint64         `json:"writes"`
	Requests      uint64         `json:"client_batch_requests"`
	Resources     Resources      `json:"resources"`
	ServerMetrics *ServerMetrics `json:"server_metrics,omitempty"`
}

type SaturationPair struct {
	Round       int              `json:"round"`
	Concurrency int              `json:"concurrency"`
	BatchSize   int              `json:"batch_size"`
	Order       []string         `json:"order"`
	Direct      SaturationResult `json:"direct"`
	Weir        SaturationResult `json:"weir"`
}

type SaturationPoint struct {
	Path                  string  `json:"path"`
	Concurrency           int     `json:"concurrency"`
	BatchSize             int     `json:"batch_size"`
	OperationsPerSec      float64 `json:"successful_operations_per_second"`
	MeanCPUPercent        float64 `json:"mean_database_cpu_budget_percent"`
	AllRoundsVerified     bool    `json:"all_rounds_successful_verified"`
	AllRoundsCPUSaturated bool    `json:"all_rounds_cpu_saturated"`
	Plateau               bool    `json:"throughput_plateau_at_next_concurrency"`
	Saturated             bool    `json:"database_saturation_demonstrated"`
}

type SaturationComparison struct {
	BatchSize    int              `json:"batch_size"`
	Direct       *SaturationPoint `json:"direct_database_saturated_point,omitempty"`
	Weir         *SaturationPoint `json:"weir_database_saturated_point,omitempty"`
	Ratio        *float64         `json:"weir_direct_saturated_throughput_ratio,omitempty"`
	DeltaPercent *float64         `json:"weir_saturated_throughput_delta_percent,omitempty"`
	Unavailable  string           `json:"unavailable,omitempty"`
}

type SaturationReport struct {
	Schema      int                    `json:"schema"`
	Started     string                 `json:"started_utc"`
	Parameters  SaturationParameters   `json:"parameters"`
	Provenance  map[string]string      `json:"provenance"`
	CPUQuota    *DatabaseCPUQuota      `json:"database_cpu_quota,omitempty"`
	Pairs       []SaturationPair       `json:"pairs"`
	Points      []SaturationPoint      `json:"points"`
	Comparisons []SaturationComparison `json:"database_saturated_comparisons"`
	Method      string                 `json:"method"`
	Incomplete  string                 `json:"incomplete,omitempty"`
}

func RunSaturation(ctx context.Context, options SaturationOptions) (*SaturationReport, error) {
	if err := validateSaturation(options); err != nil {
		return nil, err
	}
	quota, err := verifyResourceBudget(ctx, options.Resources)
	if err != nil {
		return nil, err
	}
	parameters := SaturationParameters{Dataset: options.Dataset.Config, Concurrency: options.Concurrency, BatchSizes: options.BatchSizes, Warmup: options.Warmup.String(), Duration: options.Duration.String(), Rounds: options.Rounds, WritePercent: options.WritePercent, CPUThreshold: options.CPUThreshold}
	provenance := make(map[string]string, len(options.Provenance)+3)
	for key, value := range options.Provenance {
		provenance[key] = value
	}
	provenance["go"], provenance["gomaxprocs"], provenance["client_cpu_count"] = runtime.Version(), fmt.Sprint(runtime.GOMAXPROCS(0)), fmt.Sprint(runtime.NumCPU())
	report := &SaturationReport{Schema: 2, Started: time.Now().UTC().Format(time.RFC3339Nano), Parameters: parameters, Provenance: provenance, CPUQuota: quota, Method: "duration-based ascending concurrency sweep; matched native and same-Store SDK bulk batches; deterministic worker-owned IDs and real revision changes; alternating AB/BA pairs; independent postflight; CPU saturation requires >=5 valid intervals covering >=80% of total measurement time, time-weighted mean CPU>=threshold and CPU>=threshold during >=80% of total measurement time in every round, plus <=10% throughput gain at the next concurrency level; I/O/network byte counters do not prove saturation"}
	for _, batch := range options.BatchSizes {
		for level, workers := range options.Concurrency {
			for round := range options.Rounds {
				pair := SaturationPair{Round: round + 1, Concurrency: workers, BatchSize: batch, Order: pairOrder(level + round)}
				for _, name := range pair.Order {
					if err := options.Paths.Prepare(ctx, 64); err != nil {
						report.Incomplete = "prepare: " + err.Error()
						return report, err
					}
					executor := options.Paths.Direct
					if name == "weir" {
						executor = options.Paths.Weir
					}
					state := newDurationState(options.Dataset, workers, batch, options.WritePercent)
					warmup := durationMeasureOptions{Executor: executor, State: state, Duration: options.Warmup, Timeout: options.OperationTimeout}
					warm := measureDuration(ctx, warmup)
					if !warm.complete() {
						pairValue := warm
						if name == "direct" {
							pair.Direct = pairValue
						} else {
							pair.Weir = pairValue
						}
						report.Pairs = append(report.Pairs, pair)
						report.Incomplete = fmt.Sprintf("%s warmup failed: %v", name, warm.ErrorExamples)
						return report, errors.New(report.Incomplete)
					}
					measurement := durationMeasureOptions{Executor: executor, State: state, Duration: options.Duration, Timeout: options.OperationTimeout, Paths: options.Paths, Target: options.Resources, CPUThreshold: options.CPUThreshold, Sample: true}
					result := measureDuration(ctx, measurement)
					if err := options.Paths.Verify(ctx, state.plan(), 64); err != nil {
						result.VerificationError = err.Error()
					} else {
						result.Verified = true
					}
					if name == "direct" {
						pair.Direct = result
					} else {
						pair.Weir = result
					}
					if !result.complete() || !result.Verified || ctx.Err() != nil {
						report.Incomplete = fmt.Sprintf("%s timed stage failed: %v %s", name, result.ErrorExamples, result.VerificationError)
						report.Pairs = append(report.Pairs, pair)
						return report, errors.New(report.Incomplete)
					}
				}
				report.Pairs = append(report.Pairs, pair)
				fmt.Printf("saturation backend=%s batch=%d concurrency=%d round=%d direct=%.1f weir=%.1f DB-CPU direct=%.1f%% weir=%.1f%%\n", options.Dataset.Config.Backend, batch, workers, round+1, pair.Direct.OperationsPerSec, pair.Weir.OperationsPerSec, pair.Direct.Resources.MeanCPUPercent, pair.Weir.Resources.MeanCPUPercent)
			}
		}
	}
	report.aggregate()
	return report, nil
}

func validateSaturation(options SaturationOptions) error {
	if options.Dataset == nil || options.Paths == nil || options.Rounds < 1 || options.Rounds > 10 || options.Warmup < time.Second || options.Duration < 5*time.Second || options.Duration > 10*time.Minute || options.OperationTimeout <= 0 || options.OperationTimeout > time.Minute || options.WritePercent < 0 || options.WritePercent > 100 || options.CPUThreshold < 50 || options.CPUThreshold > 100 || options.Resources.AllocatedCPUs <= 0 {
		return errors.New("invalid saturation benchmark parameters")
	}
	if len(options.Concurrency) < 2 || len(options.Concurrency) > 12 || len(options.BatchSizes) < 1 || len(options.BatchSizes) > 8 {
		return errors.New("saturation requires 2 through 12 concurrency levels and 1 through 8 batch sizes")
	}
	previous := 0
	for _, workers := range options.Concurrency {
		if workers <= previous || workers > options.Dataset.Config.Concurrency {
			return errors.New("concurrency ladder must strictly increase within the opened pool size")
		}
		previous = workers
	}
	seen := make(map[int]bool)
	for _, batch := range options.BatchSizes {
		if batch < 1 || batch > 64 || seen[batch] || options.Dataset.Config.Records < previous*batch {
			return errors.New("batch sizes must be distinct, 1 through 64, with at least max-workers*batch-size records")
		}
		seen[batch] = true
	}
	return nil
}

type durationWorker struct {
	sequence  int
	start     int
	end       int
	cursor    int
	revisions []int
}
type durationState struct {
	dataset *workload.Dataset
	workers []durationWorker
	batch   int
	writes  int
}

func newDurationState(dataset *workload.Dataset, workers, batch, writes int) *durationState {
	state := &durationState{dataset: dataset, workers: make([]durationWorker, workers), batch: batch, writes: writes}
	for index := range workers {
		start, end := index*dataset.Config.Records/workers, (index+1)*dataset.Config.Records/workers
		worker := durationWorker{start: start, end: end, revisions: make([]int, end-start)}
		state.workers[index] = worker
	}
	return state
}

func (s *durationState) next(workerIndex int) []workload.Operation {
	worker := &s.workers[workerIndex]
	x := uint64(worker.sequence)*0x9e3779b97f4a7c15 + uint64(workerIndex+1)*0xbf58476d1ce4e5b9
	x ^= x >> 30
	x *= 0xbf58476d1ce4e5b9
	x ^= x >> 27
	write := int((x>>32)%100) < s.writes
	operations := make([]workload.Operation, s.batch)
	for index := range s.batch {
		record := worker.start + worker.cursor
		if write {
			worker.revisions[worker.cursor] = 1 - worker.revisions[worker.cursor]
		}
		operation := workload.Operation{Worker: workerIndex, Sequence: worker.sequence, Record: record, Write: write, Revision: worker.revisions[worker.cursor]}
		operations[index] = operation
		worker.cursor = (worker.cursor + 1) % (worker.end - worker.start)
		worker.sequence++
	}
	return operations
}

func (s *durationState) plan() *workload.Plan {
	plan := &workload.Plan{Expected: make([]int, s.dataset.Config.Records)}
	for _, worker := range s.workers {
		copy(plan.Expected[worker.start:worker.end], worker.revisions)
	}
	return plan
}

type durationMeasureOptions struct {
	Executor     workload.Executor
	State        *durationState
	Duration     time.Duration
	Timeout      time.Duration
	Paths        *workload.Paths
	Target       fixture.ResourceTarget
	CPUThreshold float64
	Sample       bool
}

func measureDuration(ctx context.Context, options durationMeasureOptions) SaturationResult {
	var metricsBefore observe.Snapshot
	measureServer := options.Sample && options.Executor.Name() == "weir"
	if measureServer {
		metricsBefore = fetchServerMetrics(ctx, options.Target.Diagnostics)
	}
	workers := make([]workerResult, len(options.State.workers))
	reads, writes, requests := make([]uint64, len(workers)), make([]uint64, len(workers)), make([]uint64, len(workers))
	start := make(chan struct{})
	var ready, joined sync.WaitGroup
	ready.Add(len(workers))
	joined.Add(len(workers))
	var deadline time.Time
	for index := range workers {
		go func() {
			defer joined.Done()
			ready.Done()
			<-start
			worker := &workers[index]
			for ctx.Err() == nil && time.Now().Before(deadline) {
				operations := options.State.next(index)
				operationCtx, cancel := context.WithTimeout(ctx, options.Timeout)
				began := time.Now()
				outcomes := options.Executor.ExecuteBatch(operationCtx, operations)
				elapsed := time.Since(began)
				cancel()
				requests[index]++
				if len(outcomes) != len(operations) {
					outcomes = make([]workload.Outcome, len(operations))
					for i := range outcomes {
						outcome := workload.Outcome{Status: workload.Indeterminate, Error: "batch outcome count mismatch"}
						outcomes[i] = outcome
					}
				}
				for operationIndex, outcome := range outcomes {
					worker.hist.add(elapsed)
					worker.result.Attempted++
					if operations[operationIndex].Write {
						writes[index]++
					} else {
						reads[index]++
					}
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
				if worker.result.Errors+worker.result.Indeterminate > 0 {
					break
				}
			}
		}()
	}
	resources := Resources{AllocatedCPUs: options.Target.AllocatedCPUs, Source: "owned database Docker Engine raw cumulative CPU/memory/block/network counters or native ps CPU; DB cumulative counters; client/Weir ps; 1s sampling with bounded 3s calls; inspected quota denominator"}
	resourceClient, resourceErr := resourceHTTPClient(options.Target)
	if resourceClient != nil {
		defer resourceClient.CloseIdleConnections()
	}
	var monitorJoined sync.WaitGroup
	monitorStop := make(chan struct{})
	ready.Wait()
	started := time.Now()
	deadline = started.Add(options.Duration)
	if options.Sample {
		monitorJoined.Add(1)
		go func() {
			defer monitorJoined.Done()
			for {
				select {
				case <-monitorStop:
					return
				case <-ctx.Done():
					return
				default:
				}
				sampleCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
				sampleOptions := resourceSampleOptions{Target: options.Target, Paths: options.Paths, Client: resourceClient, Started: started}
				sample := sampleResources(sampleCtx, sampleOptions)
				cancel()
				if resourceErr != nil {
					sample.Error = resourceErr.Error()
				}
				resources.Samples = append(resources.Samples, sample)
				timer := time.NewTimer(time.Second)
				select {
				case <-monitorStop:
					timer.Stop()
					return
				case <-ctx.Done():
					timer.Stop()
					return
				case <-timer.C:
				}
			}
		}()
	}
	close(start)
	joined.Wait()
	elapsed := time.Since(started)
	close(monitorStop)
	monitorJoined.Wait()
	var metricsAfter observe.Snapshot
	if measureServer {
		metricsAfter = fetchServerMetrics(ctx, options.Target.Diagnostics)
	}
	base := Result{Path: options.Executor.Name(), ElapsedNS: elapsed.Nanoseconds(), Evidence: options.Executor.Evidence()}
	base.Evidence.ClientBatch = fmt.Sprintf("native database bulk / same-Store SDK batch, %d distinct records per request", options.State.batch)
	result := SaturationResult{Result: base, Resources: resources}
	if measureServer {
		result.ServerMetrics = serverMetricDelta(metricsBefore, metricsAfter, options.State.dataset.Config.StoreName)
	}
	result.Resources.MeasurementNS = elapsed.Nanoseconds()
	var hist histogram
	for index, worker := range workers {
		result.Attempted += worker.result.Attempted
		result.Succeeded += worker.result.Succeeded
		result.Errors += worker.result.Errors
		result.Indeterminate += worker.result.Indeterminate
		result.AppliedWithError += worker.result.AppliedWithError
		result.RequestBytes += worker.result.RequestBytes
		result.ResponseBytes += worker.result.ResponseBytes
		result.Reads += reads[index]
		result.Writes += writes[index]
		result.Requests += requests[index]
		for _, example := range worker.result.ErrorExamples {
			if len(result.ErrorExamples) < 5 {
				result.ErrorExamples = append(result.ErrorExamples, example)
			}
		}
		hist.merge(&worker.hist)
	}
	result.Planned = result.Attempted
	if elapsed > 0 {
		result.OperationsPerSec = float64(result.Succeeded) / elapsed.Seconds()
	}
	result.Latency = hist.summary()
	if options.Sample {
		result.Resources.qualify(options.CPUThreshold)
	}
	return result
}

func (r *SaturationReport) aggregate() {
	r.Points = nil
	r.Comparisons = nil
	for _, batch := range r.Parameters.BatchSizes {
		for _, workers := range r.Parameters.Concurrency {
			for _, path := range []string{"direct", "weir"} {
				point := SaturationPoint{Path: path, Concurrency: workers, BatchSize: batch, AllRoundsVerified: true, AllRoundsCPUSaturated: true}
				var count uint64
				var elapsed int64
				rounds := 0
				for _, pair := range r.Pairs {
					if pair.BatchSize != batch || pair.Concurrency != workers {
						continue
					}
					result := pair.Direct
					if path == "weir" {
						result = pair.Weir
					}
					rounds++
					count += result.Succeeded
					elapsed += result.ElapsedNS
					point.MeanCPUPercent += result.Resources.MeanCPUPercent
					point.AllRoundsVerified = point.AllRoundsVerified && result.complete() && result.Verified
					point.AllRoundsCPUSaturated = point.AllRoundsCPUSaturated && result.Resources.CPUSaturated
				}
				point.AllRoundsVerified = point.AllRoundsVerified && rounds == r.Parameters.Rounds
				point.AllRoundsCPUSaturated = point.AllRoundsCPUSaturated && rounds == r.Parameters.Rounds
				if elapsed > 0 {
					point.OperationsPerSec = float64(count) * 1e9 / float64(elapsed)
				}
				if rounds > 0 {
					point.MeanCPUPercent /= float64(rounds)
				}
				r.Points = append(r.Points, point)
			}
		}
	}
	for index := range r.Points {
		point := &r.Points[index]
		for nextIndex := range r.Points {
			next := r.Points[nextIndex]
			if next.Path != point.Path || next.BatchSize != point.BatchSize || next.Concurrency <= point.Concurrency {
				continue
			}
			point.Plateau = point.AllRoundsVerified && next.AllRoundsVerified && point.OperationsPerSec > 0 && next.OperationsPerSec <= point.OperationsPerSec*1.1
			break
		}
		point.Saturated = point.AllRoundsVerified && point.AllRoundsCPUSaturated && point.Plateau && r.Incomplete == ""
	}
	for _, batch := range r.Parameters.BatchSizes {
		comparison := SaturationComparison{BatchSize: batch}
		for index := range r.Points {
			point := &r.Points[index]
			if point.BatchSize != batch || !point.Saturated {
				continue
			}
			if point.Path == "direct" && (comparison.Direct == nil || point.OperationsPerSec > comparison.Direct.OperationsPerSec) {
				comparison.Direct = point
			}
			if point.Path == "weir" && (comparison.Weir == nil || point.OperationsPerSec > comparison.Weir.OperationsPerSec) {
				comparison.Weir = point
			}
		}
		if comparison.Direct != nil && comparison.Weir != nil {
			ratio := comparison.Weir.OperationsPerSec / comparison.Direct.OperationsPerSec
			delta := (ratio - 1) * 100
			comparison.Ratio, comparison.DeltaPercent = &ratio, &delta
		} else {
			comparison.Unavailable = "both paths must independently demonstrate sustained database CPU saturation and a verified next-level throughput plateau; increase concurrency or isolate client/Weir resources; I/O/network-bound saturation needs additional device/link capacity evidence"
		}
		r.Comparisons = append(r.Comparisons, comparison)
	}
}
