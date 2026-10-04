//go:build integration

package integration_test

import (
	"context"
	"fmt"
	"net/http"
	"testing"

	weir "github.com/batchstream/weir-go"
	pb "github.com/batchstream/weir-protocol/api/weir/v1"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func testPublicBatch(t *testing.T, ctx context.Context, client *weir.Client, backend *backendData) {
	t.Helper()
	secondary := backend.provisionTarget(t, ctx, "batch_extra")
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
			if result == nil || !result.Missing || result.Failure != nil {
				t.Fatalf("missing result moved or failed at index %d: %v", index, result)
			}
			continue
		}
		targets[input%len(targets)].assertReadResult(t, result, nil, int64(100+input))
	}
	// The slice API validates every item before it sends the first record.
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
		t.Fatalf("invalid request slice accepted: length=%d error=%v", len(invalidResults), err)
	}
	for index := range count {
		targets[index%len(targets)].assertMissing(t, ctx, fmt.Sprintf("late_invalid_%03d", index))
	}
	testDuplicateURIOrder(t, ctx, client, backend)
	t.Log("48 streamed SDK mutations across two same-Store targets, ordered results including precondition/missing, duplicate URI order, and slice preflight verified")
}

func (base *backendData) provisionTarget(t *testing.T, ctx context.Context, suffix string) *backendData {
	t.Helper()
	name := base.collection + "_" + suffix
	if base.name == "mongo" {
		database := base.mongo.Database()
		if err := database.CreateCollection(ctx, suffix); err != nil {
			t.Fatal("create second owned Mongo collection:", err)
		}
		secondary := &backendData{name: base.name, collection: database.Name() + "/" + suffix, mongo: database.Collection(suffix)}
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

func testDuplicateURIOrder(t *testing.T, ctx context.Context, client *weir.Client, backend *backendData) {
	t.Helper()
	const id = "ordered_mutations"
	// A failed item must not let later items for the same URI overtake it or
	// skip the remaining chain. The final value alone would miss misordered
	// intermediate acknowledgements, so check each precondition position too.
	actions := []weir.MutationAction{weir.MutationCreate, weir.MutationCreate, weir.MutationReplace, weir.MutationDelete, weir.MutationReplace, weir.MutationCreate}
	requests := make([]*weir.MutateRequest, len(actions))
	for index, action := range actions {
		write := backend.write(t, id, int64(200+index))
		request := &weir.MutateRequest{Resource: write.Resource, Action: action}
		if action != weir.MutationDelete {
			request.Document = write.Document
		}
		requests[index] = request
	}
	opts := weir.MutateOptions{StoreName: backend.name, Requests: requests}
	rpc, cancel := context.WithTimeout(ctx, rpcTimeout)
	results, err := client.Mutate(rpc, opts)
	cancel()
	if err != nil || len(results) != len(requests) {
		t.Fatalf("same-URI mutation batch: results=%v err=%v", results, err)
	}
	for index, result := range results {
		if index == 1 || index == 4 {
			assertPrecondition(t, result, nil)
		} else {
			assertApplied(t, result, nil)
		}
	}
	backend.assertPersisted(t, ctx, id, 205)
	first := &weir.ReadRequest{Resource: backend.resource(id)}
	missing := &weir.ReadRequest{Resource: backend.resource("ordered_missing")}
	primary := &weir.ReadRequest{Resource: backend.resource("primary")}
	// Repeated URIs and missing records cross multiple server aggregation groups.
	reads := make([]*weir.ReadRequest, 513)
	for index := range reads {
		switch index % 3 {
		case 0:
			reads[index] = first
		case 1:
			reads[index] = missing
		case 2:
			reads[index] = primary
		}
	}
	readOpts := weir.ReadOptions{StoreName: backend.name, Requests: reads}
	rpc, cancel = context.WithTimeout(ctx, rpcTimeout)
	readResults, err := client.Read(rpc, readOpts)
	cancel()
	if err != nil || len(readResults) != len(reads) {
		t.Fatalf("513 duplicate URI reads: results=%d err=%v", len(readResults), err)
	}
	for index, result := range readResults {
		switch index % 3 {
		case 0:
			backend.assertReadResult(t, result, nil, 205)
		case 1:
			if result == nil || !result.Missing || result.Failure != nil {
				t.Fatalf("missing input %d moved: %v", index, result)
			}
		case 2:
			backend.assertReadResult(t, result, nil, 5)
		}
	}
}

func (s *system) testServerPreflight(t *testing.T, backend *backendData) {
	t.Helper()
	for _, phase := range []string{"first_record", "after_confirmed_record"} {
		t.Run(backend.name+"/invalid_"+phase, func(t *testing.T) {
			wire := transport(t, s.cluster.Nodes[0].Application)
			rpc, cancel := context.WithTimeout(s.ctx, rpcTimeout)
			defer cancel()
			stream, err := wire.Execute(rpc)
			if err != nil {
				t.Fatal(err)
			}
			index := uint64(1)
			if phase == "after_confirmed_record" {
				write := backend.write(t, "wire_confirmed_prefix", 7)
				document := &pb.Document{ContentType: write.Document.ContentType, Data: write.Document.Data}
				action := &pb.MutateRequest_Put{Put: document}
				mutation := &pb.MutateRequest{Resource: write.Resource, Action: action}
				operation := &pb.Command_Mutate{Mutate: mutation}
				command := &pb.Command{Operation: operation}
				request := &pb.ExecuteRequest{StoreName: backend.name, Index: index, Command: command}
				if err := stream.Send(request); err != nil {
					t.Fatal(err)
				}
				response, err := stream.Recv()
				result := response.GetEvent().GetMutationResult()
				if err != nil || response.GetIndex() != index || result.GetOutcome() != pb.MutationOutcome_APPLIED || result.GetFailure() != nil {
					t.Fatal("valid prefix lacks successful acknowledgement", response, err)
				}
				index++
			}
			id := "wire_invalid_" + phase
			mutation := &pb.MutateRequest{Resource: backend.resource(id)}
			operation := &pb.Command_Mutate{Mutate: mutation}
			command := &pb.Command{Operation: operation}
			request := &pb.ExecuteRequest{StoreName: backend.name, Index: index, Command: command}
			if err := stream.Send(request); err != nil {
				t.Fatal(err)
			}
			if err := stream.CloseSend(); err != nil {
				t.Fatal(err)
			}
			response, err := stream.Recv()
			if status.Code(err) != codes.InvalidArgument || response != nil {
				t.Fatalf("invalid record was accepted: response=%v err=%v", response, err)
			}
			backend.assertMissing(t, s.ctx, id)
			if phase == "after_confirmed_record" {
				backend.assertPersisted(t, s.ctx, "wire_confirmed_prefix", 7)
			}
		})
	}
}

func (s *system) testLargeDistinctBatch(t *testing.T, backend *backendData) {
	t.Helper()
	const count = 513
	mutations := make([]*weir.MutateRequest, count)
	reads := make([]*weir.ReadRequest, count)
	for index := range mutations {
		write := backend.write(t, fmt.Sprintf("large_distinct_%03d", index), int64(10000+index))
		mutation := &weir.MutateRequest{Resource: write.Resource, Action: weir.MutationPut, Document: write.Document}
		mutations[index] = mutation
		read := &weir.ReadRequest{Resource: write.Resource}
		reads[count-1-index] = read
	}
	before := s.nodeMetrics(t)
	opts := weir.MutateOptions{StoreName: backend.name, Requests: mutations}
	rpc, cancel := context.WithTimeout(s.ctx, rpcTimeout)
	results, err := s.client.Mutate(rpc, opts)
	cancel()
	if err != nil || len(results) != count {
		t.Fatalf("513 distinct Put batch: results=%d err=%v", len(results), err)
	}
	for _, result := range results {
		assertApplied(t, result, nil)
	}
	evidence := streamBatchEvidence{Before: before, After: s.nodeMetrics(t), Store: backend.name, Method: "mutate", Records: count}
	assertStreamBatchMetrics(t, evidence)
	for index := range mutations {
		backend.assertPersisted(t, s.ctx, fmt.Sprintf("large_distinct_%03d", index), int64(10000+index))
	}
	before = s.nodeMetrics(t)
	readOptions := weir.ReadOptions{StoreName: backend.name, Requests: reads}
	rpc, cancel = context.WithTimeout(s.ctx, rpcTimeout)
	readResults, err := s.client.Read(rpc, readOptions)
	cancel()
	if err != nil || len(readResults) != count {
		t.Fatalf("513 distinct Read batch: results=%d err=%v", len(readResults), err)
	}
	for index, result := range readResults {
		backend.assertReadResult(t, result, nil, int64(10000+count-1-index))
	}
	evidence.Before, evidence.After, evidence.Method = before, s.nodeMetrics(t), "read"
	assertStreamBatchMetrics(t, evidence)
	t.Log("513 distinct native documents Put and Read in aggregation groups on one streamed call; every value independently persisted and reverse-order results verified")
}
