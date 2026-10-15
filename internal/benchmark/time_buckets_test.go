package benchmark

import (
	"context"
	"math"
	"testing"
	"time"
)

func TestTimeBucketsKeepCompletionBoundariesDrainAndUnknownSubsets(t *testing.T) {
	duration := int64(25 * time.Second)
	buckets := newTimeBuckets(duration, true)
	for _, offeredAt := range []time.Duration{0, 9 * time.Second, 10 * time.Second, 24 * time.Second} {
		timeBucketAt(buckets, int64(offeredAt)).Offered++
	}
	completed := []timeBucketOutcome{
		{OffsetNS: int64(time.Second), Latency: time.Millisecond, Items: 1},
		{OffsetNS: int64(10 * time.Second), Latency: 2 * time.Second, Items: 1, Failed: true, Unknown: true, TimedOut: true},
		{OffsetNS: int64(24 * time.Second), Latency: 20 * time.Millisecond, Items: 1},
		{OffsetNS: int64(26 * time.Second), Latency: 2 * time.Second, Items: 1},
	}
	var latency histogram
	for _, event := range completed {
		recordTimeBucketOutcome(buckets, event)
		latency.add(event.Latency)
	}
	finishTimeBuckets(buckets)
	if len(buckets) != 4 || buckets[0].WidthNS != int64(10*time.Second) || buckets[1].OffsetNS != int64(10*time.Second) || buckets[1].Completed != 0 || buckets[1].Failed != 1 || buckets[1].Unknown != 1 || buckets[2].WidthNS != int64(5*time.Second) || buckets[3].OffsetNS != duration || !buckets[3].Drain || buckets[3].WidthNS != 0 || buckets[3].Completed != 1 || buckets[3].Offered != 0 {
		t.Fatal("boundary completions, partial last window or drain were folded into another bucket", buckets)
	}
	totals := TimeBucket{Offered: 4, Completed: 3, Failed: 1, Unknown: 1, TimedOut: 1}
	options := timeBucketAudit{Buckets: buckets, DurationNS: duration, ItemsPerCall: 1, Totals: totals, Latency: latency.summary(), Histogram: latency.buckets}
	if err := auditTimeBuckets(options); err != nil {
		t.Fatal("all phase counts and pooled latencies must reconcile", err)
	}
	for _, mutation := range []string{"missing-empty-bucket", "wrong-width", "wrong-offset", "missing-drain", "missing-count", "missing-histogram", "forged-quantile", "unknown-outside-failed", "histogram-overflow", "negative-offset"} {
		t.Run(mutation, func(t *testing.T) {
			changed := options
			changed.Buckets = append([]TimeBucket(nil), options.Buckets...)
			switch mutation {
			case "missing-empty-bucket":
				changed.Buckets = changed.Buckets[1:]
			case "wrong-width":
				changed.Buckets[2].WidthNS++
			case "wrong-offset":
				changed.Buckets[1].OffsetNS++
			case "missing-drain":
				changed.Buckets = changed.Buckets[:3]
			case "missing-count":
				changed.Buckets[0].Offered--
			case "missing-histogram":
				changed.Buckets[0].Buckets = [256]uint64{}
			case "forged-quantile":
				changed.Buckets[0].Latency.P99NS = 0
			case "unknown-outside-failed":
				changed.Buckets[0].Unknown = 1
			case "histogram-overflow":
				changed.Buckets[0].Buckets[0] = math.MaxUint64
			case "negative-offset":
				timeBucketAt(changed.Buckets, -1)
			}
			if err := auditTimeBuckets(changed); err == nil {
				t.Fatal("incomplete or inconsistent temporal evidence accepted", mutation)
			}
		})
	}
	if newTimeBuckets(duration, false) != nil {
		t.Fatal("warmup retained timed-only histograms")
	}
}

func TestTimeBucketsMergePageSamplesWithoutAveragingQuantiles(t *testing.T) {
	duration := int64(time.Second)
	first, second := newTimeBuckets(duration, true), newTimeBuckets(duration, true)
	first[0].Offered, second[0].Offered = 10, 10
	for range 9 {
		event := timeBucketOutcome{OffsetNS: 1, Latency: time.Millisecond, Items: 1}
		recordTimeBucketOutcome(first, event)
	}
	first[0].Rejected = 1
	for range 10 {
		event := timeBucketOutcome{OffsetNS: 1, Latency: 200 * time.Millisecond, Items: 1}
		recordTimeBucketOutcome(second, event)
	}
	pooled := newTimeBuckets(duration, true)
	mergeTimeBuckets(pooled, first)
	mergeTimeBuckets(pooled, second)
	finishTimeBuckets(pooled)
	var latency histogram
	for range 9 {
		latency.add(time.Millisecond)
	}
	for range 10 {
		latency.add(200 * time.Millisecond)
	}
	totals := TimeBucket{Offered: 20, Rejected: 1, Completed: 19}
	options := timeBucketAudit{Buckets: pooled, DurationNS: duration, ItemsPerCall: 1, Totals: totals, Latency: latency.summary(), Histogram: latency.buckets}
	if err := auditTimeBuckets(options); err != nil || pooled[0].Latency.P50NS < int64(200*time.Millisecond) {
		t.Fatal("pooled time bucket averaged per-worker quantiles or lost rejection", err, pooled[0])
	}
}

func TestSingleRequestTimeBucketsRetainCompletionTailAndRejectOmission(t *testing.T) {
	dataset := singleDataset(t)
	executor := &singleOnlyExecutor{}
	executor.delay = 200 * time.Millisecond
	command := clientCommand{Phase: "timed", StartUnixNS: time.Now().Add(5 * time.Millisecond).UnixNano(), DurationNS: int64(100 * time.Millisecond)}
	options := singleMeasureOptions{Executor: executor, State: newDurationState(dataset, 8, 1, 100), Threads: 1, Timeout: time.Second, Command: command}
	result := measureSingleRequests(context.Background(), options)
	if result.Requests != 1 || len(result.TimeBuckets) != 2 || result.TimeBuckets[0].Offered != 1 || result.TimeBuckets[0].Completed != 0 || result.TimeBuckets[1].Completed != 1 {
		t.Fatal("completed work after phase end was attributed to a regular bucket", result)
	}
	config := clientConfig{Path: executor.Name(), Threads: 1, TotalWorkers: 8}
	reply := clientReply{Phase: "timed", PID: result.PID, Result: &result}
	if err := validateClientReply(reply, "timed", config, result.PID); err != nil {
		t.Fatal("honest completion-tail receipt rejected", err)
	}
	result.TimeBuckets = nil
	if err := validateClientReply(reply, "timed", config, result.PID); err == nil {
		t.Fatal("owned child omitted temporal evidence without being rejected")
	}
}
