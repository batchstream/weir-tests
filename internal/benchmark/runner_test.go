package benchmark

import (
	"context"
	"encoding/json"
	"math"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"github.com/batchstream/weir-tests/internal/workload"
)

type fakeExecutor struct {
	mode  string
	delay time.Duration
}

func (f *fakeExecutor) Name() string { return "direct" }
func (f *fakeExecutor) Evidence() workload.Evidence {
	evidence := workload.Evidence{Protocol: "offline test"}
	return evidence
}
func (f *fakeExecutor) Execute(ctx context.Context, operation workload.Operation) workload.Outcome {
	if f.delay > 0 {
		timer := time.NewTimer(f.delay)
		defer timer.Stop()
		select {
		case <-timer.C:
		case <-ctx.Done():
			outcome := workload.Outcome{Status: workload.Indeterminate, Error: ctx.Err().Error()}
			return outcome
		}
	}
	outcome := workload.Outcome{Status: workload.Success, RequestBytes: 3, ResponseBytes: 7}
	if f.mode == "mixed" {
		switch operation.Sequence % 3 {
		case 1:
			outcome.Status, outcome.Error = workload.Failed, "known failure"
		case 2:
			outcome.Status, outcome.Error = workload.Indeterminate, "unknown acknowledgement"
		}
	}
	return outcome
}

func testPlan(t *testing.T, count int) *workload.Plan {
	t.Helper()
	config := workload.Config{Backend: "search", StoreName: "search", Namespace: "weirtest_0123456789abcdef01234567", Records: 8, Concurrency: 2}
	dataset, err := workload.New(config)
	if err != nil {
		t.Fatal(err)
	}
	options := workload.PlanOptions{Operations: count, WritePercent: 100}
	plan, err := dataset.Plan(options)
	if err != nil {
		t.Fatal(err)
	}
	return plan
}

func TestMeasureCountsAllOutcomesWithoutInflatingThroughput(t *testing.T) {
	plan := testPlan(t, 12)
	executor := &fakeExecutor{mode: "mixed"}
	result := measure(context.Background(), executor, plan, time.Second)
	if result.Attempted != 12 || result.Succeeded != 4 || result.Errors != 4 || result.Indeterminate != 4 || result.NotAttempted != 0 || result.Latency.Samples != 12 {
		t.Fatalf("incorrect accounting: %+v", result)
	}
	want := float64(result.Succeeded) * 1e9 / float64(result.ElapsedNS)
	if math.Abs(result.OperationsPerSec-want)/want > 1e-12 || result.RequestBytes != 36 || result.ResponseBytes != 84 || result.complete() {
		t.Fatal("throughput/bytes accounting included failures")
	}
}

func TestMeasureElapsedIncludesSlowWorkersAndJoin(t *testing.T) {
	plan := testPlan(t, 6)
	executor := &fakeExecutor{delay: 10 * time.Millisecond}
	result := measure(context.Background(), executor, plan, time.Second)
	if !result.complete() || result.ElapsedNS < int64(30*time.Millisecond) || result.ElapsedNS < result.Latency.MaxNS {
		t.Fatalf("elapsed excluded worker work/join: %+v", result)
	}
}

func TestMeasureCancellationJoinsAndReportsUnattempted(t *testing.T) {
	plan := testPlan(t, 100)
	executor := &fakeExecutor{delay: time.Hour}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	result := measure(ctx, executor, plan, time.Second)
	if result.Attempted != 0 || result.NotAttempted != 100 || result.complete() {
		t.Fatal("pre-cancelled run attempted operations")
	}
	ctx, cancel = context.WithTimeout(context.Background(), 5*time.Millisecond)
	defer cancel()
	result = measure(ctx, executor, plan, time.Second)
	if result.Attempted != 2 || result.Indeterminate != 2 || result.NotAttempted != 98 || result.ElapsedNS < int64(time.Millisecond) {
		t.Fatalf("cancel/join accounting: %+v", result)
	}
}

func TestRatiosRequireAllSuccessfulVerifiedPairs(t *testing.T) {
	direct := Result{Path: "direct", Planned: 10, Attempted: 10, Succeeded: 10, ElapsedNS: int64(time.Second), OperationsPerSec: 10, Verified: true}
	via := direct
	via.Path, via.ElapsedNS, via.OperationsPerSec = "weir", int64(2*time.Second), 5
	pair := Pair{Direct: direct, Weir: via}
	pair.qualify()
	if pair.WeirDirectRatio == nil || *pair.WeirDirectRatio != 0.5 || *pair.WeirDeltaPercent != -50 {
		t.Fatal("ratio arithmetic mismatch")
	}
	parameters := Parameters{Rounds: 2}
	report := &Report{Parameters: parameters, Pairs: []Pair{pair, pair}}
	report.aggregate()
	if !report.Successful() || report.Aggregate.DirectOpsPerSec != 10 || report.Aggregate.WeirOpsPerSec != 5 {
		t.Fatal("pooled ratio mismatch")
	}
	broken := Pair{Direct: direct, Weir: via}
	broken.Weir.Succeeded, broken.Weir.Indeterminate = 9, 1
	broken.qualify()
	if broken.WeirDirectRatio != nil {
		t.Fatal("indeterminate pair produced ratio")
	}
	report.Pairs[1] = broken
	report.aggregate()
	if report.Successful() || report.Aggregate.WeirDirectRatio != nil || report.Aggregate.QualifiedPairs != 1 {
		t.Fatal("aggregate hid failed pair")
	}
	broken.Weir = via
	broken.Weir.Verified = false
	broken.qualify()
	if broken.WeirDirectRatio != nil {
		t.Fatal("unverified pair produced ratio")
	}
}

func TestAlternatingPairOrder(t *testing.T) {
	for round := range 6 {
		want := []string{"direct", "weir"}
		if round%2 != 0 {
			want = []string{"weir", "direct"}
		}
		if !reflect.DeepEqual(pairOrder(round), want) {
			t.Fatal("round did not alternate")
		}
	}
}

func TestHistogramUpperBoundsAndMerge(t *testing.T) {
	for exponent := range 32 {
		base := int64(1) << exponent
		for sub := range 17 {
			ns := (base+base*int64(sub)/16)*1000 + 1
			var histogram histogram
			histogram.add(time.Duration(ns))
			upper := histogram.quantile(99)
			if upper < ns || float64(upper) > float64(ns)*1.125+1000 {
				t.Fatalf("ns=%d upper=%d", ns, upper)
			}
		}
	}
	var first, second histogram
	first.add(0)
	second.add(time.Duration(math.MaxInt64))
	first.merge(&second)
	if first.count != 2 || first.quantile(50) != 0 || first.quantile(99) != math.MaxInt64 {
		t.Fatal("histogram zero/overflow/merge mismatch")
	}
}

func TestWriteReportPreservesRawEvidenceAndNoInvalidRatio(t *testing.T) {
	parameters := Parameters{Rounds: 1}
	report := &Report{Schema: 1, Parameters: parameters, Incomplete: "fixture failure"}
	directory := t.TempDir()
	jsonPath, markdownPath := filepath.Join(directory, "report.json"), filepath.Join(directory, "report.md")
	if err := report.Write(jsonPath, markdownPath); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(jsonPath)
	if err != nil {
		t.Fatal(err)
	}
	var parsed Report
	if err := json.Unmarshal(raw, &parsed); err != nil {
		t.Fatal(err)
	}
	if parsed.Incomplete != report.Incomplete || parsed.Aggregate.WeirDirectRatio != nil {
		t.Fatal("raw evidence changed")
	}
	if err := report.Write(jsonPath, jsonPath); err == nil {
		t.Fatal("accepted report path collision")
	}
}
