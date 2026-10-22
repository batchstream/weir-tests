package benchmark

import (
	"context"
	"testing"
	"time"

	"github.com/batchstream/weir-tests/internal/workload"
)

func TestDurationMeasurementJoinsInFlightBatchesAndCountsLogicalWork(t *testing.T) {
	config := workload.Config{Backend: "search", StoreName: "search", Namespace: "weirtest_0123456789abcdef01234567", Records: 16, Concurrency: 2}
	dataset, err := workload.New(config)
	if err != nil {
		t.Fatal(err)
	}
	executor := &fakeExecutor{delay: 2 * time.Millisecond}
	opts := durationMeasureOptions{Executor: executor, State: newDurationState(dataset, 2, 4, 0), Duration: 15 * time.Millisecond, Timeout: time.Second}
	result := measureDuration(context.Background(), opts)
	if !result.complete() || result.Attempted != result.Requests*4 || result.Reads != result.Attempted || result.ElapsedNS < int64(opts.Duration) || result.Latency.Samples != result.Attempted {
		t.Fatal("batch measurement omitted full completion or logical work", result)
	}
}

func TestBulkDurationCountsLogicalOperationsInExpiredBatches(t *testing.T) {
	config := workload.Config{Backend: "search", StoreName: "search", Namespace: "weirtest_0123456789abcdef01234567", Records: 8, Concurrency: 2}
	dataset, err := workload.New(config)
	if err != nil {
		t.Fatal(err)
	}
	executor := &fakeExecutor{delay: time.Second}
	opts := durationMeasureOptions{Executor: executor, State: newDurationState(dataset, 2, 4, 0), Duration: time.Second, Timeout: time.Millisecond}
	result := measureDuration(t.Context(), opts)
	if result.Timeouts != 8 || result.Attempted != 8 || result.Indeterminate != 8 || result.Requests != 2 {
		t.Fatalf("expired batches must count logical operations: %+v", result)
	}
}
