//go:build integration && !previous_sdk

package integration_test

import (
	"context"

	weir "github.com/batchstream/weir-go"
)

func readRecord(ctx context.Context, client *weir.Client, store string, request *weir.ReadRequest) (*weir.ReadResult, error) {
	results, err := client.Read(ctx, store, weir.WithRequests(request))
	return results[0], err
}

func readRecords(ctx context.Context, client *weir.Client, store string, requests ...*weir.ReadRequest) ([]*weir.ReadResult, error) {
	return client.Read(ctx, store, weir.WithRequests(requests...))
}

func mutateRecord(ctx context.Context, client *weir.Client, store string, request *weir.MutateRequest) (*weir.MutationResult, error) {
	results, err := client.Mutate(ctx, store, weir.WithRequests(request))
	return results[0], err
}

func mutateRecords(ctx context.Context, client *weir.Client, store string, requests ...*weir.MutateRequest) ([]*weir.MutationResult, error) {
	return client.Mutate(ctx, store, weir.WithRequests(requests...))
}

func streamReads(ctx context.Context, client *weir.Client, stream readStream) error {
	_, err := client.Read(ctx, stream.store, weir.WithNext(stream.next), weir.WithReadConsume(stream.consume))
	return err
}

func runNative(ctx context.Context, client *weir.Client, call nativeCall) (*weir.NativeResult, error) {
	return client.Native(ctx, call.store, call.request, weir.WithChunks(call.consume))
}
