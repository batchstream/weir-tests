//go:build integration

package integration_test

import (
	"context"
	"testing"

	weir "github.com/batchstream/weir-go"
)

func readRecord(ctx context.Context, client *weir.Client, store string, request *weir.ReadRequest) (*weir.ReadResult, error) {
	results, err := client.Read(ctx, store, weir.WithRequests(request))
	return results[0], err
}

func mutateRecord(ctx context.Context, client *weir.Client, store string, request *weir.MutateRequest) (*weir.MutationResult, error) {
	results, err := client.Mutate(ctx, store, weir.WithRequests(request))
	return results[0], err
}

// nativeCall is one Native call whose body goes to consume.
type nativeCall struct {
	store   string
	request *weir.NativeRequest
	consume func(context.Context, *weir.NativeResponse, []byte) error
}

type nativeInvoke func(context.Context, nativeCall) (*weir.NativeResult, error)

func runNative(ctx context.Context, client *weir.Client, call nativeCall) (*weir.NativeResult, error) {
	return client.Native(ctx, call.store, call.request, weir.WithChunks(call.consume))
}

// clientNative sends Native calls through client.
func clientNative(client *weir.Client) nativeInvoke {
	invoke := func(ctx context.Context, call nativeCall) (*weir.NativeResult, error) {
		return runNative(ctx, client, call)
	}
	return invoke
}

// replicaClient returns a Client of one replica, whose address resolves to it alone.
func replicaClient(t *testing.T, address string) *weir.Client {
	t.Helper()
	client, err := weir.New(address)
	if err != nil {
		t.Fatal("create SDK client for a replica:", err)
	}
	t.Cleanup(func() {
		if err := client.Close(); err != nil {
			t.Error("close SDK client of a replica:", err)
		}
	})
	return client
}
