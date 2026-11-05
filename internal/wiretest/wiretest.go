// Package wiretest gives tests a Client connected in memory to a server of their
// own, to check what an SDK call puts on the wire. An SDK option cannot be inspected
// outside the SDK, so the server is where a test sees its effect.
package wiretest

import (
	"context"
	"errors"
	"io"
	"net"
	"slices"
	"sync"
	"testing"

	weir "github.com/batchstream/weir-go"
	"github.com/batchstream/weir-protocol/api/protocol"
	pb "github.com/batchstream/weir-protocol/api/weir/v1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/grpc/test/bufconn"
)

// Client returns a Client whose only server is server.
func Client(t *testing.T, server pb.StoreServiceServer) *weir.Client {
	t.Helper()
	listener := bufconn.Listen(1 << 20)
	serving := grpc.NewServer()
	pb.RegisterStoreServiceServer(serving, server)
	done := make(chan error, 1)
	go func() { done <- serving.Serve(listener) }()
	t.Cleanup(func() {
		serving.Stop()
		listener.Close()
		<-done
	})
	dial := func(ctx context.Context, _ string) (net.Conn, error) { return listener.DialContext(ctx) }
	// The address only labels the server the dialer reaches.
	client, err := weir.New("127.0.0.1:1", weir.WithDialOptions(grpc.WithContextDialer(dial)))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { client.Close() })
	return client
}

// Recorder is a server that applies every mutation and keeps what it received.
type Recorder struct {
	mu        sync.Mutex
	mutations []*pb.MutateRequest
}

// Execute answers each mutation as applied and refuses anything else.
func (r *Recorder) Execute(stream grpc.BidiStreamingServer[pb.ExecuteRequests, pb.ExecuteResponses]) error {
	for {
		message, err := stream.Recv()
		if errors.Is(err, io.EOF) {
			return nil
		}
		if err != nil {
			return err
		}
		for _, request := range message.Requests {
			mutation := request.Command.GetMutate()
			if mutation == nil {
				return status.Error(codes.InvalidArgument, "mutation required")
			}
			r.mu.Lock()
			r.mutations = append(r.mutations, mutation)
			r.mu.Unlock()
			result := protocol.Mutation(pb.MutationOutcome_APPLIED, nil)
			payload := &pb.Event_MutationResult{MutationResult: result}
			event := &pb.Event{Value: payload}
			response := &pb.ExecuteResponse{Index: request.Index, Event: event}
			reply := &pb.ExecuteResponses{Responses: []*pb.ExecuteResponse{response}}
			if err := stream.Send(reply); err != nil {
				return err
			}
		}
	}
}

// Mutations returns the mutations received so far, in order of arrival.
func (r *Recorder) Mutations() []*pb.MutateRequest {
	r.mu.Lock()
	defer r.mu.Unlock()
	return slices.Clone(r.mutations)
}

// QueryVisible reports whether a mutation waits for query visibility.
func QueryVisible(mutation *pb.MutateRequest) bool {
	return mutation.GetOptions().GetVisibility() == pb.MutationVisibility_QUERY_VISIBLE
}
