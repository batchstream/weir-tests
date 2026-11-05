//go:build integration && previous_sdk

package integration_test

import (
	"context"
	"fmt"
	"testing"

	weir "github.com/batchstream/weir-go"
	"github.com/batchstream/weir-tests/internal/fixture"
)

func readRecord(ctx context.Context, client *weir.Client, store string, request *weir.ReadRequest) (*weir.ReadResult, error) {
	options := weir.ReadOneOptions{StoreName: store, Request: request}
	return client.ReadOne(ctx, options)
}

func readRecords(ctx context.Context, client *weir.Client, store string, requests ...*weir.ReadRequest) ([]*weir.ReadResult, error) {
	options := weir.ReadOptions{StoreName: store, Requests: requests}
	return client.Read(ctx, options)
}

// mutateRecord uses the single-record method of the action, which shares streams.
func mutateRecord(ctx context.Context, client *weir.Client, store string, request *weir.MutateRequest) (*weir.MutationResult, error) {
	write := &weir.WriteRequest{Resource: request.Resource, Document: request.Document}
	writeOptions := weir.WriteOptions{StoreName: store, Request: write}
	switch request.Action {
	case weir.MutationCreate:
		return client.Create(ctx, writeOptions)
	case weir.MutationPut:
		return client.Put(ctx, writeOptions)
	case weir.MutationReplace:
		return client.Replace(ctx, writeOptions)
	case weir.MutationDelete:
		remove := &weir.DeleteRequest{Resource: request.Resource}
		options := weir.DeleteOptions{StoreName: store, Request: remove}
		return client.Delete(ctx, options)
	case weir.MutationAtomicTransform:
		transform := &weir.AtomicTransformRequest{Resource: request.Resource, Lua: request.Lua, BackendExpression: request.BackendExpression}
		options := weir.AtomicTransformOptions{StoreName: store, Request: transform}
		return client.AtomicTransform(ctx, options)
	}
	return nil, fmt.Errorf("mutation action %d has no single-record method", request.Action)
}

func mutateRecords(ctx context.Context, client *weir.Client, store string, requests ...*weir.MutateRequest) ([]*weir.MutationResult, error) {
	options := weir.MutateOptions{StoreName: store, Requests: requests}
	return client.Mutate(ctx, options)
}

// streamReads turns the previous SDK's indexes, which start at 1, into positions.
func streamReads(ctx context.Context, client *weir.Client, stream readStream) error {
	consume := func(ctx context.Context, index uint64, result *weir.ReadResult) error {
		return stream.consume(ctx, int(index)-1, result)
	}
	options := weir.ReadStreamOptions{StoreName: stream.store, Next: stream.next, Consume: consume}
	return client.ReadStream(ctx, options)
}

func runNative(ctx context.Context, client *weir.Client, call nativeCall) (*weir.NativeResult, error) {
	options := weir.NativeOptions{StoreName: call.store, Request: call.request, Consume: call.consume}
	return client.Native(ctx, options)
}

// testNativeWriteChangeEvidence lives with the benchmark workload, which the current
// SDK builds. It checks the databases, not an SDK.
func testNativeWriteChangeEvidence(t *testing.T, _ context.Context, _ *fixture.Cluster) {
	t.Skip("the benchmark workload needs the current SDK")
}
