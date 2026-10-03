//go:build integration

package integration_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
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
	testMixedBatch(t, ctx, client, backend)
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
	if err != nil || !missing.GetMissing() || missing.GetFailure() != nil {
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
	t.Log("Create/preconditions/Replace/Put/AtomicTransform/Read/Delete, finite mixed Execute, Native, and direct database persistence verified")
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

func testMixedBatch(t *testing.T, ctx context.Context, client *weir.Client, backend *backendData) {
	t.Helper()
	read := &weir.ReadRequest{Resource: backend.resource("primary")}
	write := backend.write(t, "primary", 5)
	commands := []*weir.Command{
		weir.NewReadCommand(read), weir.NewReplaceCommand(write), weir.NewReadCommand(read),
	}
	results := make([]*weir.Result, len(commands))
	completed := make(chan uint64, 1)
	produced, completions := 0, 0
	batch := weir.ExecuteOptions{StoreName: backend.name}
	// Execute permits concurrent requests; Complete is the application ordering
	// boundary for this deliberate read/write/read dependency.
	batch.Produce = func(ctx context.Context) (*weir.Command, error) {
		if produced > 0 {
			select {
			case id := <-completed:
				if id != uint64(produced) {
					return nil, fmt.Errorf("completion id=%d, want %d", id, produced)
				}
			case <-ctx.Done():
				return nil, ctx.Err()
			}
		}
		if produced == len(commands) {
			return nil, io.EOF
		}
		command := commands[produced]
		produced++
		return command, nil
	}
	batch.Consume = func(_ context.Context, id uint64, event *weir.Event) error {
		if id == 0 || id > uint64(len(results)) || event.GetResult() == nil || results[id-1] != nil {
			return fmt.Errorf("unexpected or duplicate result for id %d", id)
		}
		results[id-1] = event.GetResult()
		return nil
	}
	batch.Complete = func(ctx context.Context, id uint64) error {
		completions++
		select {
		case completed <- id:
			return nil
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	rpc, cancel := context.WithTimeout(ctx, rpcTimeout)
	err := client.Execute(rpc, batch)
	cancel()
	if err != nil || produced != 3 || completions != 3 {
		t.Fatalf("finite mixed Execute: produced=%d complete=%d err=%v", produced, completions, err)
	}
	backend.assertReadResult(t, results[0].GetRead(), nil, 4)
	assertApplied(t, results[1].GetMutation(), nil)
	backend.assertReadResult(t, results[2].GetRead(), nil, 5)
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
	seen := make(map[string]bool)
	var token []byte
	exhausted := false
	for page := 0; page < 5; page++ {
		request := &weir.ScanRequest{Resource: backend.collection, PageSize: 1, ContinuationToken: bytes.Clone(token)}
		count := uint64(0)
		scan := weir.ScanOptions{StoreName: backend.name, Request: request}
		scan.Consume = func(_ context.Context, document *weir.Document) error {
			id, err := backend.scanID(document)
			if err != nil {
				return err
			}
			if seen[id] {
				return fmt.Errorf("cross-owner continuation repeated %q", id)
			}
			seen[id] = true
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
	got := make([]string, 0, len(seen))
	for id := range seen {
		got = append(got, id)
	}
	slices.Sort(got)
	want := []string{"page1", "page2", "primary"}
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

func (b *backendData) scanID(document *weir.Document) (string, error) {
	if document == nil {
		return "", errors.New("Scan omitted document")
	}
	if b.name == "mongo" {
		if document.GetMediaType() != "application/bson" {
			return "", errors.New("MongoDB Scan changed native media type")
		}
		raw := bson.Raw(document.GetData())
		if err := raw.Validate(); err != nil {
			return "", err
		}
		id, ok := raw.Lookup("_id").StringValueOK()
		if !ok {
			return "", errors.New("MongoDB Scan omitted _id")
		}
		return id, nil
	}
	var hit struct {
		ID string `json:"_id"`
	}
	if document.GetMediaType() != "application/json" || json.Unmarshal(document.GetData(), &hit) != nil || hit.ID == "" {
		return "", errors.New("Search Scan omitted native hit _id")
	}
	return hit.ID, nil
}

func testNative(t *testing.T, ctx context.Context, client *weir.Client, backend *backendData) {
	t.Helper()
	request := &weir.NativeRequest{Resource: backend.collection}
	if backend.name == "mongo" {
		parts := strings.Split(backend.collection, "/")
		filter := bson.D{{Key: "_id", Value: "primary"}}
		command := bson.D{{Key: "count", Value: parts[len(parts)-1]}, {Key: "query", Value: filter}}
		request.Body = bsonDocument(t, command).Data
		request.BodyMediaType = "application/bson"
		request.Descriptor = &weir.Document{MediaType: "application/vnd.weir.mongodb-command.v1+protobuf"}
	} else {
		descriptor := &weir.SearchHTTPRequest{Method: http.MethodGet, Path: "/_doc/primary"}
		encoded, err := weir.SearchHTTPDescriptor(descriptor)
		if err != nil {
			t.Fatal(err)
		}
		request.Descriptor = encoded
	}
	var head *weir.NativeHead
	var body []byte
	options := weir.NativeOptions{StoreName: backend.name, Request: request}
	options.Consume = func(_ context.Context, event *weir.Event) error {
		if event.GetHead() != nil {
			if head != nil {
				return errors.New("duplicate Native head")
			}
			head = event.GetHead()
		}
		if len(body)+len(event.GetChunk()) > 1<<20 {
			return errors.New("Native fixture response exceeded bound")
		}
		body = append(body, event.GetChunk()...)
		return nil
	}
	rpc, cancel := context.WithTimeout(ctx, rpcTimeout)
	end, err := client.Native(rpc, options)
	cancel()
	if err != nil || head == nil || end == nil || end.GetFailure() != nil || end.GetCompletion() != weir.NativeResponseComplete {
		t.Fatalf("Native evidence: head=%v end=%v err=%v", head, end, err)
	}
	if backend.name == "mongo" {
		raw := bson.Raw(body)
		if raw.Validate() != nil || raw.Lookup("n").AsInt64() != 1 || raw.Lookup("ok").AsFloat64() != 1 {
			t.Fatal("Native MongoDB count response mismatch")
		}
		return
	}
	metadata, err := weir.DecodeSearchHTTPResponse(head.GetMetadata())
	var response struct {
		Found  bool `json:"found"`
		Source struct {
			N int64 `json:"n"`
		} `json:"_source"`
	}
	if err != nil || metadata.StatusCode != http.StatusOK || json.Unmarshal(body, &response) != nil || !response.Found || response.Source.N != 5 {
		t.Fatalf("Native Search response: metadata=%v body=%s err=%v", metadata, body, err)
	}
}
