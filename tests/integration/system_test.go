//go:build integration

package integration_test

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	weir "github.com/batchstream/weir-go"
	pb "github.com/batchstream/weir-protocol/api/weir/v1"
	"github.com/batchstream/weir-tests/internal/fixture"
	"github.com/batchstream/weir-tests/internal/observe"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"
)

const rpcTimeout = 5 * time.Second

type system struct {
	ctx      context.Context
	cluster  *fixture.Cluster
	client   *weir.Client
	backends []*backendData
}

// The integration tag is an explicit request for real services. Missing setup
// fails instead of turning a CI run into a successful collection of skips.
func TestSystemIntegration(t *testing.T) {
	binary := os.Getenv("WEIR_TEST_BINARY")
	if binary == "" {
		t.Fatal("integration tests require WEIR_TEST_BINARY pointing to a built Weir binary; run make prepare, then make integration")
	}
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Minute)
	defer cancel()
	options := fixture.Options{
		WeirBinary: binary, MongoBinary: os.Getenv("WEIR_TEST_MONGODB_BINARY"),
		Backends:  []string{"mongo", "search"},
		Replicas:  2,
		BatchSize: 513,
	}
	cluster, err := fixture.Start(ctx, options)
	if err != nil {
		t.Fatal("start owned blackbox cluster:", err)
	}
	t.Logf("fixture artifacts: %s", cluster.Directory)
	t.Cleanup(func() {
		cleanup, stop := context.WithTimeout(context.Background(), 75*time.Second)
		defer stop()
		if err := cluster.Close(cleanup); err != nil {
			t.Error("close owned fixtures:", err)
		}
	})
	if len(cluster.Nodes) != 2 {
		t.Fatal("fixture requires two replicas")
	}
	backends := provisionBackends(t, ctx, cluster)
	client, err := weir.New(cluster.Address())
	if err != nil {
		t.Fatal("create SDK client:", err)
	}
	t.Cleanup(func() {
		if err := client.Close(); err != nil {
			t.Error("close SDK:", err)
		}
	})
	suite := &system{ctx: ctx, cluster: cluster, client: client, backends: backends}
	if !t.Run("native_reset_and_write_change_evidence", func(t *testing.T) { testNativeWriteChangeEvidence(t, ctx, cluster) }) {
		return
	}
	for _, backend := range backends {
		if !t.Run(backend.name+"_lifecycle", func(t *testing.T) {
			testLifecycle(t, ctx, client, backend)
			testCrossReplicaNativePagination(t, ctx, cluster, backend)
			testPublicBatch(t, ctx, client, backend)
			suite.testLargeDistinctBatch(t, backend)
			suite.testStreamMemory(t, backend)
			suite.testLuaSemantics(t, backend)
			suite.testProjectionAndFailures(t, backend)
		}) {
			return
		}
	}
	if !t.Run("every_replica_serves_its_stores", func(t *testing.T) {
		suite.testReplicas(t)
	}) {
		return
	}
	if !t.Run("cancellation", func(t *testing.T) {
		suite.testCancellation(t)
	}) {
		return
	}
	t.Run("replica_stop_and_restart", func(t *testing.T) {
		suite.testRecovery(t)
	})
}

func transport(t *testing.T, address string) pb.StoreServiceClient {
	t.Helper()
	connection, err := grpc.NewClient(
		"passthrough:///"+address, grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithNoProxy(), grpc.WithDisableRetry(), grpc.WithDisableServiceConfig(),
	)
	if err != nil {
		t.Fatal("open public transport:", err)
	}
	t.Cleanup(func() {
		if err := connection.Close(); err != nil {
			t.Error("close public transport:", err)
		}
	})
	return pb.NewStoreServiceClient(connection)
}

func (s *system) testReplicas(t *testing.T) {
	t.Helper()
	ctx, cluster, backends := s.ctx, s.cluster, s.backends
	for _, node := range cluster.Nodes {
		client, err := weir.New(node.Application)
		if err != nil {
			t.Fatalf("create SDK client for %s: %v", node.Application, err)
		}
		for _, backend := range backends {
			want := record{id: "primary", n: 5}
			backend.assertRead(t, ctx, client, want)
		}
		if err := client.Close(); err != nil {
			t.Fatal(err)
		}
		wire := transport(t, node.Application)
		s.assertUnhostedStore(t, wire)
	}
	for _, backend := range backends {
		s.testServerPreflight(t, backend)
		s.testBatchSingleStream(t, backend)
	}
}

// A replica rejects a Store it does not host before any business response.
func (s *system) assertUnhostedStore(t *testing.T, wire pb.StoreServiceClient) {
	t.Helper()
	read := &pb.ReadRequest{Resource: s.backends[0].resource("primary")}
	operation := &pb.Command_Read{Read: read}
	command := &pb.Command{Operation: operation}
	request := &pb.ExecuteRequest{StoreName: "unconfigured", Index: 1, Command: command}
	rpc, cancel := context.WithTimeout(s.ctx, rpcTimeout)
	defer cancel()
	stream, err := wire.Execute(rpc)
	if err != nil {
		t.Fatal(err)
	}
	if err := stream.Send(request); err != nil {
		t.Fatal(err)
	}
	if err := stream.CloseSend(); err != nil {
		t.Fatal(err)
	}
	response, err := stream.Recv()
	if response != nil || status.Code(err) != codes.NotFound {
		t.Fatalf("unhosted Store Read: want NotFound and no business response, got response=%v err=%v", response, err)
	}
}

func (s *system) testCancellation(t *testing.T) {
	t.Helper()
	ctx, client, backend := s.ctx, s.client, s.backends[0]
	canceled, cancel := context.WithCancel(ctx)
	cancel()
	read := &weir.ReadRequest{Resource: backend.resource("primary")}
	request := weir.ReadOneOptions{StoreName: backend.name, Request: read}
	result, err := client.ReadOne(canceled, request)
	if result != nil || !cancellation(err) {
		t.Fatalf("canceled Read: result=%v err=%v", result, err)
	}
	write := backend.write(t, "canceled_create", 123)
	writeOptions := weir.WriteOptions{StoreName: backend.name, Request: write}
	mutation, err := client.Create(canceled, writeOptions)
	if mutation != nil || !cancellation(err) {
		t.Fatalf("canceled Create: result=%v err=%v", mutation, err)
	}
	backend.assertMissing(t, ctx, "canceled_create")
}

func cancellation(err error) bool {
	return errors.Is(err, context.Canceled) || status.Code(err) == codes.Canceled
}

func (s *system) testRecovery(t *testing.T) {
	t.Helper()
	ctx, cluster, client, backends := s.ctx, s.cluster, s.client, s.backends
	for _, backend := range backends {
		write := backend.write(t, "durable", 99)
		options := weir.WriteOptions{StoreName: backend.name, Request: write}
		rpc, cancel := context.WithTimeout(ctx, rpcTimeout)
		result, err := client.Create(rpc, options)
		cancel()
		assertApplied(t, result, err)
		backend.assertPersisted(t, ctx, "durable", 99)
	}
	survivor, err := weir.New(cluster.Nodes[1].Application)
	if err != nil {
		t.Fatal("create SDK client for the second replica:", err)
	}
	defer func() {
		if err := survivor.Close(); err != nil {
			t.Error("close second-replica SDK:", err)
		}
	}()
	control, cancel := context.WithTimeout(ctx, 20*time.Second)
	err = cluster.StopNode(control, 0)
	cancel()
	if err != nil {
		t.Fatal("stop first replica:", err)
	}
	for _, backend := range backends {
		want := record{id: "durable", n: 99}
		backend.assertRead(t, ctx, survivor, want)
		read := &weir.ReadRequest{Resource: backend.resource("durable")}
		readOptions := weir.ReadOneOptions{StoreName: backend.name, Request: read}
		rpc, cancel := context.WithTimeout(ctx, rpcTimeout)
		result, err := client.ReadOne(rpc, readOptions)
		cancel()
		if result != nil || status.Code(err) != codes.Unavailable {
			t.Fatalf("read through a stopped replica: want Unavailable, got result=%v err=%v", result, err)
		}
	}
	control, cancel = context.WithTimeout(ctx, 25*time.Second)
	err = cluster.RestartNode(control, 0)
	cancel()
	if err != nil {
		t.Fatal("restart first replica:", err)
	}
	for _, backend := range backends {
		want := record{id: "durable", n: 99}
		backend.waitRead(t, ctx, client, want)
	}
	t.Log("stopping one replica left the other serving persisted data; the original client reconnected to the restarted replica")
}

func poll(t *testing.T, parent context.Context, description string, check func(context.Context) error) {
	t.Helper()
	ctx, cancel := context.WithTimeout(parent, 30*time.Second)
	defer cancel()
	var last error
	for {
		attempt, stop := context.WithTimeout(ctx, time.Second)
		last = check(attempt)
		stop()
		if last == nil {
			return
		}
		timer := time.NewTimer(100 * time.Millisecond)
		select {
		case <-ctx.Done():
			timer.Stop()
			t.Fatalf("%s did not converge: %v (last observation: %v)", description, ctx.Err(), last)
		case <-timer.C:
		}
	}
}

func (s *system) nodeMetrics(t *testing.T) []observe.Snapshot {
	t.Helper()
	snapshots := make([]observe.Snapshot, len(s.cluster.Nodes))
	for index, node := range s.cluster.Nodes {
		snapshots[index] = observe.Fetch(s.ctx, node.Diagnostics)
		if snapshots[index].Error != "" {
			t.Fatalf("node %d diagnostic observation failed: %s", index, snapshots[index].Error)
		}
	}
	return snapshots
}

type streamBatchEvidence struct {
	Before, After []observe.Snapshot
	Store, Method string
	Records       int
}

func assertStreamBatchMetrics(t *testing.T, evidence streamBatchEvidence) {
	t.Helper()
	var replicas int
	var rpcCount float64
	labels := map[string]string{"store": evidence.Store, "operation": evidence.Method}
	storeLabels := map[string]string{"store": evidence.Store}
	methodLabels := map[string]string{"method": "execute"}
	for index, before := range evidence.Before {
		after := evidence.After[index]
		left, beforeFound := before.Sum("weir_store_records_total", labels)
		right, afterFound := after.Sum("weir_store_records_total", labels)
		if !beforeFound || !afterFound {
			t.Fatalf("replica %d terminal-record observation unavailable", index)
		}
		if right < left {
			t.Fatal("owned store metrics reset")
		}
		delta := right - left
		if delta != 0 {
			if delta != float64(evidence.Records) {
				t.Fatalf("record stream split across replicas: node=%d delta=%v", index, delta)
			}
			replicas++
			count, err := observe.Delta(before, after, "weir_store_batch_operations_count", storeLabels)
			if err != nil || count < 1 {
				t.Fatalf("aggregation invocations=%v error=%v", count, err)
			}
			operations, err := observe.Delta(before, after, "weir_store_batch_operations_sum", storeLabels)
			if err != nil || operations != float64(evidence.Records) {
				t.Fatalf("aggregated operations=%v error=%v", operations, err)
			}
		}
		left, beforeFound = before.Sum("weir_rpc_completions_total", methodLabels)
		right, afterFound = after.Sum("weir_rpc_completions_total", methodLabels)
		if !beforeFound || !afterFound {
			t.Fatalf("node %d streamed completion observation unavailable", index)
		}
		if right < left {
			t.Fatal("RPC completion metrics reset")
		}
		rpcCount += right - left
	}
	if replicas != 1 || rpcCount != 1 {
		t.Fatalf("one replica streams the batch: replicas=%d completed RPCs=%v", replicas, rpcCount)
	}
}

func (s *system) testBatchSingleStream(t *testing.T, backend *backendData) {
	t.Helper()
	before := s.nodeMetrics(t)
	reads := make([]*weir.ReadRequest, 32)
	for index := range reads {
		request := &weir.ReadRequest{Resource: backend.resource("primary")}
		reads[index] = request
	}
	opts := weir.ReadOptions{StoreName: backend.name, Requests: reads}
	rpc, cancel := context.WithTimeout(s.ctx, rpcTimeout)
	results, err := s.client.Read(rpc, opts)
	cancel()
	if err != nil || len(results) != len(reads) {
		t.Fatalf("streamed batch: results=%d err=%v", len(results), err)
	}
	for _, result := range results {
		backend.assertReadResult(t, result, nil, 5)
	}
	evidence := streamBatchEvidence{Before: before, After: s.nodeMetrics(t), Store: backend.name, Method: "read", Records: len(reads)}
	assertStreamBatchMetrics(t, evidence)
}
