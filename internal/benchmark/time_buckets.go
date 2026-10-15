package benchmark

import (
	"errors"
	"math"
	"math/bits"
	"reflect"
	"time"
)

const timeBucketWidthNS = int64(10 * time.Second)

// Regular buckets use completion time relative to the scheduled phase start.
// Offered/rejected arrivals use their scheduled time instead. The final bucket
// retains every completion at or after phase end; width zero means an open tail.
// Failed includes unknown outcomes; Unknown and TimedOut are subsets of Failed.
// Counts use the phase's business units; latency samples count calls/pages.
type TimeBucket struct {
	OffsetNS       int64       `json:"offset_ns"`
	WidthNS        int64       `json:"width_ns"`
	Drain          bool        `json:"drain"`
	Offered        uint64      `json:"offered"`
	Rejected       uint64      `json:"rejected"`
	Completed      uint64      `json:"completed"`
	Failed         uint64      `json:"failed"`
	Unknown        uint64      `json:"unknown"`
	TimedOut       uint64      `json:"timed_out"`
	InvalidOffsets uint64      `json:"invalid_offsets"`
	Latency        Latency     `json:"latency"`
	Buckets        [256]uint64 `json:"latency_histogram_buckets"`
}

type timeBucketOutcome struct {
	OffsetNS                  int64
	Latency                   time.Duration
	Items                     uint64
	Failed, Unknown, TimedOut bool
}

func newTimeBuckets(durationNS int64, timed bool) []TimeBucket {
	if !timed {
		return nil
	}
	regular := int((durationNS + timeBucketWidthNS - 1) / timeBucketWidthNS)
	buckets := make([]TimeBucket, regular+1)
	for index := range regular {
		offset := int64(index) * timeBucketWidthNS
		buckets[index].OffsetNS = offset
		buckets[index].WidthNS = min(timeBucketWidthNS, durationNS-offset)
	}
	buckets[regular].OffsetNS = durationNS
	buckets[regular].Drain = true
	return buckets
}

func timeBucketAt(buckets []TimeBucket, offsetNS int64) *TimeBucket {
	if len(buckets) == 0 {
		return nil
	}
	if offsetNS < 0 {
		buckets[0].InvalidOffsets++
	}
	return &buckets[timeBucketIndex(buckets, offsetNS)]
}

func timeBucketIndex(buckets []TimeBucket, offsetNS int64) int {
	if offsetNS < 0 {
		return 0
	}
	if offsetNS >= buckets[len(buckets)-1].OffsetNS {
		return len(buckets) - 1
	}
	return int(offsetNS / timeBucketWidthNS)
}

func recordTimeBucketOutcome(buckets []TimeBucket, outcome timeBucketOutcome) {
	bucket := timeBucketAt(buckets, outcome.OffsetNS)
	if bucket == nil {
		return
	}
	if outcome.Latency < 0 {
		bucket.InvalidOffsets++
	}
	if outcome.Failed {
		bucket.Failed += outcome.Items
	} else {
		bucket.Completed += outcome.Items
	}
	if outcome.Unknown {
		bucket.Unknown += outcome.Items
	}
	if outcome.TimedOut {
		bucket.TimedOut += outcome.Items
	}
	ns := max(int64(outcome.Latency), 0)
	bucket.Buckets[histogramBucketIndex(ns)]++
	bucket.Latency.Samples++
	bucket.Latency.MaxNS = max(bucket.Latency.MaxNS, ns)
}

func finishTimeBuckets(buckets []TimeBucket) {
	for index := range buckets {
		bucket := &buckets[index]
		hist := histogram{buckets: bucket.Buckets, count: bucket.Latency.Samples, max: bucket.Latency.MaxNS}
		bucket.Latency = hist.summary()
	}
}

func mergeTimeBuckets(target, source []TimeBucket) {
	for index := range source {
		to, from := &target[index], source[index]
		to.Offered += from.Offered
		to.Rejected += from.Rejected
		to.Completed += from.Completed
		to.Failed += from.Failed
		to.Unknown += from.Unknown
		to.TimedOut += from.TimedOut
		to.InvalidOffsets += from.InvalidOffsets
		to.Latency.Samples += from.Latency.Samples
		to.Latency.MaxNS = max(to.Latency.MaxNS, from.Latency.MaxNS)
		for bin, count := range from.Buckets {
			to.Buckets[bin] += count
		}
	}
}

type timeBucketAudit struct {
	Buckets      []TimeBucket
	DurationNS   int64
	ItemsPerCall uint64
	Totals       TimeBucket
	Latency      Latency
	Histogram    [256]uint64
}

func auditTimeBuckets(options timeBucketAudit) error {
	if options.DurationNS <= 0 || options.DurationNS > int64(10*time.Minute) || options.ItemsPerCall == 0 || options.ItemsPerCall > 32 {
		return errors.New("invalid time-bucket phase duration or call size")
	}
	expected := newTimeBuckets(options.DurationNS, true)
	if len(options.Buckets) != len(expected) {
		return errors.New("missing regular or drain time bucket")
	}
	var total TimeBucket
	var pooled histogram
	for index, bucket := range options.Buckets {
		layout := expected[index]
		if bucket.OffsetNS != layout.OffsetNS || bucket.WidthNS != layout.WidthNS || bucket.Drain != layout.Drain || bucket.InvalidOffsets != 0 {
			return errors.New("time-bucket layout or event offset invalid")
		}
		if bucket.Failed > math.MaxUint64-bucket.Completed || bucket.Unknown > bucket.Failed || bucket.TimedOut > bucket.Failed || bucket.Latency.Samples > math.MaxUint64/options.ItemsPerCall || bucket.Completed+bucket.Failed != bucket.Latency.Samples*options.ItemsPerCall {
			return errors.New("time-bucket completion/status counts inconsistent")
		}
		var samples uint64
		for bin, count := range bucket.Buckets {
			if count > math.MaxUint64-samples || count > math.MaxUint64-pooled.buckets[bin] {
				return errors.New("time-bucket histogram overflow")
			}
			samples += count
			pooled.buckets[bin] += count
		}
		hist := histogram{buckets: bucket.Buckets, count: samples, max: bucket.Latency.MaxNS}
		if samples != bucket.Latency.Samples || bucket.Latency.MaxNS < 0 || samples == 0 && bucket.Latency.MaxNS != 0 || !reflect.DeepEqual(bucket.Latency, hist.summary()) {
			return errors.New("time-bucket latency histogram inconsistent")
		}
		pooled.max = max(pooled.max, bucket.Latency.MaxNS)
		if samples > math.MaxUint64-pooled.count {
			return errors.New("time-bucket sample count overflow")
		}
		pooled.count += samples
		pairs := [][2]*uint64{{&total.Offered, &bucket.Offered}, {&total.Rejected, &bucket.Rejected}, {&total.Completed, &bucket.Completed}, {&total.Failed, &bucket.Failed}, {&total.Unknown, &bucket.Unknown}, {&total.TimedOut, &bucket.TimedOut}}
		for _, pair := range pairs {
			if *pair[1] > math.MaxUint64-*pair[0] {
				return errors.New("time-bucket cumulative count overflow")
			}
			*pair[0] += *pair[1]
		}
	}
	want := options.Totals
	if total.Offered != want.Offered || total.Rejected != want.Rejected || total.Completed != want.Completed || total.Failed != want.Failed || total.Unknown != want.Unknown || total.TimedOut != want.TimedOut || total.Failed > math.MaxUint64-total.Completed || total.Rejected > math.MaxUint64-total.Completed-total.Failed || total.Offered != total.Completed+total.Failed+total.Rejected {
		return errors.New("time-bucket totals differ from the complete phase")
	}
	if pooled.buckets != options.Histogram || !reflect.DeepEqual(pooled.summary(), options.Latency) {
		return errors.New("time-bucket pooled latency differs from complete phase")
	}
	return nil
}

func auditCrawlerTimeBuckets(worker crawlerWorkerResult, durationNS int64, pageSize int) error {
	totals := TimeBucket{Offered: worker.Offered, Rejected: worker.Rejected, Completed: worker.Completed, Failed: worker.Failed, Unknown: worker.Indeterminate, TimedOut: worker.TimedOut}
	options := timeBucketAudit{Buckets: worker.TimeBuckets, DurationNS: durationNS, ItemsPerCall: uint64(pageSize), Totals: totals, Latency: worker.Latency, Histogram: worker.Buckets}
	return auditTimeBuckets(options)
}

func histogramBucketIndex(ns int64) int {
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
	return index
}
