package benchmark

import (
	"context"
	"testing"
	"time"

	"github.com/batchstream/weir-tests/internal/workload"
)

// Calling a bulk API for even one item invalidates this benchmark's baseline.
type singleOnlyExecutor struct{ fakeExecutor }

func (*singleOnlyExecutor) ExecuteBatch(context.Context, []workload.Operation) []workload.Outcome {
	panic("single-request workload must never call ExecuteBatch")
}

func singleDataset(t *testing.T) *workload.Dataset {
	t.Helper()
	config := workload.Config{Backend: "search", StoreName: "search", Namespace: "weirtest_0123456789abcdef01234567", Records: 32, Concurrency: 8}
	dataset, err := workload.New(config)
	if err != nil {
		t.Fatal(err)
	}
	return dataset
}

func TestSingleCallsUseDisjointGlobalWorkersAndKeepWarmupRevisions(t *testing.T) {
	dataset := singleDataset(t)
	state := newDurationState(dataset, 8, 1, 100)
	expected := make([]int, dataset.Config.Records)
	for range 16 {
		for worker := range 8 {
			operation := state.nextSingle(worker)
			start, end := worker*dataset.Config.Records/8, (worker+1)*dataset.Config.Records/8
			if operation.Record < start || operation.Record >= end || !operation.Write || operation.Revision == expected[operation.Record] {
				t.Fatal("cross-process ownership or real write-change invariant violated", operation)
			}
			expected[operation.Record] = operation.Revision
		}
	}
	for record, revision := range state.plan().Expected {
		if revision != expected[record] {
			t.Fatal("postflight state lost warmup revisions", record)
		}
	}
}

func TestSingleMeasurementNeverCallsBulkAndJoinsEachRequest(t *testing.T) {
	dataset := singleDataset(t)
	state := newDurationState(dataset, 8, 1, 0)
	executor := &singleOnlyExecutor{fakeExecutor: fakeExecutor{delay: 2 * time.Millisecond}}
	command := clientCommand{Phase: "timed", StartUnixNS: time.Now().Add(5 * time.Millisecond).UnixNano(), DurationNS: int64(15 * time.Millisecond)}
	opts := singleMeasureOptions{Executor: executor, State: state, WorkerOffset: 4, Threads: 2, Timeout: time.Second, Command: command}
	result := measureSingleRequests(context.Background(), opts)
	if !result.Result.complete() || result.Requests != result.Result.Attempted || result.Reads != result.Requests || result.Writes != 0 || result.Result.ElapsedNS < command.DurationNS || result.Result.Latency.Samples != result.Requests || result.PoolLimit != 2 {
		t.Fatal("single-request accounting or in-flight join invalid", result)
	}
	if len(result.Expected) != 2 || result.Expected[0].Start != 16 || result.Expected[1].Start != 20 {
		t.Fatal("child reported another process's records", result.Expected)
	}
}

func TestSingleMeasurementStopsOnUnknownWithoutRetry(t *testing.T) {
	dataset := singleDataset(t)
	executor := &singleOnlyExecutor{fakeExecutor: fakeExecutor{delay: 20 * time.Millisecond}}
	command := clientCommand{Phase: "timed", StartUnixNS: time.Now().UnixNano(), DurationNS: int64(100 * time.Millisecond)}
	opts := singleMeasureOptions{Executor: executor, State: newDurationState(dataset, 8, 1, 100), Threads: 2, Timeout: time.Millisecond, Command: command}
	result := measureSingleRequests(context.Background(), opts)
	if result.Result.Attempted != 2 || result.Result.Indeterminate != 2 || result.Result.Succeeded != 0 || result.Result.complete() {
		t.Fatal("unknown business writes were replayed or counted as success", result)
	}
}

func TestChildConfigurationAndAcknowledgementCannotHideMissingCalls(t *testing.T) {
	dataset := singleDataset(t)
	config := clientConfig{Dataset: dataset.Config, Path: "direct", Threads: 2, TotalWorkers: 8, WorkerOffset: 4, TimeoutNS: int64(time.Second)}
	if err := validateClientConfig(config); err != nil {
		t.Fatal(err)
	}
	base := Result{Path: "direct", Attempted: 1, Planned: 1, Succeeded: 1, Latency: Latency{Samples: 1}}
	result := ClientProcessResult{Phase: "timed", PID: 7, Threads: 2, WorkerOffset: 4, PoolLimit: 2, Requests: 1, Reads: 1, Result: base}
	result.Buckets[0] = 1
	reply := clientReply{Phase: "timed", PID: 7, Result: &result}
	err := validateClientReply(reply, "timed", config, 7)
	if err != nil {
		t.Fatal(err)
	}
	result.Buckets[0] = 0
	err = validateClientReply(reply, "timed", config, 7)
	if err == nil {
		t.Fatal("missing per-request latency observation accepted")
	}
	config.WorkerOffset = 7
	if err := validateClientConfig(config); err == nil {
		t.Fatal("overlapping worker range accepted")
	}
}

func TestBusinessReportPoolsLatencyAndKeepsUnderfullObservations(t *testing.T) {
	var fast, slow histogram
	for range 100 {
		fast.add(time.Millisecond)
	}
	slow.add(100 * time.Millisecond)
	fastBase := Result{Latency: fast.summary()}
	slowBase := Result{Latency: slow.summary()}
	fastClient := ClientProcessResult{Result: fastBase, Buckets: fast.buckets}
	slowClient := ClientProcessResult{Result: slowBase, Buckets: slow.buckets}
	base := Result{Planned: 101, Attempted: 101, Succeeded: 101, ElapsedNS: int64(time.Second), Verified: true}
	stage := SaturationResult{Result: base, ClientProcesses: []ClientProcessResult{fastClient, slowClient}}
	parameters := SaturationParameters{Concurrency: []int{4, 8}, BatchSizes: []int{1}, Rounds: 1, ClientProcesses: 2}
	report := &SaturationReport{Parameters: parameters}
	for _, level := range parameters.Concurrency {
		pair := SaturationPair{Round: 1, Concurrency: level, BatchSize: 1, Direct: stage, Weir: stage}
		report.Pairs = append(report.Pairs, pair)
	}
	report.aggregate()
	report.aggregateBusiness()
	if len(report.BusinessComparisons) != 2 || report.Comparisons[0].Ratio != nil || report.BusinessPeaks == nil {
		t.Fatal("underfull CPU hid measured business performance or fabricated capacity")
	}
	latency := report.BusinessComparisons[0].DirectLatency
	if latency.Samples != 101 || latency.P99NS > int64(2*time.Millisecond) || latency.MaxNS != int64(100*time.Millisecond) {
		t.Fatal("process percentiles were averaged rather than pooling all calls", latency)
	}
}

func TestLateBarrierInvalidatesThePhaseWithoutBusinessRequests(t *testing.T) {
	dataset := singleDataset(t)
	executor := &singleOnlyExecutor{}
	command := clientCommand{Phase: "timed", StartUnixNS: time.Now().Add(-200 * time.Millisecond).UnixNano(), DurationNS: int64(time.Second)}
	opts := singleMeasureOptions{Executor: executor, State: newDurationState(dataset, 8, 1, 0), Threads: 1, Timeout: time.Second, Command: command}
	result := measureSingleRequests(context.Background(), opts)
	if result.Requests != 0 || result.Result.complete() || result.StartLagNS < int64(100*time.Millisecond) || len(result.Result.ErrorExamples) == 0 {
		t.Fatal("late common start silently shortened a supposedly valid measurement", result)
	}
}
