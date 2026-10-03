package benchmark

import (
	"context"
	"fmt"
	"math"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/batchstream/weir-tests/internal/workload"
)

func TestResourceEvidenceRejectsMissingIntervalsAndShortSpikes(t *testing.T) {
	resources := Resources{AllocatedCPUs: 1, MeasurementNS: int64(10 * time.Second)}
	for index := range 11 {
		cpu := int64(float64(index) * float64(time.Second) * 0.95)
		sample := ResourceSample{ElapsedNS: int64(time.Duration(index) * time.Second), CPUTimeNS: &cpu}
		resources.Samples = append(resources.Samples, sample)
	}
	resources.qualify(90)
	if !resources.CPUSaturated || resources.Coverage != 1 || resources.FullCPUFraction != 1 {
		t.Fatal("continuous full CPU evidence rejected", resources)
	}
	missing := Resources{AllocatedCPUs: 1, MeasurementNS: int64(10 * time.Second), Samples: append([]ResourceSample(nil), resources.Samples...)}
	for index := 6; index < 11; index++ {
		missing.Samples[index].Error = "monitor timed out"
	}
	missing.qualify(90)
	if missing.CPUSaturated || missing.Coverage != 0.5 || missing.FullCPUFraction != 0.5 {
		t.Fatal("five early full CPU samples hid missing half of run", missing)
	}
	spike := Resources{AllocatedCPUs: 1, MeasurementNS: int64(10 * time.Second), Samples: append([]ResourceSample(nil), resources.Samples...)}
	for index := 1; index < 11; index++ {
		cpu := int64(float64(index) * float64(time.Second) * 0.2)
		if index == 10 {
			cpu += int64(750 * time.Millisecond)
		}
		spike.Samples[index].CPUTimeNS = &cpu
	}
	spike.qualify(90)
	if spike.CPUSaturated {
		t.Fatal("one CPU spike was reported as full load")
	}
}

func TestDatabaseCounterFailuresDoNotInvalidateIndependentCPUIntervals(t *testing.T) {
	resources := Resources{AllocatedCPUs: 1, MeasurementNS: int64(10 * time.Second)}
	clock := time.Date(2026, time.October, 4, 0, 0, 0, 0, time.UTC)
	for index := range 11 {
		cpu := int64(time.Duration(index) * time.Second)
		sample := ResourceSample{ElapsedNS: int64(time.Duration(index) * time.Second), CPUTimeNS: &cpu, CPUClockNS: clock.Add(time.Duration(index) * time.Second).UnixNano(), DatabaseCounterError: "projected database counters unavailable"}
		resources.Samples = append(resources.Samples, sample)
	}
	resources.qualify(90)
	if !resources.CPUSaturated || resources.Intervals != 10 || resources.Coverage != 1 || resources.FullCPUFraction != 1 || resources.MeanCPUPercent != 100 {
		t.Fatal("independent valid cumulative CPU evidence was discarded because another observation failed", resources)
	}
	for _, sample := range resources.Samples {
		if sample.DatabaseCounterError == "" || sample.Error != "" {
			t.Fatal("counter failure was hidden or classified as a CPU failure", sample)
		}
	}
	failedCPU := Resources{AllocatedCPUs: 1, MeasurementNS: resources.MeasurementNS, Samples: append([]ResourceSample(nil), resources.Samples...)}
	for index := range failedCPU.Samples {
		failedCPU.Samples[index].Error = "Docker cumulative CPU sampling failed"
	}
	failedCPU.qualify(90)
	if failedCPU.CPUSaturated || failedCPU.Coverage != 0 {
		t.Fatal("separating counter failures incorrectly admitted failed CPU samples", failedCPU)
	}
}

func TestDockerEvidenceUsesRawCumulativeCPUAndReadClock(t *testing.T) {
	container := strings.Repeat("a", 64)
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path != "/containers/"+container+"/stats" || request.URL.Query().Get("stream") != "false" || request.URL.Query().Get("one-shot") != "true" {
			t.Error("unexpected Docker Engine resource request", request.URL)
		}
		fmt.Fprint(writer, `{"read":"2026-10-03T00:00:00Z","cpu_stats":{"cpu_usage":{"total_usage":1234567890}},"precpu_stats":{"cpu_usage":{"total_usage":1}},"memory_stats":{"usage":1024},"networks":{"eth0":{"rx_bytes":20,"tx_bytes":30}},"blkio_stats":{"io_service_bytes_recursive":[{"op":"Read","value":40},{"op":"Write","value":50},{"op":"Total","value":90}]}}`)
	}))
	defer server.Close()
	client := server.Client()
	transport := client.Transport.(*http.Transport).Clone()
	transport.Proxy = func(*http.Request) (*url.URL, error) { return url.Parse(server.URL) }
	client.Transport = transport
	defer client.CloseIdleConnections()
	sample, err := sampleDocker(context.Background(), client, container)
	if err != nil || sample.CPUTimeNS == nil || *sample.CPUTimeNS != 1234567890 || sample.CPUClockNS == 0 || sample.MemoryBytes != 1024 || sample.BlockReadBytes != 40 || sample.BlockWriteBytes != 50 || sample.NetworkInBytes != 20 || sample.NetworkOutBytes != 30 || sample.CPUBudgetPercent != nil {
		t.Fatal("resource collection used snapshot percentage or lost raw counters", sample, err)
	}
}

func TestNativeCPUUsesCumulativeTimeAndAllocatedBudget(t *testing.T) {
	resources := Resources{AllocatedCPUs: 2, MeasurementNS: int64(10 * time.Second)}
	for index := range 11 {
		cpu := int64(time.Duration(index) * 2 * time.Second)
		sample := ResourceSample{ElapsedNS: int64(time.Duration(index) * time.Second), CPUTimeNS: &cpu}
		resources.Samples = append(resources.Samples, sample)
	}
	resources.qualify(90)
	if !resources.CPUSaturated || resources.MeanCPUPercent != 100 {
		t.Fatal("CPU time did not normalize to allocated cores", resources)
	}
	for _, test := range []struct {
		raw  string
		want time.Duration
	}{{"00:02", 2 * time.Second}, {"1:02.50", 62500 * time.Millisecond}, {"01:02:03", 3723 * time.Second}, {"2-01:02:03", (2*86400 + 3723) * time.Second}} {
		got, err := parseCPUTime(test.raw)
		if err != nil || got != int64(test.want) {
			t.Fatalf("%s: %d %v", test.raw, got, err)
		}
	}
	for _, raw := range []string{"no-time", "-1:02", "1:2:3:4"} {
		if _, err := parseCPUTime(raw); err == nil {
			t.Fatal("invalid CPU clock accepted", raw)
		}
	}
}

func TestSaturationRatioNeedsBothPathsFullCPUAndThroughputPlateau(t *testing.T) {
	parameters := SaturationParameters{Concurrency: []int{8, 32}, BatchSizes: []int{16}, Rounds: 2}
	report := &SaturationReport{Parameters: parameters}
	for _, workers := range parameters.Concurrency {
		for round := range 2 {
			base := Result{Planned: 1000, Attempted: 1000, Succeeded: 1000, ElapsedNS: int64(time.Second), Verified: true}
			direct := SaturationResult{Result: base, Resources: Resources{CPUSaturated: true, MeanCPUPercent: 95}}
			via := direct
			via.Succeeded = 800
			via.Attempted = 800
			via.Planned = 800
			if workers == 32 {
				direct.Succeeded = 1050
				direct.Attempted = 1050
				direct.Planned = 1050
				via.Succeeded = 840
				via.Attempted = 840
				via.Planned = 840
			}
			pair := SaturationPair{Round: round + 1, Concurrency: workers, BatchSize: 16, Direct: direct, Weir: via}
			report.Pairs = append(report.Pairs, pair)
		}
	}
	report.aggregate()
	if report.Comparisons[0].Ratio == nil || math.Abs(*report.Comparisons[0].Ratio-0.8) > 1e-12 {
		t.Fatal("verified saturated capacity ratio missing", report.Comparisons)
	}
	report.Pairs[0].Weir.Resources.CPUSaturated = false
	report.aggregate()
	if report.Comparisons[0].Ratio != nil || !strings.Contains(report.Markdown(), "unavailable") {
		t.Fatal("client/Weir-bound plateau produced database-full-load comparison")
	}
	report.Pairs[0].Weir.Resources.CPUSaturated = true
	for index := range report.Pairs {
		if report.Pairs[index].Concurrency == 32 {
			report.Pairs[index].Weir.Succeeded = 1600
			report.Pairs[index].Weir.Planned = 1600
			report.Pairs[index].Weir.Attempted = 1600
		}
	}
	report.aggregate()
	if report.Comparisons[0].Ratio != nil {
		t.Fatal("rising throughput without next-stage plateau produced saturated capacity")
	}
}

func TestDurationWorkloadKeepsDistinctIDsAndRealRevisionChanges(t *testing.T) {
	config := workload.Config{Backend: "search", StoreName: "search", Namespace: "weirtest_0123456789abcdef01234567", Records: 24, Concurrency: 2}
	dataset, err := workload.New(config)
	if err != nil {
		t.Fatal(err)
	}
	state := newDurationState(dataset, 2, 8, 100)
	last := make(map[int]int)
	for range 5 {
		for worker := range 2 {
			batch := state.next(worker)
			seen := make(map[int]bool)
			for _, operation := range batch {
				if seen[operation.Record] || operation.Revision == last[operation.Record] || operation.Record < worker*12 || operation.Record >= (worker+1)*12 {
					t.Fatal("batch duplicates, no-op write, or cross-worker race", operation)
				}
				seen[operation.Record] = true
				last[operation.Record] = operation.Revision
			}
		}
	}
	for record, revision := range state.plan().Expected {
		if revision != last[record] {
			t.Fatal("postflight state differs", record)
		}
	}
}

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
