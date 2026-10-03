package workload

import (
	"context"
	"errors"
	"io"
	"net"
	"sync/atomic"
	"testing"
	"time"

	weir "github.com/batchstream/weir-go"
	"github.com/batchstream/weir-protocol/api/protocol"
	pb "github.com/batchstream/weir-protocol/api/weir/v1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"
	"google.golang.org/grpc/test/bufconn"
)

type acknowledgementPeer struct {
	pb.UnimplementedStoreServiceServer
	mode    string
	streams atomic.Int64
}

func (p *acknowledgementPeer) Execute(stream pb.StoreService_ExecuteServer) error {
	p.streams.Add(1)
	for {
		request, err := stream.Recv()
		if errors.Is(err, io.EOF) {
			if p.mode == "final_error" {
				return status.Error(codes.Unavailable, "final RPC status lost")
			}
			return nil
		}
		if err != nil {
			return err
		}
		var failure *pb.Failure
		if p.mode == "applied_failure" {
			failure = protocol.Fail(pb.FailureCode_UNAVAILABLE, "write applied but replica confirmation failed")
		}
		mutation := protocol.Mutation(pb.MutationOutcome_APPLIED, failure)
		resultVariant := &pb.Result_Mutation{Mutation: mutation}
		result := &pb.Result{Index: request.RequestId, Result: resultVariant}
		eventVariant := &pb.Event_Result{Result: result}
		event := &pb.Event{Version: 1, Value: eventVariant}
		raw, err := protocol.MarshalEvent(event)
		if err != nil {
			return err
		}
		frame := &pb.ExecuteResponse{RequestId: request.RequestId, EventFragment: raw}
		if err := stream.Send(frame); err != nil {
			return err
		}
		if p.mode == "partial_error" {
			return status.Error(codes.Unavailable, "RPC lost after first applied acknowledgement")
		}
		complete := &pb.ExecuteResponse{RequestId: request.RequestId, RequestComplete: true}
		if err := stream.Send(complete); err != nil {
			return err
		}
	}
}

func TestSDKMutationAdapterPreservesAppliedFailuresAndUnknownEntries(t *testing.T) {
	for _, mode := range []string{"applied_failure", "final_error", "partial_error"} {
		t.Run(mode, func(t *testing.T) {
			listener := bufconn.Listen(1 << 20)
			server := grpc.NewServer()
			peer := &acknowledgementPeer{mode: mode}
			pb.RegisterStoreServiceServer(server, peer)
			done := make(chan error, 1)
			go func() { done <- server.Serve(listener) }()
			t.Cleanup(func() { server.Stop(); listener.Close(); <-done })
			dialOptions := []grpc.DialOption{grpc.WithTransportCredentials(insecure.NewCredentials()), grpc.WithNoProxy(), grpc.WithDisableRetry(), grpc.WithContextDialer(func(ctx context.Context, _ string) (net.Conn, error) { return listener.DialContext(ctx) })}
			connection, err := grpc.NewClient("passthrough:///offline-weir", dialOptions...)
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { connection.Close() })
			client := pb.NewStoreServiceClient(connection)
			document := &weir.Document{MediaType: "application/json", Data: []byte(`{"n":1}`)}
			first := &weir.MutateRequest{Resource: "records/s:first", Action: weir.MutationPut, Document: document}
			second := &weir.MutateRequest{Resource: "records/s:second", Action: weir.MutationPut, Document: document}
			opts := weir.MutateOptions{StoreName: "search", Requests: []*weir.MutateRequest{first, second}}
			ctx, cancel := context.WithTimeout(t.Context(), 3*time.Second)
			defer cancel()
			replies, rpcErr := weir.Mutate(ctx, client, opts)
			if len(replies) != 2 || peer.streams.Load() != 1 {
				t.Fatal("batch lost accounting or replayed", len(replies), peer.streams.Load(), rpcErr)
			}
			if mode == "applied_failure" && rpcErr != nil || mode != "applied_failure" && rpcErr == nil {
				t.Fatal("unexpected final status", rpcErr)
			}
			for index, reply := range replies {
				outcome := mutationOutcome(reply, len(document.Data), rpcErr)
				if mode == "partial_error" && index == 1 {
					if reply != nil || outcome.Status != Indeterminate || outcome.Applied || outcome.RequestBytes != 0 {
						t.Fatal("unacknowledged write became applied", reply, outcome)
					}
					continue
				}
				if outcome.Status != Failed || !outcome.Applied || outcome.RequestBytes != uint64(len(document.Data)) || outcome.Error == "" {
					t.Fatal("APPLIED evidence disappeared behind business/transport error", reply, outcome)
				}
			}
		})
	}
}
