//go:build integration

package integration_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"slices"
	"strings"
	"testing"

	weir "github.com/batchstream/weir-go"
	pb "github.com/batchstream/weir-protocol/api/weir/v1"
	"github.com/batchstream/weir-tests/internal/fixture"
	"go.mongodb.org/mongo-driver/v2/bson"
)

func testLifecycle(t *testing.T, ctx context.Context, client *weir.Client, backend *backendData) {
	t.Helper()
	write := backend.write(t, "primary", 1)
	options := weir.WriteOptions{StoreName: backend.name, Request: write}
	rpc, cancel := context.WithTimeout(ctx, rpcTimeout)
	result, err := client.Create(rpc, options)
	cancel()
	assertApplied(t, result, err)
	rpc, cancel = context.WithTimeout(ctx, rpcTimeout)
	result, err = client.Create(rpc, options)
	cancel()
	assertPrecondition(t, result, err)
	options.Request = backend.write(t, "missing_replace", 42)
	rpc, cancel = context.WithTimeout(ctx, rpcTimeout)
	result, err = client.Replace(rpc, options)
	cancel()
	assertPrecondition(t, result, err)
	backend.assertMissing(t, ctx, "missing_replace")

	options.Request = backend.write(t, "primary", 2)
	rpc, cancel = context.WithTimeout(ctx, rpcTimeout)
	result, err = client.Replace(rpc, options)
	cancel()
	assertApplied(t, result, err)
	options.Request = backend.write(t, "primary", 3)
	rpc, cancel = context.WithTimeout(ctx, rpcTimeout)
	result, err = client.Put(rpc, options)
	cancel()
	assertApplied(t, result, err)
	transform := backend.transform(t, "primary")
	transformOptions := weir.AtomicTransformOptions{StoreName: backend.name, Request: transform}
	rpc, cancel = context.WithTimeout(ctx, rpcTimeout)
	result, err = client.AtomicTransform(rpc, transformOptions)
	cancel()
	assertApplied(t, result, err)
	want := record{id: "primary", n: 4}
	backend.assertRead(t, ctx, client, want)
	backend.assertPersisted(t, ctx, "primary", 4)
	testTypedSequence(t, ctx, client, backend)
	testNative(t, ctx, client, backend)

	options.Request = backend.write(t, "disposable", 7)
	rpc, cancel = context.WithTimeout(ctx, rpcTimeout)
	result, err = client.Put(rpc, options)
	cancel()
	assertApplied(t, result, err)
	remove := &weir.DeleteRequest{Resource: backend.resource("disposable")}
	removeOptions := weir.DeleteOptions{StoreName: backend.name, Request: remove}
	rpc, cancel = context.WithTimeout(ctx, rpcTimeout)
	result, err = client.Delete(rpc, removeOptions)
	cancel()
	assertApplied(t, result, err)
	read := &weir.ReadRequest{Resource: backend.resource("disposable")}
	readOptions := weir.ReadOneOptions{StoreName: backend.name, Request: read}
	rpc, cancel = context.WithTimeout(ctx, rpcTimeout)
	missing, err := client.ReadOne(rpc, readOptions)
	cancel()
	if err != nil || missing == nil || !missing.Missing || missing.Failure != nil {
		t.Fatalf("Read after Delete: result=%v err=%v", missing, err)
	}
	backend.assertMissing(t, ctx, "disposable")
	for i := 1; i <= 2; i++ {
		options.Request = backend.write(t, fmt.Sprintf("page%d", i), int64(10+i))
		rpc, cancel = context.WithTimeout(ctx, rpcTimeout)
		result, err = client.Create(rpc, options)
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
	readOptions := weir.ReadOptions{StoreName: backend.name, Requests: []*weir.ReadRequest{read}}
	rpc, cancel := context.WithTimeout(ctx, rpcTimeout)
	before, err := client.Read(rpc, readOptions)
	cancel()
	if err != nil || len(before) != 1 {
		t.Fatalf("typed Read before mutation: results=%v err=%v", before, err)
	}
	backend.assertReadResult(t, before[0], nil, 4)
	write := backend.write(t, "primary", 5)
	mutation := &weir.MutateRequest{Resource: write.Resource, Action: weir.MutationReplace, Document: write.Document}
	mutationOptions := weir.MutateOptions{StoreName: backend.name, Requests: []*weir.MutateRequest{mutation}}
	rpc, cancel = context.WithTimeout(ctx, rpcTimeout)
	results, err := client.Mutate(rpc, mutationOptions)
	cancel()
	if err != nil || len(results) != 1 {
		t.Fatalf("typed Mutate: results=%v err=%v", results, err)
	}
	assertApplied(t, results[0], nil)
	rpc, cancel = context.WithTimeout(ctx, rpcTimeout)
	after, err := client.Read(rpc, readOptions)
	cancel()
	if err != nil || len(after) != 1 {
		t.Fatalf("typed Read after mutation: results=%v err=%v", after, err)
	}
	backend.assertReadResult(t, after[0], nil, 5)
	backend.assertPersisted(t, ctx, "primary", 5)
}

func testCrossOwnerScan(t *testing.T, ctx context.Context, cluster *fixture.Cluster, backend *backendData) {
	t.Helper()
	if backend.name == "search" {
		refresh := httpRequest{method: http.MethodPost, path: "/" + backend.collection + "/_refresh"}
		code, body := backend.httpDo(t, ctx, refresh)
		if code != http.StatusOK {
			t.Fatalf("refresh owned Search index: status=%d body=%s", code, body)
		}
	}
	owners := []int{0, 1}
	transports := make([]pb.StoreServiceClient, len(owners))
	for i, owner := range owners {
		transports[i] = transport(t, cluster.Nodes[owner].Application)
	}
	seen := make(map[int64]bool)
	var token []byte
	exhausted := false
	for page := 0; page < 5; page++ {
		request := &weir.ScanRequest{Resource: backend.collection, PageSize: 1, ContinuationToken: bytes.Clone(token)}
		count := uint64(0)
		scan := weir.ScanOptions{StoreName: backend.name, Request: request}
		scan.Consume = func(_ context.Context, document *weir.Document) error {
			number, err := backend.number(document)
			if err != nil {
				return err
			}
			if seen[number] {
				return fmt.Errorf("cross-owner continuation repeated %d", number)
			}
			seen[number] = true
			count++
			return nil
		}
		rpc, cancel := context.WithTimeout(ctx, rpcTimeout)
		end, err := weir.Scan(rpc, transports[page%len(transports)], scan)
		cancel()
		if err != nil || end == nil || end.GetFailure() != nil || end.GetDocumentCount() != count || count > 1 {
			t.Fatalf("cross-owner Scan page=%d count=%d end=%v err=%v", page, count, end, err)
		}
		next := end.GetNextContinuationToken()
		if end.GetExhausted() == (len(next) != 0) {
			t.Fatal("Scan page must have exactly one of exhausted or continuation")
		}
		if end.GetExhausted() {
			exhausted = true
			break
		}
		if count == 0 || bytes.Equal(token, next) {
			t.Fatal("Scan continuation did not advance")
		}
		token = bytes.Clone(next)
	}
	got := make([]int64, 0, len(seen))
	for number := range seen {
		got = append(got, number)
	}
	slices.Sort(got)
	want := []int64{5, 11, 12}
	if !exhausted || !slices.Equal(got, want) {
		t.Fatalf("cross-owner complete Scan set: exhausted=%v got=%v want=%v", exhausted, got, want)
	}
	refused := errors.New("consumer refused page")
	request := &weir.ScanRequest{Resource: backend.collection, PageSize: 1}
	scan := weir.ScanOptions{StoreName: backend.name, Request: request}
	scan.Consume = func(context.Context, *weir.Document) error { return refused }
	rpc, cancel := context.WithTimeout(ctx, rpcTimeout)
	end, err := weir.Scan(rpc, transports[0], scan)
	cancel()
	if end != nil || !errors.Is(err, refused) {
		t.Fatalf("failed consumer exposed a checkpoint: end=%v err=%v", end, err)
	}
	t.Log("all finite Scan pages resumed on alternating owners without omissions/duplicates; failed consumer received no checkpoint")
}

func testNative(t *testing.T, ctx context.Context, client *weir.Client, backend *backendData) {
	t.Helper()
	request := &weir.NativeRequest{Resource: backend.collection}
	if backend.name == "mongo" {
		parts := strings.Split(backend.collection, "/")
		filter := bson.D{{Key: "_id", Value: "primary"}}
		command := bson.D{{Key: "count", Value: parts[len(parts)-1]}, {Key: "query", Value: filter}}
		request.Request = bsonDocument(t, command)
	} else {
		request = nativeHTTPGet(t, backend.collection, "/_doc/primary")
	}
	var head *weir.NativeResponse
	var body []byte
	options := weir.NativeOptions{StoreName: backend.name, Request: request}
	options.Consume = func(_ context.Context, response *weir.NativeResponse, chunk []byte) error {
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
	end, err := client.Native(rpc, options)
	cancel()
	if err != nil || head == nil || end == nil || end.Response != head || end.Failure != nil || end.Completion != weir.NativeResponseComplete {
		t.Fatalf("Native evidence: head=%v end=%v err=%v", head, end, err)
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
