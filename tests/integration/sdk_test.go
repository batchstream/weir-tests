//go:build integration

package integration_test

import (
	"context"
	"testing"

	weir "github.com/batchstream/weir-go"
)

// These tests run against the current SDK and, with the previous_sdk tag, the one
// before it. sdk_current_test.go and sdk_previous_test.go hold the calls whose API
// differs, with these types in common.

// readStream is a read call fed record by record, with results handed over as they
// complete. The index is the record's position in the input, from 0.
type readStream struct {
	store   string
	next    func(context.Context) (*weir.ReadRequest, error)
	consume func(ctx context.Context, index int, result *weir.ReadResult) error
}

// nativeCall is one Native call whose body goes to consume.
type nativeCall struct {
	store   string
	request *weir.NativeRequest
	consume func(context.Context, *weir.NativeResponse, []byte) error
}

type nativeInvoke func(context.Context, nativeCall) (*weir.NativeResult, error)

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
