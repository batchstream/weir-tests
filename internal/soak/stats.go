package soak

import (
	"context"
	"errors"
	"math"
	"sync"
	"time"

	weir "github.com/batchstream/weir-go"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
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

// Counters summarize one backend's operations, or one SDK call's, over an
// interval or a run. Calls counts SDK calls, so a stream or a batch of several
// records counts once. Timeouts counts calls and records that ended at a deadline.
type Counters struct {
	Calls             uint64  `json:"calls"`
	Writes            uint64  `json:"writes"`
	Retries           uint64  `json:"retries"`
	Applied           uint64  `json:"applied"`
	AppliedWithFault  uint64  `json:"applied_with_failure"`
	Unknown           uint64  `json:"unknown"`
	NotApplied        uint64  `json:"not_applied"`
	NotStarted        uint64  `json:"not_started"`
	Abandoned         uint64  `json:"abandoned"`
	SDKErrors         uint64  `json:"sdk_errors"`
	Timeouts          uint64  `json:"timeouts"`
	LastSDKError      string  `json:"last_sdk_error,omitempty"`
	LastFailure       string  `json:"last_failure,omitempty"`
	Reads             uint64  `json:"reads"`
	ReadFailures      uint64  `json:"read_failures"`
	Verified          uint64  `json:"verified"`
	VerifyUnavailable uint64  `json:"verify_unavailable"`
	WriteP50          float64 `json:"write_p50_ms"`
	WriteP99          float64 `json:"write_p99_ms"`
	WriteMax          float64 `json:"write_max_ms"`
	ReadP50           float64 `json:"read_p50_ms"`
	ReadP99           float64 `json:"read_p99_ms"`
	ReadMax           float64 `json:"read_max_ms"`
	// APIs breaks a backend's counters down by SDK call.
	APIs map[string]Counters `json:"apis,omitempty"`
}

type period struct {
	counters     Counters
	writeLatency histogram
	readLatency  histogram
	writeMax     time.Duration
	readMax      time.Duration
	apis         map[string]*period
}

// of returns the period of one SDK call.
func (p *period) of(api string) *period {
	if p.apis == nil {
		p.apis = make(map[string]*period)
	}
	sub, ok := p.apis[api]
	if !ok {
		sub = &period{}
		p.apis[api] = sub
	}
	return sub
}

func (p *period) report() Counters {
	counters := p.counters
	counters.WriteP50 = p.writeLatency.quantile(0.50)
	counters.WriteP99 = p.writeLatency.quantile(0.99)
	counters.WriteMax = float64(p.writeMax) / float64(time.Millisecond)
	counters.ReadP50 = p.readLatency.quantile(0.50)
	counters.ReadP99 = p.readLatency.quantile(0.99)
	counters.ReadMax = float64(p.readMax) / float64(time.Millisecond)
	if len(p.apis) > 0 {
		counters.APIs = make(map[string]Counters, len(p.apis))
		for api, sub := range p.apis {
			counters.APIs[api] = sub.report()
		}
	}
	return counters
}

// stats accumulates one backend's operations for the current interval and
// the whole run.
type stats struct {
	mu       sync.Mutex
	interval period
	run      period
}

// record updates the interval and the run, and with an api also that call's.
func (s *stats) record(api string, update func(*period)) {
	s.mu.Lock()
	defer s.mu.Unlock()
	update(&s.interval)
	update(&s.run)
	if api != "" {
		update(s.interval.of(api))
		update(s.run.of(api))
	}
}

func (s *stats) called(api string) {
	s.record(api, func(p *period) { p.counters.Calls++ })
}

func (s *stats) write(api string, result outcome, failed bool, elapsed time.Duration, retry bool) {
	s.record(api, func(p *period) {
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
func (s *stats) sdkError(api string, err error) {
	timeout := errors.Is(err, context.DeadlineExceeded) || status.Code(err) == codes.DeadlineExceeded
	s.record(api, func(p *period) {
		p.counters.SDKErrors++
		p.counters.LastSDKError = err.Error()
		if timeout {
			p.counters.Timeouts++
		}
	})
}

// failure keeps a record's Failure, if any, as evidence of why it did not succeed.
func (s *stats) failure(api string, failure *weir.Failure) {
	if failure == nil {
		return
	}
	s.record(api, func(p *period) {
		p.counters.LastFailure = failure.GetCode().String() + ": " + failure.GetMessage()
		if failure.GetCode() == weir.FailureDeadlineExceeded {
			p.counters.Timeouts++
		}
	})
}

// backendRefusal keeps why a backend answered a Native command with an error.
func (s *stats) backendRefusal(api string, err error) {
	s.record(api, func(p *period) { p.counters.LastFailure = err.Error() })
}

// read counts one read. Only successes enter the latency quantiles; the longest
// read includes failures, so a call that waited for its deadline shows.
func (s *stats) read(api string, failed bool, elapsed time.Duration) {
	s.record(api, func(p *period) {
		p.counters.Reads++
		p.readMax = max(p.readMax, elapsed)
		if failed {
			p.counters.ReadFailures++
			return
		}
		p.readLatency.add(elapsed)
	})
}

func (s *stats) verified(available bool) {
	s.record("", func(p *period) {
		if available {
			p.counters.Verified++
			return
		}
		p.counters.VerifyUnavailable++
	})
}

func (s *stats) abandoned() {
	s.record("", func(p *period) { p.counters.Abandoned++ })
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
