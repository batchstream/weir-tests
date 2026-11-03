package soak

import (
	"context"
	"errors"
	"fmt"
	"io"
	"math/rand/v2"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	weir "github.com/batchstream/weir-go"
	"go.mongodb.org/mongo-driver/v2/bson"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// memoryStore holds sequences in memory; unavailable fails direct reads.
type memoryStore struct {
	seqs        map[string]int64
	unavailable bool
}

func (m *memoryStore) resource(id string) string { return "records/s:" + id }
func (m *memoryStore) contentType() string       { return "text/plain" }

func (m *memoryStore) encode(_ string, seq int64, _ string) ([]byte, error) {
	return []byte(strconv.FormatInt(seq, 10)), nil
}

func (m *memoryStore) decode(raw []byte) (int64, error) { return strconv.ParseInt(string(raw), 10, 64) }

func (m *memoryStore) read(_ context.Context, id string) (int64, error) {
	if m.unavailable {
		return 0, errors.New("unavailable")
	}
	return m.seqs[id], nil
}

func (m *memoryStore) nativeRead(id string) (*weir.NativeRequest, error) {
	request := &weir.NativeRequest{Resource: id}
	return request, nil
}

func (m *memoryStore) nativeSeq(_ *weir.NativeResponse, body []byte) (int64, error) {
	if string(body) == "refused" {
		return 0, errBackendRefused
	}
	return m.decode(body)
}

func (m *memoryStore) readAll(context.Context, []string) (map[string]int64, error) {
	return m.seqs, nil
}

func (m *memoryStore) connections(context.Context) (int64, error) { return 1, nil }
func (m *memoryStore) create(context.Context) error               { return nil }
func (m *memoryStore) drop(context.Context) error                 { return nil }
func (m *memoryStore) close(context.Context) error                { return nil }

// step is one scripted Put: the reported outcome and whether the write
// actually reaches the store.
type step struct {
	outcome weir.MutationOutcome
	lands   bool
}

type scriptedClient struct {
	store  *memoryStore
	steps  []step
	read   *weir.ReadResult
	native []byte
	// mutate answers a Mutate call; mutateStream and readStream run a stream.
	mutate       func(weir.MutateOptions) ([]*weir.MutationResult, error)
	reads        func(weir.ReadOptions) ([]*weir.ReadResult, error)
	mutateStream func(context.Context, weir.MutateStreamOptions) error
	readStream   func(context.Context, weir.ReadStreamOptions) error
	transforms   []*weir.AtomicTransformRequest
}

func (c *scriptedClient) Put(_ context.Context, options weir.WriteOptions) (*weir.MutationResult, error) {
	next := c.steps[0]
	c.steps = c.steps[1:]
	if next.lands {
		seq, _ := c.store.decode(options.Request.Document.Data)
		c.store.seqs["k"] = seq
	}
	result := &weir.MutationResult{Outcome: next.outcome}
	return result, nil
}

func (c *scriptedClient) ReadOne(context.Context, weir.ReadOneOptions) (*weir.ReadResult, error) {
	return c.read, nil
}

func (c *scriptedClient) AtomicTransform(_ context.Context, options weir.AtomicTransformOptions) (*weir.MutationResult, error) {
	c.transforms = append(c.transforms, options.Request)
	result := &weir.MutationResult{Outcome: weir.MutationApplied}
	return result, nil
}

func (c *scriptedClient) Mutate(_ context.Context, options weir.MutateOptions) ([]*weir.MutationResult, error) {
	return c.mutate(options)
}

func (c *scriptedClient) Read(_ context.Context, options weir.ReadOptions) ([]*weir.ReadResult, error) {
	return c.reads(options)
}

func (c *scriptedClient) MutateStream(ctx context.Context, options weir.MutateStreamOptions) error {
	return c.mutateStream(ctx, options)
}

func (c *scriptedClient) ReadStream(ctx context.Context, options weir.ReadStreamOptions) error {
	return c.readStream(ctx, options)
}

func (c *scriptedClient) Native(ctx context.Context, options weir.NativeOptions) (*weir.NativeResult, error) {
	if err := options.Consume(ctx, nil, c.native); err != nil {
		return nil, err
	}
	result := &weir.NativeResult{Completion: weir.NativeResponseComplete}
	return result, nil
}

// readyTokens holds count tokens a worker can take at once.
func readyTokens(count int) chan struct{} {
	tokens := make(chan struct{}, count)
	for range count {
		tokens <- struct{}{}
	}
	return tokens
}

// withKeys gives the worker count keys, returned for inspection.
func withKeys(load *worker, count int) []*keyState {
	load.keys = make([]*keyState, count)
	for index := range load.keys {
		load.keys[index] = &keyState{id: fmt.Sprintf("k%d", index)}
	}
	return load.keys
}

func applied() *weir.MutationResult {
	result := &weir.MutationResult{Outcome: weir.MutationApplied}
	return result
}

func notStarted() *weir.MutationResult {
	result := &weir.MutationResult{Outcome: weir.MutationNotStarted}
	return result
}

func newTestWorker(store *memoryStore, client *scriptedClient) (*worker, *keyState) {
	key := &keyState{id: "k"}
	load := &worker{
		backend:       "test",
		store:         store,
		client:        client,
		keys:          []*keyState{key},
		verifyPercent: 100,
		timeout:       time.Second,
		lateWindow:    time.Minute,
		random:        rand.New(rand.NewPCG(1, 2)),
		stats:         &stats{},
		violations:    &violationLog{},
		stop:          make(chan struct{}),
	}
	return load, key
}

func TestKeyAllowsOnlyTheLatestAppliedOrAnUnknownSequence(t *testing.T) {
	key := &keyState{id: "k"}
	if !key.allows(0) || key.allows(1) {
		t.Fatal("a never-applied key may only be absent")
	}
	issued := time.Now()
	key.unknown = []unknownWrite{{seq: 1, issued: issued}}
	if !key.allows(0) || !key.allows(1) || key.allows(2) {
		t.Fatal("an unknown write may or may not have landed")
	}
	key.settle(3, issued.Add(time.Minute), time.Minute)
	if key.allows(0) || key.allows(2) || !key.allows(3) || !key.allows(1) {
		t.Fatal("after an APPLIED write only it or a late unknown write may be present")
	}
	key.settle(4, issued.Add(time.Minute+time.Millisecond), time.Minute)
	if key.allows(1) || key.allows(3) || !key.allows(4) || len(key.unknown) != 0 {
		t.Fatal("an unknown write issued a late window before an APPLIED one cannot land after it")
	}
}

func TestRefusedWritesRetryWithFreshSequencesUntilApplied(t *testing.T) {
	store := &memoryStore{seqs: map[string]int64{}}
	steps := []step{{outcome: weir.MutationNotStarted}, {outcome: weir.MutationNotApplied}, {outcome: weir.MutationApplied, lands: true}}
	client := &scriptedClient{store: store, steps: steps}
	load, key := newTestWorker(store, client)
	load.write(apiPut, key, load.putCall(nil))
	totals := load.stats.total()
	if key.applied != 3 || store.seqs["k"] != 3 || totals.Writes != 3 || totals.Retries != 2 || totals.NotStarted != 1 || totals.NotApplied != 1 || totals.Applied != 1 || totals.Verified != 3 {
		t.Fatalf("key=%+v store=%v totals=%+v", key, store.seqs, totals)
	}
	if count, _ := load.violations.snapshot(); count != 0 {
		t.Fatalf("violations=%d", count)
	}
}

func TestRefusalThatTookEffectIsAViolation(t *testing.T) {
	store := &memoryStore{seqs: map[string]int64{}}
	steps := []step{{outcome: weir.MutationNotApplied, lands: true}, {outcome: weir.MutationApplied, lands: true}}
	client := &scriptedClient{store: store, steps: steps}
	load, key := newTestWorker(store, client)
	load.write(apiPut, key, load.putCall(nil))
	count, kept := load.violations.snapshot()
	if count != 1 || kept[0].Kind != "direct-after-refused" || kept[0].Observed != 1 || key.applied != 2 {
		t.Fatalf("count=%d kept=%+v key=%+v", count, kept, key)
	}
}

func TestUnknownWritesWidenWhatTheDatabaseMayHold(t *testing.T) {
	store := &memoryStore{seqs: map[string]int64{}}
	steps := []step{{outcome: weir.MutationUnknown, lands: true}, {outcome: weir.MutationUnknown}}
	client := &scriptedClient{store: store, steps: steps}
	load, key := newTestWorker(store, client)
	load.write(apiPut, key, load.putCall(nil))
	load.write(apiPut, key, load.putCall(nil))
	run := &backendRun{config: Backend{Kind: "test"}, store: store, keys: load.keys}
	check := finalCheck(run, load.violations)
	if len(key.unknown) != 2 || check.Violations != 0 || check.Present != 1 || check.Unknown != 1 {
		t.Fatalf("key=%+v check=%+v", key, check)
	}
}

func TestLostAppliedWriteFailsTheFinalCheck(t *testing.T) {
	store := &memoryStore{seqs: map[string]int64{}}
	steps := []step{{outcome: weir.MutationApplied, lands: true}, {outcome: weir.MutationApplied}}
	client := &scriptedClient{store: store, steps: steps}
	load, key := newTestWorker(store, client)
	load.verifyPercent = 0
	load.write(apiPut, key, load.putCall(nil))
	load.write(apiPut, key, load.putCall(nil))
	run := &backendRun{config: Backend{Kind: "test"}, store: store, keys: load.keys}
	check := finalCheck(run, load.violations)
	if check.Violations != 1 || key.applied != 2 {
		t.Fatalf("check=%+v key=%+v", check, key)
	}
}

func TestSearchDocumentErrorsFailTheCheckInsteadOfReadingAbsent(t *testing.T) {
	handler := http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path == "/soak/_mget" {
			_, _ = writer.Write([]byte(`{"docs":[{"_id":"a","found":true,"_source":{"seq":3}},` +
				`{"_id":"b","error":{"type":"no_shard_available_action_exception"}}]}`))
			return
		}
		_, _ = writer.Write([]byte(`{}`))
	})
	server := httptest.NewServer(handler)
	defer server.Close()
	store, err := openSearch(context.Background(), server.URL, "soak")
	if err != nil {
		t.Fatal(err)
	}
	found, err := store.readAll(context.Background(), []string{"a", "b"})
	if err == nil {
		t.Fatalf("found=%v, want an error for the unavailable document", found)
	}
}

func TestStopAbandonsARetryingWrite(t *testing.T) {
	store := &memoryStore{seqs: map[string]int64{}, unavailable: true}
	steps := []step{{outcome: weir.MutationNotStarted}}
	client := &scriptedClient{store: store, steps: steps}
	load, key := newTestWorker(store, client)
	stop := make(chan struct{})
	close(stop)
	load.stop = stop
	load.write(apiPut, key, load.putCall(nil))
	totals := load.stats.total()
	if totals.Abandoned != 1 || totals.VerifyUnavailable != 1 || key.applied != 0 {
		t.Fatalf("totals=%+v key=%+v", totals, key)
	}
}

func TestWeirReadsMustReturnAnAllowedSequence(t *testing.T) {
	store := &memoryStore{seqs: map[string]int64{}}
	document := &weir.Document{ContentType: "text/plain", Data: []byte("1")}
	client := &scriptedClient{store: store, read: &weir.ReadResult{Document: document}}
	load, key := newTestWorker(store, client)
	key.applied = 2
	load.read(key)
	key.applied = 1
	load.read(key)
	count, kept := load.violations.snapshot()
	if count != 1 || kept[0].Kind != "weir-read" || load.stats.total().Reads != 2 {
		t.Fatalf("count=%d kept=%+v", count, kept)
	}
}

func TestLuaWritesStoreTheirInput(t *testing.T) {
	store := &memoryStore{seqs: map[string]int64{}}
	client := &scriptedClient{store: store}
	load, key := newTestWorker(store, client)
	if !load.luaTurn(readyTokens(1)) {
		t.Fatal("a token was ready")
	}
	if len(client.transforms) != 1 || string(client.transforms[0].Lua.Input.Data) != "1" || key.applied != 1 {
		t.Fatalf("transforms=%+v key=%+v", client.transforms, key)
	}
	if api := load.stats.total().APIs[apiLua]; api.Calls != 1 || api.Applied != 1 {
		t.Fatalf("api=%+v", api)
	}
}

func TestMutateLeavesPositionsWithoutAResultUnknown(t *testing.T) {
	store := &memoryStore{seqs: map[string]int64{}}
	client := &scriptedClient{store: store}
	client.mutate = func(options weir.MutateOptions) ([]*weir.MutationResult, error) {
		if len(options.Requests) != 3 {
			t.Errorf("requests=%d", len(options.Requests))
		}
		return []*weir.MutationResult{applied(), nil, notStarted()}, errors.New("broken")
	}
	load, _ := newTestWorker(store, client)
	keys := withKeys(load, 3)
	load.batch = 16
	load.verifyPercent = 0
	load.mutateTurn(readyTokens(3))
	if keys[0].applied != 1 || len(keys[1].unknown) != 1 || keys[1].applied != 0 || len(keys[2].unknown) != 0 {
		t.Fatalf("keys=%+v %+v %+v", keys[0], keys[1], keys[2])
	}
	api := load.stats.total().APIs[apiMutate]
	if api.Calls != 1 || api.Writes != 3 || api.Applied != 1 || api.Unknown != 1 || api.NotStarted != 1 || api.SDKErrors != 1 {
		t.Fatalf("api=%+v", api)
	}
	if load.stats.total().Verified != 1 {
		t.Fatalf("a refusal is checked against the database: %+v", load.stats.total())
	}
}

// sendAll drives a stream's producer until it ends the input or limit records
// were sent, and returns how many were.
func sendAll(t *testing.T, ctx context.Context, next func(context.Context) (*weir.MutateRequest, error), limit int) int {
	t.Helper()
	for sent := 0; sent < limit; sent++ {
		_, err := next(ctx)
		if errors.Is(err, io.EOF) {
			return sent
		}
		if err != nil {
			t.Fatal(err)
		}
	}
	return limit
}

func TestMutateStreamLeavesRecordsWithoutAResultUnknown(t *testing.T) {
	store := &memoryStore{seqs: map[string]int64{}}
	client := &scriptedClient{store: store}
	client.mutateStream = func(ctx context.Context, options weir.MutateStreamOptions) error {
		sendAll(t, ctx, options.Next, 3)
		if err := options.Consume(ctx, 1, applied()); err != nil {
			t.Fatal(err)
		}
		if err := options.Consume(ctx, 3, notStarted()); err != nil {
			t.Fatal(err)
		}
		return status.Error(codes.Unavailable, "connection lost")
	}
	load, _ := newTestWorker(store, client)
	keys := withKeys(load, 3)
	load.streamLifetime = time.Hour
	load.verifyPercent = 0
	load.mutateStreamTurn(readyTokens(3))
	if keys[0].applied != 1 || len(keys[1].unknown) != 1 || keys[2].issued != 1 || keys[2].applied != 0 || len(keys[2].unknown) != 0 {
		t.Fatalf("keys=%+v %+v %+v", keys[0], keys[1], keys[2])
	}
	api := load.stats.total().APIs[apiMutateStream]
	if api.Calls != 1 || api.Writes != 3 || api.Applied != 1 || api.Unknown != 1 || api.NotStarted != 1 || api.SDKErrors != 1 || api.Timeouts != 0 {
		t.Fatalf("api=%+v", api)
	}
}

func TestStreamChecksEveryRefusalBeforeTheKeyIsWrittenAgain(t *testing.T) {
	store := &memoryStore{seqs: map[string]int64{}}
	client := &scriptedClient{store: store}
	refused := &weir.MutationResult{Outcome: weir.MutationNotApplied}
	client.mutateStream = func(ctx context.Context, options weir.MutateStreamOptions) error {
		// The refused write took effect, and a later write replaces it.
		if _, err := options.Next(ctx); err != nil {
			t.Fatal(err)
		}
		store.seqs["k"] = 1
		if err := options.Consume(ctx, 1, refused); err != nil {
			t.Fatal(err)
		}
		// The only key stays busy until its check ran.
		if _, err := options.Next(ctx); err != nil {
			t.Fatal(err)
		}
		store.seqs["k"] = 2
		return options.Consume(ctx, 2, applied())
	}
	load, key := newTestWorker(store, client)
	load.verifyPercent = 0
	load.streamLifetime = time.Hour
	load.mutateStreamTurn(readyTokens(2))
	count, kept := load.violations.snapshot()
	if count != 1 || kept[0].Kind != "direct-after-refused" || kept[0].Observed != 1 || key.applied != 2 {
		t.Fatalf("count=%d kept=%+v key=%+v", count, kept, key)
	}
}

func TestStreamSamplesAppliedWritesForTheDatabaseCheck(t *testing.T) {
	store := &memoryStore{seqs: map[string]int64{}}
	client := &scriptedClient{store: store}
	client.mutateStream = func(ctx context.Context, options weir.MutateStreamOptions) error {
		if _, err := options.Next(ctx); err != nil {
			t.Fatal(err)
		}
		// The database lost this APPLIED write.
		return options.Consume(ctx, 1, applied())
	}
	load, _ := newTestWorker(store, client)
	load.streamLifetime = time.Hour
	load.mutateStreamTurn(readyTokens(1))
	if count, kept := load.violations.snapshot(); count != 1 || kept[0].Kind != "direct-after-applied" {
		t.Fatalf("count=%d kept=%+v", count, kept)
	}
}

func TestStreamRecordsWithoutAResultMayLandAfterTheStreamGaveUp(t *testing.T) {
	store := &memoryStore{seqs: map[string]int64{}}
	client := &scriptedClient{store: store}
	var gaveUp time.Time
	client.mutateStream = func(ctx context.Context, options weir.MutateStreamOptions) error {
		if _, err := options.Next(ctx); err != nil {
			t.Fatal(err)
		}
		time.Sleep(10 * time.Millisecond)
		gaveUp = time.Now()
		return errors.New("broken")
	}
	load, key := newTestWorker(store, client)
	load.streamLifetime = time.Hour
	load.mutateStreamTurn(readyTokens(1))
	if len(key.unknown) != 1 || key.unknown[0].issued.Before(gaveUp) {
		t.Fatalf("unknown=%+v gaveUp=%v", key.unknown, gaveUp)
	}
}

func TestRunMustFitBothStreamTurns(t *testing.T) {
	config := Config{
		WeirAddress: "weir:7447", WeirDiagnostics: "weir:7449", Output: "out",
		Backends: []Backend{{Kind: "mongo", Store: "mongo", URI: "mongodb://mongo", Workers: 4, KeysPerWorker: 1, Rate: 1}},
		Duration: time.Minute, SampleInterval: time.Second, OperationTimeout: time.Second,
		Profiles: []ProfileShare{{Name: "stream", Weight: 1}}, BatchSize: 1, StreamLifetime: 30 * time.Second, ReadPercent: 10,
	}
	if err := config.validate(); err == nil || !strings.Contains(err.Error(), "stream-lifetime") {
		t.Fatalf("err=%v", err)
	}
	config.ReadPercent = 0
	if err := config.validate(); err != nil {
		t.Fatal(err)
	}
	config.ReadPercent = 10
	config.StreamLifetime = 20 * time.Second
	if err := config.validate(); err != nil {
		t.Fatal(err)
	}
}

func TestStreamEndsItsInputAtItsLifetime(t *testing.T) {
	store := &memoryStore{seqs: map[string]int64{}}
	client := &scriptedClient{store: store}
	sent := 0
	client.mutateStream = func(ctx context.Context, options weir.MutateStreamOptions) error {
		sent = sendAll(t, ctx, options.Next, 2)
		return nil
	}
	load, _ := newTestWorker(store, client)
	withKeys(load, 4)
	load.streamLifetime = 50 * time.Millisecond
	// No token arrives for a second record, so only the lifetime releases it.
	if !load.mutateStreamTurn(readyTokens(1)) {
		t.Fatal("a token was ready")
	}
	if sent != 1 {
		t.Fatalf("sent=%d, want one record before the lifetime ended", sent)
	}
}

func TestStreamEndsItsInputAtStop(t *testing.T) {
	store := &memoryStore{seqs: map[string]int64{}}
	load, _ := newTestWorker(store, &scriptedClient{store: store})
	withKeys(load, 4)
	stop := make(chan struct{})
	close(stop)
	load.stop = stop
	stream := load.newStreamTurn(apiMutateStream, readyTokens(0), context.Background())
	if _, _, err := stream.reserve(context.Background()); err != nil {
		t.Fatalf("the first record needs no token: %v", err)
	}
	if _, _, err := stream.reserve(context.Background()); !errors.Is(err, io.EOF) {
		t.Fatalf("err=%v, want the input to end at stop", err)
	}
}

func TestStreamNeverHasTwoWritesInFlightForOneKey(t *testing.T) {
	store := &memoryStore{seqs: map[string]int64{}}
	load, _ := newTestWorker(store, &scriptedClient{store: store})
	keys := withKeys(load, 2)
	turn := load.newStreamTurn(apiMutateStream, readyTokens(8), context.Background())
	first, _, err := turn.reserve(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	second, _, err := turn.reserve(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if first == second {
		t.Fatalf("both records went to %+v", first)
	}
	waiting, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	if _, _, err := turn.reserve(waiting); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("err=%v, want a third record to wait for room", err)
	}
	turn.mu.Lock()
	turn.release(keys[0])
	turn.mu.Unlock()
	third, _, err := turn.reserve(context.Background())
	if err != nil || third != keys[0] {
		t.Fatalf("third=%+v err=%v", third, err)
	}
}

func TestReadStreamChecksResultsAndCountsTheRestAsFailed(t *testing.T) {
	store := &memoryStore{seqs: map[string]int64{}}
	stale := &weir.Document{ContentType: "text/plain", Data: []byte("1")}
	client := &scriptedClient{store: store}
	client.readStream = func(ctx context.Context, options weir.ReadStreamOptions) error {
		for range 2 {
			if _, err := options.Next(ctx); err != nil {
				t.Fatal(err)
			}
		}
		result := &weir.ReadResult{Document: stale}
		if err := options.Consume(ctx, 1, result); err != nil {
			t.Fatal(err)
		}
		return context.DeadlineExceeded
	}
	load, _ := newTestWorker(store, client)
	keys := withKeys(load, 2)
	keys[0].applied = 2
	load.streamLifetime = time.Hour
	load.readStreamTurn(readyTokens(2))
	count, kept := load.violations.snapshot()
	if count != 1 || kept[0].Kind != "weir-read" {
		t.Fatalf("count=%d kept=%+v", count, kept)
	}
	api := load.stats.total().APIs[apiReadStream]
	if api.Reads != 2 || api.ReadFailures != 1 || api.SDKErrors != 1 || api.Timeouts != 1 {
		t.Fatalf("api=%+v", api)
	}
}

func TestReadBatchChecksEveryKey(t *testing.T) {
	store := &memoryStore{seqs: map[string]int64{}}
	client := &scriptedClient{store: store}
	client.reads = func(weir.ReadOptions) ([]*weir.ReadResult, error) {
		missing := &weir.ReadResult{Missing: true}
		return []*weir.ReadResult{missing, nil}, errors.New("broken")
	}
	load, _ := newTestWorker(store, client)
	keys := withKeys(load, 2)
	keys[0].applied = 1
	load.batch = 2
	load.readBatchTurn(readyTokens(2))
	count, kept := load.violations.snapshot()
	api := load.stats.total().APIs[apiRead]
	if count != 1 || kept[0].Kind != "weir-read" || api.Reads != 2 || api.ReadFailures != 1 || api.SDKErrors != 1 {
		t.Fatalf("count=%d kept=%+v api=%+v", count, kept, api)
	}
}

func TestNativeReadsMustReturnAnAllowedSequence(t *testing.T) {
	store := &memoryStore{seqs: map[string]int64{}}
	client := &scriptedClient{store: store, native: []byte("1")}
	load, key := newTestWorker(store, client)
	key.applied = 2
	load.nativeTurn(readyTokens(1))
	if count, kept := load.violations.snapshot(); count != 1 || kept[0].Kind != "weir-native" {
		t.Fatalf("count=%d kept=%+v", count, kept)
	}
	client.native = []byte("refused")
	load.nativeTurn(readyTokens(1))
	client.native = []byte("2")
	load.nativeTurn(readyTokens(1))
	count, _ := load.violations.snapshot()
	api := load.stats.total().APIs[apiNative]
	if count != 1 || api.Reads != 3 || api.ReadFailures != 1 || !strings.Contains(api.LastFailure, "refused") {
		t.Fatalf("count=%d api=%+v", count, api)
	}
}

func TestMongoNativeReplies(t *testing.T) {
	mongo := &mongoStore{}
	found, err := bson.Marshal(bson.D{{Key: "cursor", Value: bson.D{{Key: "firstBatch", Value: bson.A{bson.D{{Key: "_id", Value: "k"}, {Key: "seq", Value: int64(7)}}}}}}, {Key: "ok", Value: 1.0}})
	if err != nil {
		t.Fatal(err)
	}
	if seq, err := mongo.nativeSeq(nil, found); err != nil || seq != 7 {
		t.Fatalf("seq=%d err=%v", seq, err)
	}
	absent, err := bson.Marshal(bson.D{{Key: "cursor", Value: bson.D{{Key: "firstBatch", Value: bson.A{}}}}, {Key: "ok", Value: 1}})
	if err != nil {
		t.Fatal(err)
	}
	if seq, err := mongo.nativeSeq(nil, absent); err != nil || seq != 0 {
		t.Fatalf("seq=%d err=%v", seq, err)
	}
	refused, err := bson.Marshal(bson.D{{Key: "ok", Value: 0.0}, {Key: "errmsg", Value: "not primary"}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := mongo.nativeSeq(nil, refused); !errors.Is(err, errBackendRefused) {
		t.Fatalf("err=%v", err)
	}
}

func TestSearchNativeReplies(t *testing.T) {
	search := &searchStore{}
	response := func(line string) *weir.NativeResponse {
		metadata := &weir.Document{ContentType: "application/http", Data: []byte(line + "\r\n\r\n")}
		wrapped := &weir.NativeResponse{Metadata: metadata}
		return wrapped
	}
	if seq, err := search.nativeSeq(response("HTTP/1.1 200 OK"), []byte(`{"found":true,"_source":{"seq":7}}`)); err != nil || seq != 7 {
		t.Fatalf("seq=%d err=%v", seq, err)
	}
	if seq, err := search.nativeSeq(response("HTTP/1.1 404 Not Found"), []byte(`{"found":false}`)); err != nil || seq != 0 {
		t.Fatalf("seq=%d err=%v", seq, err)
	}
	if _, err := search.nativeSeq(response("HTTP/1.1 503 Service Unavailable"), []byte(`{}`)); !errors.Is(err, errBackendRefused) {
		t.Fatalf("err=%v", err)
	}
	if _, err := search.nativeSeq(response("HTTP/1.1 404 Not Found"), []byte(`{"error":{"type":"index_not_found_exception"}}`)); !errors.Is(err, errBackendRefused) {
		t.Fatalf("err=%v", err)
	}
	if _, err := search.nativeSeq(response("HTTP/1.1 200 OK"), []byte(`not json`)); err == nil || errors.Is(err, errBackendRefused) {
		t.Fatalf("err=%v, want a malformed success to stay a violation", err)
	}
}

func TestStatsBreakDownByCallAndCountTimeouts(t *testing.T) {
	var counts stats
	counts.sdkError(apiPut, context.DeadlineExceeded)
	counts.sdkError(apiPut, fmt.Errorf("wrapped: %w", status.Error(codes.DeadlineExceeded, "late")))
	counts.sdkError(apiPut, status.Error(codes.Unavailable, "gone"))
	failure := &weir.Failure{Code: weir.FailureDeadlineExceeded, Message: "late"}
	counts.failure(apiLua, failure)
	counts.failure(apiLua, nil)
	counts.read(apiNative, true, 5*time.Second)
	counts.read(apiNative, false, time.Millisecond)
	totals := counts.total()
	if put := totals.APIs[apiPut]; put.SDKErrors != 3 || put.Timeouts != 2 || put.LastSDKError == "" {
		t.Fatalf("put=%+v", put)
	}
	if lua := totals.APIs[apiLua]; lua.Timeouts != 1 || !strings.Contains(lua.LastFailure, "late") {
		t.Fatalf("lua=%+v", lua)
	}
	native := totals.APIs[apiNative]
	if native.ReadMax != 5000 || native.ReadP99 > 2 || native.ReadFailures != 1 {
		t.Fatalf("a read that waited for its deadline shows in the maximum, not the quantiles: %+v", native)
	}
	if totals.SDKErrors != 3 || totals.Timeouts != 3 || totals.Reads != 2 {
		t.Fatalf("totals=%+v", totals)
	}
}

func TestParseProfiles(t *testing.T) {
	shares, err := ParseProfiles(DefaultProfiles)
	if err != nil {
		t.Fatal(err)
	}
	covered := make(map[string]bool)
	for _, share := range shares {
		chosen := profiles[share.Name]
		covered[chosen.writeAPI] = true
		covered[chosen.readAPI] = true
	}
	for _, api := range []string{apiPut, apiPutVisible, apiLua, apiMutate, apiMutateStream, apiReadOne, apiRead, apiReadStream, apiNative} {
		if !covered[api] {
			t.Errorf("the default profiles do not call %s", api)
		}
	}
	for _, spec := range []string{"", "single", "single=0", "single=-1", "nope=1", "single=1,single=2"} {
		if _, err := ParseProfiles(spec); err == nil {
			t.Errorf("%q parsed", spec)
		}
	}
}

func TestProfilesSpreadWorkersByWeight(t *testing.T) {
	shares := []ProfileShare{{Name: "single", Weight: 3}, {Name: "batch", Weight: 1}}
	counts := make(map[string]int)
	for index := range 100 {
		counts[profileOf(shares, index, 100)]++
	}
	if counts["single"] != 75 || counts["batch"] != 25 {
		t.Fatalf("counts=%v", counts)
	}
	if err := checkShares(shares, 4); err != nil {
		t.Fatal(err)
	}
	rare := []ProfileShare{{Name: "single", Weight: 99}, {Name: "batch", Weight: 1}}
	if err := checkShares(rare, 8); err == nil {
		t.Fatal("eight workers cannot give a 1% profile any")
	}
}

func TestEveryConfiguredCallMustSucceed(t *testing.T) {
	shares := []ProfileShare{{Name: "stream", Weight: 1}}
	reads := Counters{Reads: 3}
	writes := Counters{Applied: 1}
	failed := Counters{Reads: 3, ReadFailures: 3}
	tests := []struct {
		name        string
		readPercent int
		apis        map[string]Counters
		want        string
	}{
		{name: "both", readPercent: 10, apis: map[string]Counters{apiMutateStream: writes, apiReadStream: reads}},
		{name: "no write", readPercent: 10, apis: map[string]Counters{apiReadStream: reads}, want: apiMutateStream},
		{name: "reads all failed", readPercent: 10, apis: map[string]Counters{apiMutateStream: writes, apiReadStream: failed}, want: apiReadStream},
		{name: "writes all APPLIED with a failure", readPercent: 10, apis: map[string]Counters{apiMutateStream: {Applied: 2, AppliedWithFault: 2}, apiReadStream: reads}, want: apiMutateStream},
		{name: "reads not configured", readPercent: 0, apis: map[string]Counters{apiMutateStream: writes}},
		{name: "writes not configured", readPercent: 100, apis: map[string]Counters{apiReadStream: reads}},
	}
	for _, test := range tests {
		totals := Counters{APIs: test.apis}
		missing := strings.Join(untestedCalls(shares, test.readPercent, totals), ",")
		if missing != test.want {
			t.Errorf("%s: missing %q, want %q", test.name, missing, test.want)
		}
	}
}

func TestHistogramQuantilesBoundTheirSamples(t *testing.T) {
	var latencies histogram
	if latencies.quantile(0.99) != 0 {
		t.Fatal("an empty histogram has no quantile")
	}
	for range 99 {
		latencies.add(time.Millisecond)
	}
	latencies.add(time.Second)
	p50, p99, p100 := latencies.quantile(0.50), latencies.quantile(0.99), latencies.quantile(1)
	if p50 < 1 || p50 > 1.2 || p99 != p50 || p100 < 1000 || p100 > 1200 {
		t.Fatalf("p50=%v p99=%v p100=%v", p50, p99, p100)
	}
}

func TestMetricSumAddsEveryLabelSetOfOneName(t *testing.T) {
	raw := "# TYPE go_goroutines gauge\ngo_goroutines 12\ngo_goroutines_extra 5\n" +
		"weir_memory_live_bytes{store=\"a\"} 1e3\nweir_memory_live_bytes{store=\"b\"} 24\n"
	if metricSum(raw, "go_goroutines") != 12 || metricSum(raw, "weir_memory_live_bytes") != 1024 || metricSum(raw, "missing") != 0 {
		t.Fatal("metric sums differ")
	}
}
