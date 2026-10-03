package workload

import (
	"context"
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
	calls   atomic.Int64
	applied atomic.Int64
}

func (p *acknowledgementPeer) Mutate(ctx context.Context, request *pb.MutateBatchRequest) (*pb.MutateBatchResponse, error) {
	p.calls.Add(1)
	// Simulate durable writes before acknowledgement is lost. Neither the SDK
	// nor the benchmark adapter can infer these outcomes from an RPC error.
	p.applied.Add(int64(len(request.Requests)))
	switch p.mode {
	case "lost_after_apply":
		return nil, status.Error(codes.Unavailable, "entire batch acknowledgement lost")
	case "deadline_after_apply", "canceled_after_apply":
		<-ctx.Done()
		return nil, status.FromContextError(ctx.Err()).Err()
	}
	results := make([]*pb.MutationResult, len(request.Requests))
	for index := range results {
		failure := protocol.Fail(pb.FailureCode_UNAVAILABLE, "write applied but replica confirmation failed")
		results[index] = protocol.Mutation(pb.MutationOutcome_APPLIED, failure)
	}
	response := &pb.MutateBatchResponse{Results: results}
	return response, nil
}

func mutationTransport(t *testing.T, peer *acknowledgementPeer) pb.StoreServiceClient {
	t.Helper()
	listener := bufconn.Listen(1 << 20)
	server := grpc.NewServer()
	pb.RegisterStoreServiceServer(server, peer)
	done := make(chan error, 1)
	go func() { done <- server.Serve(listener) }()
	t.Cleanup(func() { server.Stop(); listener.Close(); <-done })
	dialOptions := []grpc.DialOption{
		grpc.WithTransportCredentials(insecure.NewCredentials()), grpc.WithNoProxy(),
		grpc.WithDisableRetry(), grpc.WithDisableServiceConfig(),
		grpc.WithContextDialer(func(ctx context.Context, _ string) (net.Conn, error) { return listener.DialContext(ctx) }),
	}
	connection, err := grpc.NewClient("passthrough:///offline-weir", dialOptions...)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { connection.Close() })
	return pb.NewStoreServiceClient(connection)
}

func TestSDKUnaryMutationAcknowledgementAndNoReplay(t *testing.T) {
	for _, mode := range []string{"applied_failure", "lost_after_apply", "deadline_after_apply", "canceled_after_apply"} {
		t.Run(mode, func(t *testing.T) {
			peer := &acknowledgementPeer{mode: mode}
			client := mutationTransport(t, peer)
			document := &weir.Document{MediaType: "application/json", Data: []byte(`{"n":1}`)}
			first := &weir.MutateRequest{Resource: "records/s:first", Action: weir.MutationPut, Document: document}
			second := &weir.MutateRequest{Resource: "records/s:second", Action: weir.MutationPut, Document: document}
			opts := weir.MutateOptions{StoreName: "search", Requests: []*weir.MutateRequest{first, second}}
			ctx, cancel := context.WithTimeout(t.Context(), time.Second)
			defer cancel()
			if mode == "canceled_after_apply" {
				go func() {
					for peer.applied.Load() == 0 && ctx.Err() == nil {
						time.Sleep(time.Millisecond)
					}
					cancel()
				}()
			}
			replies, rpcErr := weir.Mutate(ctx, client, opts)
			if len(replies) != 2 || peer.calls.Load() != 1 || peer.applied.Load() != 2 {
				t.Fatalf("whole batch lost accounting or replayed: replies=%v calls=%d applied=%d err=%v", replies, peer.calls.Load(), peer.applied.Load(), rpcErr)
			}
			if mode == "applied_failure" && rpcErr != nil || mode != "applied_failure" && rpcErr == nil {
				t.Fatal("unexpected final status", rpcErr)
			}
			for _, reply := range replies {
				outcome := mutationOutcome(reply, len(document.Data), rpcErr)
				if mode == "applied_failure" {
					if outcome.Status != Failed || !outcome.Applied || outcome.RequestBytes != uint64(len(document.Data)) || outcome.Error == "" {
						t.Fatal("confirmed APPLIED business failure lost evidence", reply, outcome)
					}
				} else if reply != nil || outcome.Status != Indeterminate || outcome.Applied || outcome.RequestBytes != 0 || outcome.Error == "" {
					t.Fatal("failed unary RPC invented a partial acknowledgement", reply, outcome)
				}
			}
		})
	}
}

func TestSDKUnaryMutationPreflightSendsNoBatch(t *testing.T) {
	peer := &acknowledgementPeer{mode: "applied_failure"}
	client := mutationTransport(t, peer)
	document := &weir.Document{MediaType: "application/json", Data: []byte(`{"n":1}`)}
	first := &weir.MutateRequest{Resource: "records/s:first", Action: weir.MutationPut, Document: document}
	invalid := &weir.MutateRequest{Resource: "records/s:second", Action: 0, Document: document}
	opts := weir.MutateOptions{StoreName: "search", Requests: []*weir.MutateRequest{first, invalid}}
	replies, err := weir.Mutate(t.Context(), client, opts)
	if err == nil || replies != nil || peer.calls.Load() != 0 || peer.applied.Load() != 0 {
		t.Fatalf("late-invalid batch reached backend: replies=%v calls=%d err=%v", replies, peer.calls.Load(), err)
	}
}
