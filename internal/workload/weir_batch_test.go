package workload

import (
	"context"
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
	mode     string
	calls    atomic.Int64
	applied  atomic.Int64
	document []byte
}

func (p *acknowledgementPeer) Execute(stream grpc.BidiStreamingServer[pb.ExecuteRequest, pb.ExecuteResponse]) error {
	p.calls.Add(1)
	for {
		request, err := stream.Recv()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return err
		}
		if request.Command.GetRead() != nil {
			document := &pb.Document{MediaType: "application/json", Data: p.document}
			payload := &pb.ReadResult_Document{Document: document}
			result := &pb.ReadResult{Result: payload}
			value := &pb.Event_ReadResult{ReadResult: result}
			event := &pb.Event{Value: value}
			response := &pb.ExecuteResponse{Index: request.Index, Event: event}
			if err := stream.Send(response); err != nil {
				return err
			}
			return status.Error(codes.Unavailable, "later read result lost")
		}
		mutations := request.Command.GetMutate().Requests
		// Durable writes can precede lost acknowledgements; never infer replay safety.
		p.applied.Add(int64(len(mutations)))
		switch p.mode {
		case "lost_after_apply":
			return status.Error(codes.Unavailable, "window acknowledgement lost")
		case "deadline_after_apply", "canceled_after_apply":
			<-stream.Context().Done()
			return status.FromContextError(stream.Context().Err()).Err()
		}
		for index := range mutations {
			var failure *pb.Failure
			if p.mode == "applied_failure" {
				failure = protocol.Fail(pb.FailureCode_UNAVAILABLE, "write applied but replica confirmation failed")
			}
			result := protocol.Mutation(pb.MutationOutcome_APPLIED, failure)
			payload := &pb.Event_MutationResult{MutationResult: result}
			event := &pb.Event{Value: payload}
			response := &pb.ExecuteResponse{Index: request.Index + uint64(index), Event: event}
			if err := stream.Send(response); err != nil {
				return err
			}
			if p.mode == "partial_after_apply" {
				return status.Error(codes.Unavailable, "later acknowledgement lost")
			}
		}
	}
}

func TestSDKStreamReadPreservesConfirmedPrefixThroughLaterFailure(t *testing.T) {
	dataset := testDataset(t, "search")
	first := Operation{Record: 0}
	second := Operation{Record: 1}
	peer := &acknowledgementPeer{document: dataset.Document(first)}
	client := mutationTransport(t, peer)
	firstRequest := &weir.ReadRequest{Resource: dataset.Resource(first.Record)}
	secondRequest := &weir.ReadRequest{Resource: dataset.Resource(second.Record)}
	options := weir.ReadOptions{StoreName: "search", Requests: []*weir.ReadRequest{firstRequest, secondRequest}}
	replies, err := weir.Read(t.Context(), client, options)
	if err == nil || len(replies) != 2 || replies[0] == nil || replies[1] != nil || peer.calls.Load() != 1 {
		t.Fatal("unexpected partial read evidence", replies, err)
	}
	path := &weirPath{dataset: dataset}
	confirmed := path.readOutcome(replies[0], first, err)
	unknown := path.readOutcome(replies[1], second, err)
	if confirmed.Status != Success || confirmed.ResponseBytes == 0 || unknown.Status != Failed || unknown.ResponseBytes != 0 {
		t.Fatal("later stream error changed confirmed read accounting", confirmed, unknown)
	}
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

func TestSDKStreamMutationAcknowledgementAndNoReplay(t *testing.T) {
	for _, mode := range []string{"applied_failure", "partial_after_apply", "lost_after_apply", "deadline_after_apply", "canceled_after_apply"} {
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
			for index, reply := range replies {
				outcome := mutationOutcome(reply, len(document.Data), rpcErr)
				if mode == "applied_failure" {
					if outcome.Status != Failed || !outcome.Applied || outcome.RequestBytes != uint64(len(document.Data)) || outcome.Error == "" {
						t.Fatal("confirmed APPLIED business failure lost evidence", reply, outcome)
					}
				} else if mode == "partial_after_apply" && index == 0 {
					if outcome.Status != Success || !outcome.Applied || outcome.RequestBytes != uint64(len(document.Data)) || outcome.Error != "" {
						t.Fatal("later stream failure revoked a successful acknowledgement", reply, outcome)
					}
				} else if reply != nil || outcome.Status != Indeterminate || outcome.Applied || outcome.RequestBytes != 0 || outcome.Error == "" {
					t.Fatal("failed stream invented a partial acknowledgement", reply, outcome)
				}
			}
		})
	}
}

func TestSDKStreamMutationPreflightSendsNoBatch(t *testing.T) {
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
