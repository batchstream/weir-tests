//go:build integration

package integration_test

import (
	"context"
	"fmt"
	"net/http"
	"testing"

	weir "github.com/batchstream/weir-go"
)

func testPublicBatch(t *testing.T, ctx context.Context, client *weir.Client, backend *backendData) {
	t.Helper()
	secondary := provisionBatchTarget(t, ctx, backend)
	targets := []*backendData{backend, secondary}
	const count = 48
	mutations := make([]*weir.MutateRequest, count)
	for index := range count {
		target := targets[index%len(targets)]
		write := target.write(t, fmt.Sprintf("public_batch_%03d", index), int64(100+index))
		request := &weir.MutateRequest{Resource: write.Resource, Action: weir.MutationCreate, Document: write.Document}
		mutations[index] = request
	}
	// One real backend precondition failure must remain in its input position,
	// without converting successful independent mutations into an RPC error.
	duplicate := backend.write(t, "primary", 999)
	mutations[24] = &weir.MutateRequest{Resource: duplicate.Resource, Action: weir.MutationCreate, Document: duplicate.Document}
	opts := weir.MutateOptions{StoreName: backend.name, Requests: mutations}
	rpc, cancel := context.WithTimeout(ctx, rpcTimeout)
	results, err := client.Mutate(rpc, opts)
	cancel()
	if err != nil || len(results) != count {
		t.Fatalf("public batch Mutate: length=%d error=%v", len(results), err)
	}
	for index, result := range results {
		if index == 24 {
			assertPrecondition(t, result, nil)
			continue
		}
		assertApplied(t, result, nil)
		targets[index%len(targets)].assertPersisted(t, ctx, fmt.Sprintf("public_batch_%03d", index), int64(100+index))
	}
	backend.assertPersisted(t, ctx, "primary", 5)
	reads := make([]*weir.ReadRequest, count+1)
	for index := range count {
		input := count - 1 - index
		resource := targets[input%len(targets)].resource(fmt.Sprintf("public_batch_%03d", input))
		request := &weir.ReadRequest{Resource: resource}
		reads[index] = request
	}
	missing := &weir.ReadRequest{Resource: backend.resource("public_batch_absent")}
	reads[count] = missing
	readOptions := weir.ReadOptions{StoreName: backend.name, Requests: reads}
	rpc, cancel = context.WithTimeout(ctx, rpcTimeout)
	readResults, err := client.Read(rpc, readOptions)
	cancel()
	if err != nil || len(readResults) != count+1 {
		t.Fatalf("public batch Read: length=%d error=%v", len(readResults), err)
	}
	for index, result := range readResults {
		input := count - 1 - index
		if index == count || input == 24 {
			if result == nil || !result.GetMissing() || result.GetFailure() != nil {
				t.Fatalf("missing result moved or failed at index %d: %v", index, result)
			}
			continue
		}
		targets[input%len(targets)].assertReadResult(t, result, nil, int64(100+input))
	}
	// Invalid input at the end must be rejected before any earlier mutation
	// reaches either target, even when the batch exceeds the inflight window.
	rejected := make([]*weir.MutateRequest, count)
	for index := range count {
		target := targets[index%len(targets)]
		write := target.write(t, fmt.Sprintf("late_invalid_%03d", index), int64(index))
		request := &weir.MutateRequest{Resource: write.Resource, Action: weir.MutationPut, Document: write.Document}
		rejected[index] = request
	}
	rejected[count-1].Action = 0
	invalidOptions := weir.MutateOptions{StoreName: backend.name, Requests: rejected}
	rpc, cancel = context.WithTimeout(ctx, rpcTimeout)
	invalidResults, err := client.Mutate(rpc, invalidOptions)
	cancel()
	if err == nil || invalidResults != nil {
		t.Fatalf("invalid whole batch accepted: length=%d error=%v", len(invalidResults), err)
	}
	for index := range count {
		targets[index%len(targets)].assertMissing(t, ctx, fmt.Sprintf("late_invalid_%03d", index))
	}
	t.Log("48 independent SDK mutations across two same-Store targets, ordered results including precondition/missing, and late-invalid all-input preflight verified")
}

func provisionBatchTarget(t *testing.T, ctx context.Context, base *backendData) *backendData {
	t.Helper()
	name := base.collection + "_batch_extra"
	if base.name == "mongo" {
		database := base.mongo.Database()
		if err := database.CreateCollection(ctx, "batch_extra"); err != nil {
			t.Fatal("create second owned Mongo collection:", err)
		}
		secondary := &backendData{name: base.name, collection: database.Name() + "/batch_extra", mongo: database.Collection("batch_extra")}
		return secondary
	}
	secondary := &backendData{name: base.name, collection: name, searchURL: base.searchURL, http: base.http}
	create := httpRequest{method: http.MethodPut, path: "/" + name}
	code, body := secondary.httpDo(t, ctx, create)
	if code != http.StatusOK {
		t.Fatalf("create second exclusive Search target: %d %s", code, body)
	}
	t.Cleanup(func() {
		cleanup, cancel := context.WithTimeout(context.Background(), rpcTimeout)
		defer cancel()
		remove := httpRequest{method: http.MethodDelete, path: "/" + name}
		code, body := secondary.httpDo(t, cleanup, remove)
		if code != http.StatusOK {
			t.Errorf("drop second owned Search target: %d %s", code, body)
		}
	})
	return secondary
}
