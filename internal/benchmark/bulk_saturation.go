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

func runBulkSaturation(ctx context.Context, options SaturationOptions) (*SaturationReport, error) {
	if err := validateSaturation(options); err != nil {
		return nil, err
	}
	quota, err := verifyResourceBudget(ctx, options.Resources)
	if err != nil {
		return nil, err
	}
	parameters := SaturationParameters{Dataset: options.Dataset.Config, Concurrency: options.Concurrency, BatchSizes: options.BatchSizes, Warmup: options.Warmup.String(), Duration: options.Duration.String(), Rounds: options.Rounds, WritePercent: options.WritePercent, OperationTimeout: options.OperationTimeout.String(), CPUThreshold: options.CPUThreshold}
	provenance := make(map[string]string, len(options.Provenance)+3)
	for key, value := range options.Provenance {
		provenance[key] = value
	}
	provenance["go"], provenance["gomaxprocs"], provenance["client_cpu_count"] = runtime.Version(), fmt.Sprint(runtime.GOMAXPROCS(0)), fmt.Sprint(runtime.NumCPU())
	report := &SaturationReport{Schema: 1, Kind: SaturationKindBulk, Started: time.Now().UTC().Format(time.RFC3339Nano), Parameters: parameters, Provenance: provenance, CPUQuota: quota, Method: "duration-based ascending concurrency sweep; matched native and same-Store SDK bulk batches; deterministic worker-owned IDs and real revision changes; alternating AB/BA pairs; independent postflight; CPU saturation requires >=5 valid intervals covering >=80% of total measurement time, time-weighted mean CPU>=threshold and CPU>=threshold during >=80% of total measurement time in every round, plus <=10% throughput gain at the next concurrency level; I/O/network byte counters do not prove saturation"}
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
	var beforeDatabase, afterDatabase *workload.DatabaseCounters
	var counterErrors []string
	if options.Sample && options.Paths != nil {
		counters, err := options.Paths.DatabaseCounters(ctx)
		if err != nil {
			counterErrors = append(counterErrors, err.Error())
		} else {
			beforeDatabase = &counters
		}
	}
	var metricsBefore observe.Snapshot
	measureServer := options.Sample && options.Executor.Name() == "weir"
	if measureServer {
		metricsBefore = fetchServerMetrics(ctx, options.Target)
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
				timedOut := errors.Is(operationCtx.Err(), context.DeadlineExceeded)
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
					if timedOut && outcome.Status != workload.Success {
						worker.result.Timeouts++
					}
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
		metricsAfter = fetchServerMetrics(ctx, options.Target)
	}
	if options.Sample && options.Paths != nil {
		counters, err := options.Paths.DatabaseCounters(ctx)
		if err != nil {
			counterErrors = append(counterErrors, err.Error())
		} else {
			afterDatabase = &counters
		}
	}
	base := Result{Path: options.Executor.Name(), ElapsedNS: elapsed.Nanoseconds(), Evidence: options.Executor.Evidence()}
	base.Evidence.ClientBatch = fmt.Sprintf("native database bulk / same-Store SDK batch, %d distinct records per request", options.State.batch)
	result := SaturationResult{Result: base, Resources: resources}
	result.DatabaseBefore = beforeDatabase
	result.DatabaseAfter = afterDatabase
	result.PhysicalCommandUnavailable = mergeCommandUnavailable(counterErrors...)
	result.calculatePhysicalCommands()
	if measureServer {
		result.ServerMetrics = serverMetricDelta(metricsBefore, metricsAfter, options.State.dataset.Config.StoreName)
	}
	result.Resources.MeasurementNS = elapsed.Nanoseconds()
	var hist histogram
	for index, worker := range workers {
		result.Timeouts += worker.result.Timeouts
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
