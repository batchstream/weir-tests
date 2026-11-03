package soak

import (
	"context"
	"errors"
	"io"
	"math/rand/v2"
	"sync"
	"time"

	weir "github.com/batchstream/weir-go"
)

// records is the part of the Weir SDK a worker calls.
type records interface {
	Put(ctx context.Context, options weir.WriteOptions) (*weir.MutationResult, error)
	AtomicTransform(ctx context.Context, options weir.AtomicTransformOptions) (*weir.MutationResult, error)
	Mutate(ctx context.Context, options weir.MutateOptions) ([]*weir.MutationResult, error)
	MutateStream(ctx context.Context, options weir.MutateStreamOptions) error
	ReadOne(ctx context.Context, options weir.ReadOneOptions) (*weir.ReadResult, error)
	Read(ctx context.Context, options weir.ReadOptions) ([]*weir.ReadResult, error)
	ReadStream(ctx context.Context, options weir.ReadStreamOptions) error
	Native(ctx context.Context, options weir.NativeOptions) (*weir.NativeResult, error)
}

// worker writes and reads its own keys through the SDK calls of its profile,
// one turn at a time, so each key has at most one write in flight.
type worker struct {
	backend        string
	storeName      string
	store          store
	client         records
	keys           []*keyState
	padding        string
	profile        string
	batch          int
	streamLifetime time.Duration
	readPercent    int
	verifyPercent  int
	timeout        time.Duration
	lateWindow     time.Duration
	random         *rand.Rand
	stats          *stats
	violations     *violationLog
	stop           <-chan struct{}
	cursor         int
}

func (w *worker) run(tokens <-chan struct{}) {
	chosen := profiles[w.profile]
	reading := false
	for {
		if chosen.alternate && w.readPercent > 0 && w.readPercent < 100 {
			reading = !reading
		} else {
			reading = w.random.IntN(100) < w.readPercent
		}
		next := chosen.write
		if reading {
			next = chosen.read
		}
		if !next(w, tokens) {
			return
		}
	}
}

// take waits for count tokens and reports false if the run stops first.
func (w *worker) take(tokens <-chan struct{}, count int) bool {
	for range count {
		select {
		case <-w.stop:
			return false
		case <-tokens:
		}
	}
	return true
}

func (w *worker) nextKey() *keyState {
	key := w.keys[w.cursor%len(w.keys)]
	w.cursor++
	return key
}

func (w *worker) nextKeys(count int) []*keyState {
	keys := make([]*keyState, min(count, len(w.keys)))
	for index := range keys {
		keys[index] = w.nextKey()
	}
	return keys
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

// pendingWrite is one issued write whose outcome is not known yet.
type pendingWrite struct {
	api    string
	key    *keyState
	seq    int64
	issued time.Time
	retry  bool
}

// prepare gives key a fresh sequence and its document.
func (w *worker) prepare(api string, key *keyState, retry bool) (pendingWrite, *weir.Document, error) {
	key.issued++
	pending := pendingWrite{api: api, key: key, seq: key.issued, issued: time.Now(), retry: retry}
	data, err := w.store.encode(key.id, pending.seq, w.padding)
	if err != nil {
		w.violations.add(w.backend, "encode", key, pending.seq)
		return pending, nil, err
	}
	document := &weir.Document{ContentType: w.store.contentType(), Data: data}
	return pending, document, nil
}

// settle records a write's reported outcome and what its key may now hold.
// A nil result leaves the outcome unknown.
func (w *worker) settle(pending pendingWrite, result *weir.MutationResult, elapsed time.Duration) outcome {
	reported := classify(result)
	failure := result.GetFailure()
	w.stats.write(pending.api, reported, failure != nil, elapsed, pending.retry)
	w.stats.failure(pending.api, failure)
	switch reported {
	case outcomeApplied:
		pending.key.settle(pending.seq, pending.issued, w.lateWindow)
	case outcomeUnknown:
		write := unknownWrite{seq: pending.seq, issued: pending.issued}
		pending.key.unknown = append(pending.key.unknown, write)
	}
	return reported
}

// checkAfter names the direct database check a write's outcome calls for, if
// any: one after every refusal and for a sample of APPLIED writes.
func (w *worker) checkAfter(reported outcome) string {
	switch reported {
	case outcomeApplied:
		if w.random.IntN(100) < w.verifyPercent {
			return "applied"
		}
	case outcomeNotApplied, outcomeNotStarted:
		return "refused"
	}
	return ""
}

// checkDatabase runs the check a write's outcome calls for. Nothing may be in
// flight for the key.
func (w *worker) checkDatabase(key *keyState, reported outcome) {
	if after := w.checkAfter(reported); after != "" {
		w.verify(key, after)
	}
}

// writeCall sends one write of document to key.
type writeCall func(ctx context.Context, key *keyState, document *weir.Document) (*weir.MutationResult, error)

// write issues one write and, while Weir reports it took no effect, checks
// the database agrees and retries with a fresh sequence.
func (w *worker) write(api string, key *keyState, call writeCall) {
	for attempt := 0; ; attempt++ {
		pending, document, err := w.prepare(api, key, attempt > 0)
		if err != nil {
			return
		}
		ctx, cancel := context.WithTimeout(context.Background(), w.timeout)
		result, err := call(ctx, key, document)
		elapsed := time.Since(pending.issued)
		cancel()
		w.stats.called(api)
		// An SDK error without a result leaves the outcome unknown.
		if result == nil && err != nil {
			w.stats.sdkError(api, err)
		}
		reported := w.settle(pending, result, elapsed)
		w.checkDatabase(key, reported)
		if reported == outcomeApplied || reported == outcomeUnknown {
			return
		}
		if !w.pause(backoff(attempt)) {
			w.stats.abandoned()
			return
		}
	}
}

func (w *worker) putCall(options *weir.MutationOptions) writeCall {
	call := func(ctx context.Context, key *keyState, document *weir.Document) (*weir.MutationResult, error) {
		request := &weir.WriteRequest{Resource: w.store.resource(key.id), Document: document, Options: options}
		writeOptions := weir.WriteOptions{StoreName: w.storeName, Request: request}
		return w.client.Put(ctx, writeOptions)
	}
	return call
}

// luaReplace makes a Lua transform store its input, so the document it leaves
// has the shape every other write gives it.
const luaReplace = `return function(current, incoming) return incoming end`

func (w *worker) luaCall(ctx context.Context, key *keyState, document *weir.Document) (*weir.MutationResult, error) {
	program := &weir.LuaTransform{Source: []byte(luaReplace), Input: document}
	request := &weir.AtomicTransformRequest{Resource: w.store.resource(key.id), Lua: program}
	options := weir.AtomicTransformOptions{StoreName: w.storeName, Request: request}
	return w.client.AtomicTransform(ctx, options)
}

func (w *worker) putTurn(tokens <-chan struct{}) bool {
	if !w.take(tokens, 1) {
		return false
	}
	w.write(apiPut, w.nextKey(), w.putCall(nil))
	return true
}

func (w *worker) visibleTurn(tokens <-chan struct{}) bool {
	if !w.take(tokens, 1) {
		return false
	}
	visible := &weir.MutationOptions{Visibility: weir.MutationQueryVisible}
	w.write(apiPutVisible, w.nextKey(), w.putCall(visible))
	return true
}

func (w *worker) luaTurn(tokens <-chan struct{}) bool {
	if !w.take(tokens, 1) {
		return false
	}
	w.write(apiLua, w.nextKey(), w.luaCall)
	return true
}

// mutateTurn writes several keys in one Mutate call. Refused records are not
// sent again; the workload's next turn writes them with a fresh sequence.
func (w *worker) mutateTurn(tokens <-chan struct{}) bool {
	keys := w.nextKeys(w.batch)
	if !w.take(tokens, len(keys)) {
		return false
	}
	pending := make([]pendingWrite, 0, len(keys))
	requests := make([]*weir.MutateRequest, 0, len(keys))
	for _, key := range keys {
		write, document, err := w.prepare(apiMutate, key, false)
		if err != nil {
			return true
		}
		request := &weir.MutateRequest{Resource: w.store.resource(key.id), Action: weir.MutationPut, Document: document}
		pending = append(pending, write)
		requests = append(requests, request)
	}
	ctx, cancel := context.WithTimeout(context.Background(), w.timeout)
	defer cancel()
	started := time.Now()
	options := weir.MutateOptions{StoreName: w.storeName, Requests: requests}
	results, err := w.client.Mutate(ctx, options)
	elapsed := time.Since(started)
	w.stats.called(apiMutate)
	if err != nil {
		w.stats.sdkError(apiMutate, err)
	}
	for index, write := range pending {
		var result *weir.MutationResult
		if index < len(results) {
			result = results[index]
		}
		w.checkDatabase(write.key, w.settle(write, result, elapsed))
	}
	return true
}

// streamTurn is one stream a worker keeps open for its lifetime, sending a
// record per token, as an application that holds a stream does. A credit is one
// more record that may be in flight, which never exceeds the worker's keys.
type streamTurn struct {
	worker   *worker
	api      string
	tokens   <-chan struct{}
	lifetime context.Context
	credits  chan struct{}
	// checks holds keys awaiting a direct database check, which stay busy and
	// keep their credit until it ran, so no later write can hide what it finds.
	checks  chan streamCheck
	checked chan struct{}
	mu      sync.Mutex
	started bool
	busy    map[*keyState]bool
	sent    uint64
}

type streamCheck struct {
	key   *keyState
	after string
}

// streamWindow is the most records a worker's stream keeps in flight, the most
// unconsumed results the SDK allows one stream.
const streamWindow = 32

func (w *worker) newStreamTurn(api string, tokens <-chan struct{}, lifetime context.Context) *streamTurn {
	turn := &streamTurn{worker: w, api: api, tokens: tokens, lifetime: lifetime, busy: make(map[*keyState]bool)}
	turn.credits = make(chan struct{}, min(streamWindow, len(w.keys)))
	for range cap(turn.credits) {
		turn.credits <- struct{}{}
	}
	return turn
}

// reserve waits for room and a token for the next record, the first one
// without waiting because the worker took its token before opening the stream,
// and returns the key it goes to. io.EOF ends the stream's input.
func (t *streamTurn) reserve(ctx context.Context) (*keyState, uint64, error) {
	t.mu.Lock()
	first := !t.started
	t.started = true
	t.mu.Unlock()
	select {
	case <-ctx.Done():
		return nil, 0, ctx.Err()
	case <-t.credits:
	}
	if !first {
		select {
		case <-ctx.Done():
			return nil, 0, ctx.Err()
		case <-t.worker.stop:
			return nil, 0, io.EOF
		case <-t.lifetime.Done():
			return nil, 0, io.EOF
		case <-t.tokens:
		}
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	key := t.worker.nextKey()
	for t.busy[key] {
		key = t.worker.nextKey()
	}
	t.busy[key] = true
	t.sent++
	return key, t.sent, nil
}

// release returns a record's room once its result arrived.
func (t *streamTurn) release(key *keyState) {
	delete(t.busy, key)
	t.credits <- struct{}{}
}

// startChecks runs direct database checks apart from Consume, which must return
// promptly. A stream's checks wait for one another, as a worker's single writes
// wait for theirs.
func (t *streamTurn) startChecks() {
	t.checks = make(chan streamCheck, cap(t.credits))
	t.checked = make(chan struct{})
	go func() {
		defer close(t.checked)
		for check := range t.checks {
			t.worker.verify(check.key, check.after)
			t.mu.Lock()
			t.release(check.key)
			t.mu.Unlock()
		}
	}()
}

// endChecks waits for the checks of a stream whose Consume has returned for good.
func (t *streamTurn) endChecks() {
	close(t.checks)
	<-t.checked
}

// settled queues the check a record's outcome calls for, or returns its room.
// Checks never outnumber the credits, so this cannot block.
func (t *streamTurn) settled(key *keyState, reported outcome) {
	after := t.worker.checkAfter(reported)
	if after == "" {
		t.release(key)
		return
	}
	check := streamCheck{key: key, after: after}
	t.checks <- check
}

// settleStreamWrite settles a stream's record. It has no deadline of its own, so
// one whose outcome stays unknown may land as late after the stream gave up on
// it as a single write may after being issued.
func (w *worker) settleStreamWrite(write pendingWrite, result *weir.MutationResult) outcome {
	elapsed := time.Since(write.issued)
	if classify(result) == outcomeUnknown {
		write.issued = time.Now()
	}
	return w.settle(write, result, elapsed)
}

func (w *worker) mutateStreamTurn(tokens <-chan struct{}) bool {
	if !w.take(tokens, 1) {
		return false
	}
	ctx, cancel := context.WithTimeout(context.Background(), w.streamLifetime+w.timeout)
	defer cancel()
	lifetime, ended := context.WithTimeout(ctx, w.streamLifetime)
	defer ended()
	turn := w.newStreamTurn(apiMutateStream, tokens, lifetime)
	turn.startChecks()
	flight := make(map[uint64]pendingWrite)
	options := weir.MutateStreamOptions{StoreName: w.storeName}
	options.Next = func(ctx context.Context) (*weir.MutateRequest, error) {
		key, index, err := turn.reserve(ctx)
		if err != nil {
			return nil, err
		}
		turn.mu.Lock()
		defer turn.mu.Unlock()
		write, document, err := w.prepare(apiMutateStream, key, false)
		if err != nil {
			return nil, err
		}
		flight[index] = write
		request := &weir.MutateRequest{Resource: w.store.resource(key.id), Action: weir.MutationPut, Document: document}
		return request, nil
	}
	options.Consume = func(_ context.Context, index uint64, result *weir.MutationResult) error {
		turn.mu.Lock()
		defer turn.mu.Unlock()
		write := flight[index]
		delete(flight, index)
		turn.settled(write.key, w.settleStreamWrite(write, result))
		return nil
	}
	err := w.client.MutateStream(ctx, options)
	w.stats.called(apiMutateStream)
	if err != nil {
		w.stats.sdkError(apiMutateStream, err)
	}
	turn.mu.Lock()
	// Records without a result stay unknown.
	for _, write := range flight {
		w.settleStreamWrite(write, nil)
	}
	turn.mu.Unlock()
	turn.endChecks()
	return true
}

func (w *worker) readOneTurn(tokens <-chan struct{}) bool {
	if !w.take(tokens, 1) {
		return false
	}
	w.read(w.nextKey())
	return true
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
	w.stats.called(apiReadOne)
	if result == nil && err != nil {
		w.stats.sdkError(apiReadOne, err)
	}
	w.checkRead(apiReadOne, key, result, elapsed)
}

// checkRead records one read's result, which must return a sequence the
// reported outcomes allow. A nil result failed.
func (w *worker) checkRead(api string, key *keyState, result *weir.ReadResult, elapsed time.Duration) {
	failed := result == nil || result.Failure != nil || !result.Missing && result.Document == nil
	w.stats.read(api, failed, elapsed)
	if result != nil {
		w.stats.failure(api, result.Failure)
	}
	if failed {
		return
	}
	var observed int64
	if !result.Missing {
		var err error
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

func (w *worker) readBatchTurn(tokens <-chan struct{}) bool {
	keys := w.nextKeys(w.batch)
	if !w.take(tokens, len(keys)) {
		return false
	}
	requests := make([]*weir.ReadRequest, len(keys))
	for index, key := range keys {
		requests[index] = &weir.ReadRequest{Resource: w.store.resource(key.id)}
	}
	ctx, cancel := context.WithTimeout(context.Background(), w.timeout)
	defer cancel()
	started := time.Now()
	options := weir.ReadOptions{StoreName: w.storeName, Requests: requests}
	results, err := w.client.Read(ctx, options)
	elapsed := time.Since(started)
	w.stats.called(apiRead)
	if err != nil {
		w.stats.sdkError(apiRead, err)
	}
	for index, key := range keys {
		var result *weir.ReadResult
		if index < len(results) {
			result = results[index]
		}
		w.checkRead(apiRead, key, result, elapsed)
	}
	return true
}

func (w *worker) readStreamTurn(tokens <-chan struct{}) bool {
	if !w.take(tokens, 1) {
		return false
	}
	ctx, cancel := context.WithTimeout(context.Background(), w.streamLifetime+w.timeout)
	defer cancel()
	lifetime, ended := context.WithTimeout(ctx, w.streamLifetime)
	defer ended()
	turn := w.newStreamTurn(apiReadStream, tokens, lifetime)
	type pendingRead struct {
		key    *keyState
		issued time.Time
	}
	flight := make(map[uint64]pendingRead)
	options := weir.ReadStreamOptions{StoreName: w.storeName}
	options.Next = func(ctx context.Context) (*weir.ReadRequest, error) {
		key, index, err := turn.reserve(ctx)
		if err != nil {
			return nil, err
		}
		turn.mu.Lock()
		defer turn.mu.Unlock()
		read := pendingRead{key: key, issued: time.Now()}
		flight[index] = read
		request := &weir.ReadRequest{Resource: w.store.resource(key.id)}
		return request, nil
	}
	options.Consume = func(_ context.Context, index uint64, result *weir.ReadResult) error {
		turn.mu.Lock()
		defer turn.mu.Unlock()
		read := flight[index]
		delete(flight, index)
		w.checkRead(apiReadStream, read.key, result, time.Since(read.issued))
		turn.release(read.key)
		return nil
	}
	err := w.client.ReadStream(ctx, options)
	w.stats.called(apiReadStream)
	if err != nil {
		w.stats.sdkError(apiReadStream, err)
	}
	turn.mu.Lock()
	defer turn.mu.Unlock()
	for _, read := range flight {
		w.checkRead(apiReadStream, read.key, nil, time.Since(read.issued))
	}
	return true
}

// nativeTurn reads a key with the backend's own command through Native. A
// complete response must return a sequence the reported outcomes allow.
func (w *worker) nativeTurn(tokens <-chan struct{}) bool {
	if !w.take(tokens, 1) {
		return false
	}
	key := w.nextKey()
	request, err := w.store.nativeRead(key.id)
	if err != nil {
		w.violations.add(w.backend, "encode-native", key, 0)
		return true
	}
	var body []byte
	options := weir.NativeOptions{StoreName: w.storeName, Request: request}
	options.Consume = func(_ context.Context, _ *weir.NativeResponse, chunk []byte) error {
		body = append(body, chunk...)
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), w.timeout)
	defer cancel()
	started := time.Now()
	result, err := w.client.Native(ctx, options)
	elapsed := time.Since(started)
	w.stats.called(apiNative)
	if err != nil {
		w.stats.sdkError(apiNative, err)
	}
	complete := err == nil && result != nil && result.Failure == nil && result.Completion == weir.NativeResponseComplete
	if result != nil {
		w.stats.failure(apiNative, result.Failure)
	}
	if !complete {
		w.stats.read(apiNative, true, elapsed)
		return true
	}
	observed, err := w.store.nativeSeq(result.Response, body)
	w.stats.read(apiNative, errors.Is(err, errBackendRefused), elapsed)
	if errors.Is(err, errBackendRefused) {
		w.stats.backendRefusal(apiNative, err)
		return true
	}
	if err != nil {
		w.violations.add(w.backend, "weir-native-undecodable", key, 0)
		return true
	}
	if !key.allows(observed) {
		w.violations.add(w.backend, "weir-native", key, observed)
	}
	return true
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
