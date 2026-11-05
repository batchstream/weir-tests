//go:build integration

package integration_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"slices"
	"testing"

	weir "github.com/batchstream/weir-go"
	"github.com/batchstream/weir-tests/internal/fixture"
	"go.mongodb.org/mongo-driver/v2/bson"
)

func testLifecycle(t *testing.T, ctx context.Context, client *weir.Client, backend *backendData) {
	t.Helper()
	write := backend.writeAs(t, weir.MutationCreate, "primary", 1)
	rpc, cancel := context.WithTimeout(ctx, rpcTimeout)
	result, err := mutateRecord(rpc, client, backend.name, write)
	cancel()
	assertApplied(t, result, err)
	rpc, cancel = context.WithTimeout(ctx, rpcTimeout)
	result, err = mutateRecord(rpc, client, backend.name, write)
	cancel()
	assertPrecondition(t, result, err)
	write = backend.writeAs(t, weir.MutationReplace, "missing_replace", 42)
	rpc, cancel = context.WithTimeout(ctx, rpcTimeout)
	result, err = mutateRecord(rpc, client, backend.name, write)
	cancel()
	assertPrecondition(t, result, err)
	backend.assertMissing(t, ctx, "missing_replace")

	write = backend.writeAs(t, weir.MutationReplace, "primary", 2)
	rpc, cancel = context.WithTimeout(ctx, rpcTimeout)
	result, err = mutateRecord(rpc, client, backend.name, write)
	cancel()
	assertApplied(t, result, err)
	write = backend.write(t, "primary", 3)
	rpc, cancel = context.WithTimeout(ctx, rpcTimeout)
	result, err = mutateRecord(rpc, client, backend.name, write)
	cancel()
	assertApplied(t, result, err)
	transform := backend.transform(t, "primary")
	rpc, cancel = context.WithTimeout(ctx, rpcTimeout)
	result, err = mutateRecord(rpc, client, backend.name, transform)
	cancel()
	assertApplied(t, result, err)
	want := record{id: "primary", n: 4}
	backend.assertRead(t, ctx, client, want)
	backend.assertPersisted(t, ctx, "primary", 4)
	testTypedSequence(t, ctx, client, backend)
	testNative(t, ctx, client, backend)

	write = backend.write(t, "disposable", 7)
	rpc, cancel = context.WithTimeout(ctx, rpcTimeout)
	result, err = mutateRecord(rpc, client, backend.name, write)
	cancel()
	assertApplied(t, result, err)
	remove := &weir.MutateRequest{Resource: backend.resource("disposable"), Action: weir.MutationDelete}
	rpc, cancel = context.WithTimeout(ctx, rpcTimeout)
	result, err = mutateRecord(rpc, client, backend.name, remove)
	cancel()
	assertApplied(t, result, err)
	read := &weir.ReadRequest{Resource: backend.resource("disposable")}
	rpc, cancel = context.WithTimeout(ctx, rpcTimeout)
	missing, err := readRecord(rpc, client, backend.name, read)
	cancel()
	if err != nil || missing == nil || !missing.Missing || missing.Failure != nil {
		t.Fatalf("Read after Delete: result=%v err=%v", missing, err)
	}
	backend.assertMissing(t, ctx, "disposable")
	for i := 1; i <= 2; i++ {
		write = backend.writeAs(t, weir.MutationCreate, fmt.Sprintf("page%d", i), int64(10+i))
		rpc, cancel = context.WithTimeout(ctx, rpcTimeout)
		result, err = mutateRecord(rpc, client, backend.name, write)
		cancel()
		assertApplied(t, result, err)
	}
	t.Log("Create/preconditions/Replace/Put/AtomicTransform/Read/Delete, typed Read/Mutate sequence, Native, and direct database persistence verified")
}

func assertApplied(t *testing.T, result *weir.MutationResult, err error) {
	t.Helper()
	if err != nil || result == nil || result.GetOutcome() != weir.MutationApplied || result.GetFailure() != nil {
		t.Fatalf("mutation acknowledgement: result=%v err=%v", result, err)
	}
}

func assertPrecondition(t *testing.T, result *weir.MutationResult, err error) {
	t.Helper()
	if err != nil || result == nil || result.GetOutcome() != weir.MutationNotApplied || result.GetFailure().GetCode() != weir.FailurePreconditionFailed {
		t.Fatalf("precondition evidence: result=%v err=%v", result, err)
	}
}

func testTypedSequence(t *testing.T, ctx context.Context, client *weir.Client, backend *backendData) {
	t.Helper()
	read := &weir.ReadRequest{Resource: backend.resource("primary")}
	rpc, cancel := context.WithTimeout(ctx, rpcTimeout)
	before, err := client.Read(rpc, backend.name, weir.WithRequests(read))
	cancel()
	if err != nil || len(before) != 1 {
		t.Fatalf("typed Read before mutation: results=%v err=%v", before, err)
	}
	backend.assertReadResult(t, before[0], nil, 4)
	mutation := backend.writeAs(t, weir.MutationReplace, "primary", 5)
	rpc, cancel = context.WithTimeout(ctx, rpcTimeout)
	results, err := client.Mutate(rpc, backend.name, weir.WithRequests(mutation))
	cancel()
	if err != nil || len(results) != 1 {
		t.Fatalf("typed Mutate: results=%v err=%v", results, err)
	}
	assertApplied(t, results[0], nil)
	rpc, cancel = context.WithTimeout(ctx, rpcTimeout)
	after, err := client.Read(rpc, backend.name, weir.WithRequests(read))
	cancel()
	if err != nil || len(after) != 1 {
		t.Fatalf("typed Read after mutation: results=%v err=%v", after, err)
	}
	backend.assertReadResult(t, after[0], nil, 5)
	backend.assertPersisted(t, ctx, "primary", 5)
}

func testCrossReplicaNativePagination(t *testing.T, ctx context.Context, cluster *fixture.Cluster, backend *backendData) {
	t.Helper()
	if backend.name == "search" {
		refresh := httpRequest{method: http.MethodPost, path: "/" + backend.collection + "/_refresh"}
		code, body := backend.httpDo(t, ctx, refresh)
		if code != http.StatusOK {
			t.Fatalf("refresh owned Search index: status=%d body=%s", code, body)
		}
	}
	replicas := []*weir.Client{
		replicaClient(t, cluster.Nodes[0].Application), replicaClient(t, cluster.Nodes[1].Application),
	}
	calls := 0
	invoke := func(ctx context.Context, call nativeCall) (*weir.NativeResult, error) {
		replica := replicas[calls%len(replicas)]
		calls++
		return runNative(ctx, replica, call)
	}
	query := nativeQuery{}
	pager, err := backend.nativePager(invoke, query)
	if err != nil {
		t.Fatal(err)
	}
	documents := collectNativePages(t, ctx, pager)
	got := make([]int64, 0, len(documents))
	for _, document := range documents {
		n, err := backend.number(document)
		if err != nil {
			t.Fatal(err)
		}
		got = append(got, n)
	}
	slices.Sort(got)
	want := []int64{5, 11, 12}
	if pager.pages < 3 || !slices.Equal(got, want) {
		t.Fatalf("cross-replica Native pagination: pages=%d got=%v want=%v", pager.pages, got, want)
	}

	// A caller abandoning a page owns the live backend cursor and its cleanup.
	early, err := backend.nativePager(invoke, query)
	if err != nil {
		t.Fatal(err)
	}
	cleanupNativePager(t, early)
	first, err := early.next(ctx)
	if err != nil || len(first) != 1 || early.exhausted || backend.name == "mongo" && early.mongoCursor == 0 || backend.name == "search" && early.scrollID == "" {
		t.Fatalf("early-stop Native page: documents=%d err=%v", len(first), err)
	}
	probe, err := early.continuation()
	if err != nil {
		t.Fatal(err)
	}
	if err := early.close(); err != nil || !early.reclaimed {
		t.Fatal("explicit early-stop cleanup did not reclaim a live cursor:", err)
	}
	head, body, err := early.exchange(ctx, probe)
	if err != nil {
		t.Fatal("closed continuation lost Native transfer completion:", err)
	}
	if backend.name == "mongo" {
		raw := bson.Raw(body)
		if raw.Validate() != nil {
			t.Fatal("invalid closed-cursor BSON")
		}
		ok, valid := raw.Lookup("ok").AsFloat64OK()
		code, hasCode := raw.Lookup("code").AsInt64OK()
		if !valid || ok != 0 || !hasCode || code != 43 {
			t.Fatalf("closed Mongo cursor business error: %s", raw)
		}
	} else {
		metadata, err := weir.ParseHTTPNativeResponse(head)
		var failure struct {
			Error json.RawMessage `json:"error"`
		}
		if err != nil || metadata.StatusCode != http.StatusNotFound || json.Unmarshal(body, &failure) != nil || len(failure.Error) == 0 {
			t.Fatalf("closed Search scroll business error: metadata=%v body=%s err=%v", metadata, body, err)
		}
	}
	t.Log("backend-owned Native continuation crossed replicas and exhausted without omissions/duplicates; caller explicitly reclaimed early-stop cursor; closed continuation retained complete backend error response")
}

func testNative(t *testing.T, ctx context.Context, client *weir.Client, backend *backendData) {
	t.Helper()
	request := &weir.NativeRequest{Resource: backend.collection}
	if backend.name == "mongo" {
		filter := bson.D{{Key: "_id", Value: "primary"}}
		command := bson.D{{Key: "count", Value: backend.mongo.Name()}, {Key: "query", Value: filter}}
		request.Resource = weir.EncodeSegment(backend.mongo.Database().Name())
		request.Request = bsonDocument(t, command)
	} else {
		request = nativeHTTPGet(t, backend.collection, "/_doc/primary")
	}
	var head *weir.NativeResponse
	var body []byte
	call := nativeCall{store: backend.name, request: request}
	call.consume = func(_ context.Context, response *weir.NativeResponse, chunk []byte) error {
		if response == nil || head != nil && head != response {
			return errors.New("Native response metadata changed between chunks")
		}
		head = response
		if len(body)+len(chunk) > 1<<20 {
			return errors.New("Native fixture response exceeded bound")
		}
		body = append(body, chunk...)
		return nil
	}
	rpc, cancel := context.WithTimeout(ctx, rpcTimeout)
	end, err := runNative(rpc, client, call)
	cancel()
	if err != nil || head == nil || end == nil || end.Response != head || end.Failure != nil || end.Completion != weir.NativeResponseComplete {
		t.Fatalf("Native evidence: head=%v end=%v err=%v", head, end, err)
	}
	refused := errors.New("Native consumer refused body")
	call.consume = func(context.Context, *weir.NativeResponse, []byte) error { return refused }
	rpc, cancel = context.WithTimeout(ctx, rpcTimeout)
	interrupted, err := runNative(rpc, client, call)
	cancel()
	if !errors.Is(err, refused) || interrupted == nil || interrupted.Response == nil || interrupted.Completion == weir.NativeResponseComplete {
		t.Fatalf("refused Native body claimed complete transfer: result=%v err=%v", interrupted, err)
	}
	if backend.name == "mongo" {
		raw := bson.Raw(body)
		if raw.Validate() != nil || raw.Lookup("n").AsInt64() != 1 || raw.Lookup("ok").AsFloat64() != 1 {
			t.Fatal("Native MongoDB count response mismatch")
		}
		return
	}
	metadata, err := weir.ParseHTTPNativeResponse(head)
	if err != nil {
		t.Fatal(err)
	}
	var response struct {
		Found  bool `json:"found"`
		Source struct {
			N int64 `json:"n"`
		} `json:"_source"`
	}
	if metadata == nil || metadata.StatusCode != http.StatusOK || json.Unmarshal(body, &response) != nil || !response.Found || response.Source.N != 5 {
		t.Fatalf("Native Search response: metadata=%v body=%s err=%v", metadata, body, err)
	}
}

func nativeHTTPGet(t *testing.T, resource, path string) *weir.NativeRequest {
	t.Helper()
	request, err := http.NewRequest(http.MethodGet, "http://weir.invalid"+path, nil)
	if err != nil {
		t.Fatal(err)
	}
	native, err := weir.NewHTTPNativeRequest(resource, request)
	if err != nil {
		t.Fatal(err)
	}
	return native
}
