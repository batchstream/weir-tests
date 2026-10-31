package soak

import (
	"context"
	"errors"
	"math/rand/v2"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"

	weir "github.com/batchstream/weir-go"
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
	store *memoryStore
	steps []step
	read  *weir.ReadResult
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
	load.write(key)
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
	load.write(key)
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
	load.write(key)
	load.write(key)
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
	load.write(key)
	load.write(key)
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
	load.write(key)
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
