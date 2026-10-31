package soak

import (
	"context"
	"math/rand/v2"
	"time"

	weir "github.com/batchstream/weir-go"
)

// records is the part of the Weir SDK a worker calls.
type records interface {
	Put(ctx context.Context, options weir.WriteOptions) (*weir.MutationResult, error)
	ReadOne(ctx context.Context, options weir.ReadOneOptions) (*weir.ReadResult, error)
}

// worker writes and reads its own keys one operation at a time, so each key
// has at most one write in flight.
type worker struct {
	backend       string
	storeName     string
	store         store
	client        records
	keys          []*keyState
	padding       string
	readPercent   int
	verifyPercent int
	timeout       time.Duration
	lateWindow    time.Duration
	random        *rand.Rand
	stats         *stats
	violations    *violationLog
	stop          <-chan struct{}
}

func (w *worker) run(tokens <-chan struct{}) {
	for next := 0; ; next++ {
		select {
		case <-w.stop:
			return
		case <-tokens:
		}
		key := w.keys[next%len(w.keys)]
		if w.random.IntN(100) < w.readPercent {
			w.read(key)
			continue
		}
		w.write(key)
	}
}

func classify(result *weir.MutationResult) outcome {
	switch result.GetOutcome() {
	case weir.MutationApplied:
		return outcomeApplied
	case weir.MutationNotApplied:
		return outcomeNotApplied
	case weir.MutationNotStarted:
		return outcomeNotStarted
	default:
		return outcomeUnknown
	}
}

// write issues one write and, while Weir reports it took no effect, checks
// the database agrees and retries with a fresh sequence.
func (w *worker) write(key *keyState) {
	for attempt := 0; ; attempt++ {
		key.issued++
		seq := key.issued
		data, err := w.store.encode(key.id, seq, w.padding)
		if err != nil {
			w.violations.add(w.backend, "encode", key, seq)
			return
		}
		document := &weir.Document{ContentType: w.store.contentType(), Data: data}
		request := &weir.WriteRequest{Resource: w.store.resource(key.id), Document: document}
		options := weir.WriteOptions{StoreName: w.storeName, Request: request}
		ctx, cancel := context.WithTimeout(context.Background(), w.timeout)
		started := time.Now()
		result, err := w.client.Put(ctx, options)
		elapsed := time.Since(started)
		cancel()
		// An SDK error without a result leaves the outcome unknown.
		if result == nil && err != nil {
			w.stats.sdkError(err)
		}
		reported := classify(result)
		w.stats.write(reported, result.GetFailure() != nil, elapsed, attempt > 0)
		switch reported {
		case outcomeApplied:
			key.settle(seq, started, w.lateWindow)
			if w.random.IntN(100) < w.verifyPercent {
				w.verify(key, "applied")
			}
			return
		case outcomeUnknown:
			write := unknownWrite{seq: seq, issued: started}
			key.unknown = append(key.unknown, write)
			return
		}
		w.verify(key, "refused")
		if !w.pause(backoff(attempt)) {
			w.stats.abandoned()
			return
		}
	}
}

// verify reads key directly from the database, which must hold a sequence
// the reported outcomes allow.
func (w *worker) verify(key *keyState, after string) {
	ctx, cancel := context.WithTimeout(context.Background(), w.timeout)
	defer cancel()
	observed, err := w.store.read(ctx, key.id)
	w.stats.verified(err == nil)
	if err == nil && !key.allows(observed) {
		w.violations.add(w.backend, "direct-after-"+after, key, observed)
	}
}

// read reads key through Weir; a successful read must return a sequence the
// reported outcomes allow.
func (w *worker) read(key *keyState) {
	request := &weir.ReadRequest{Resource: w.store.resource(key.id)}
	options := weir.ReadOneOptions{StoreName: w.storeName, Request: request}
	ctx, cancel := context.WithTimeout(context.Background(), w.timeout)
	started := time.Now()
	result, err := w.client.ReadOne(ctx, options)
	elapsed := time.Since(started)
	cancel()
	if result == nil && err != nil {
		w.stats.sdkError(err)
	}
	failed := err != nil || result == nil || result.Failure != nil || !result.Missing && result.Document == nil
	w.stats.read(failed, elapsed)
	if failed {
		return
	}
	var observed int64
	if !result.Missing {
		observed, err = w.store.decode(result.Document.Data)
		if err != nil || result.Document.ContentType != w.store.contentType() {
			w.violations.add(w.backend, "weir-read-undecodable", key, 0)
			return
		}
	}
	if !key.allows(observed) {
		w.violations.add(w.backend, "weir-read", key, observed)
	}
}

// pause waits for delay and reports false if the run stops first.
func (w *worker) pause(delay time.Duration) bool {
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-w.stop:
		return false
	case <-timer.C:
		return true
	}
}

func backoff(attempt int) time.Duration {
	return min(100*time.Millisecond<<min(attempt, 4), time.Second)
}

// pace sends rate tokens per second, dropping those no worker is ready for.
func pace(stop <-chan struct{}, rate float64, tokens chan<- struct{}) {
	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()
	var credit float64
	for {
		select {
		case <-stop:
			return
		case <-ticker.C:
		}
		for credit += rate / 100; credit >= 1; credit-- {
			select {
			case tokens <- struct{}{}:
			default:
			}
		}
	}
}
