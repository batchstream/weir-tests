package soak

import (
	"math"
	"sync"
	"time"
)

// histogram counts durations in buckets that grow by 2^(1/4) from 100µs.
type histogram struct {
	counts [96]uint64
	total  uint64
}

const histogramBase = 100 * time.Microsecond

func (h *histogram) add(elapsed time.Duration) {
	bucket := 0
	if elapsed > histogramBase {
		bucket = int(math.Ceil(4 * math.Log2(float64(elapsed)/float64(histogramBase))))
	}
	bucket = min(bucket, len(h.counts)-1)
	h.counts[bucket]++
	h.total++
}

// quantile returns the upper bound of the bucket holding quantile q, in
// milliseconds, or 0 without samples.
func (h *histogram) quantile(q float64) float64 {
	if h.total == 0 {
		return 0
	}
	rank := uint64(math.Ceil(q * float64(h.total)))
	var seen uint64
	for bucket, count := range h.counts {
		seen += count
		if seen >= rank {
			bound := float64(histogramBase) * math.Pow(2, float64(bucket)/4)
			return bound / float64(time.Millisecond)
		}
	}
	return 0
}

type outcome int

const (
	outcomeApplied outcome = iota
	outcomeUnknown
	outcomeNotApplied
	outcomeNotStarted
)

// Counters summarize one backend's operations over an interval or a run.
type Counters struct {
	Writes            uint64  `json:"writes"`
	Retries           uint64  `json:"retries"`
	Applied           uint64  `json:"applied"`
	AppliedWithFault  uint64  `json:"applied_with_failure"`
	Unknown           uint64  `json:"unknown"`
	NotApplied        uint64  `json:"not_applied"`
	NotStarted        uint64  `json:"not_started"`
	Abandoned         uint64  `json:"abandoned"`
	SDKErrors         uint64  `json:"sdk_errors"`
	LastSDKError      string  `json:"last_sdk_error,omitempty"`
	Reads             uint64  `json:"reads"`
	ReadFailures      uint64  `json:"read_failures"`
	Verified          uint64  `json:"verified"`
	VerifyUnavailable uint64  `json:"verify_unavailable"`
	WriteP50          float64 `json:"write_p50_ms"`
	WriteP99          float64 `json:"write_p99_ms"`
	WriteMax          float64 `json:"write_max_ms"`
	ReadP50           float64 `json:"read_p50_ms"`
	ReadP99           float64 `json:"read_p99_ms"`
}

type period struct {
	counters     Counters
	writeLatency histogram
	readLatency  histogram
	writeMax     time.Duration
}

func (p *period) report() Counters {
	counters := p.counters
	counters.WriteP50 = p.writeLatency.quantile(0.50)
	counters.WriteP99 = p.writeLatency.quantile(0.99)
	counters.WriteMax = float64(p.writeMax) / float64(time.Millisecond)
	counters.ReadP50 = p.readLatency.quantile(0.50)
	counters.ReadP99 = p.readLatency.quantile(0.99)
	return counters
}

// stats accumulates one backend's operations for the current interval and
// the whole run.
type stats struct {
	mu       sync.Mutex
	interval period
	run      period
}

func (s *stats) record(update func(*period)) {
	s.mu.Lock()
	defer s.mu.Unlock()
	update(&s.interval)
	update(&s.run)
}

func (s *stats) write(result outcome, failed bool, elapsed time.Duration, retry bool) {
	s.record(func(p *period) {
		p.counters.Writes++
		if retry {
			p.counters.Retries++
		}
		switch result {
		case outcomeApplied:
			p.counters.Applied++
			if failed {
				p.counters.AppliedWithFault++
			}
		case outcomeUnknown:
			p.counters.Unknown++
		case outcomeNotApplied:
			p.counters.NotApplied++
		case outcomeNotStarted:
			p.counters.NotStarted++
		}
		p.writeLatency.add(elapsed)
		p.writeMax = max(p.writeMax, elapsed)
	})
}

// sdkError counts a call the SDK ended with an error instead of a result.
func (s *stats) sdkError(err error) {
	s.record(func(p *period) {
		p.counters.SDKErrors++
		p.counters.LastSDKError = err.Error()
	})
}

func (s *stats) read(failed bool, elapsed time.Duration) {
	s.record(func(p *period) {
		p.counters.Reads++
		if failed {
			p.counters.ReadFailures++
			return
		}
		p.readLatency.add(elapsed)
	})
}

func (s *stats) verified(available bool) {
	s.record(func(p *period) {
		if available {
			p.counters.Verified++
			return
		}
		p.counters.VerifyUnavailable++
	})
}

func (s *stats) abandoned() {
	s.record(func(p *period) { p.counters.Abandoned++ })
}

// rotate returns the finished interval and starts a new one.
func (s *stats) rotate() Counters {
	s.mu.Lock()
	defer s.mu.Unlock()
	counters := s.interval.report()
	s.interval = period{}
	return counters
}

func (s *stats) total() Counters {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.run.report()
}
