//go:build integration

package integration_test

import (
	"context"
	"errors"
	"fmt"
	"os"
	"slices"
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
	ctx         context.Context
	cluster     *fixture.Cluster
	client      *weir.Client
	openOptions weir.OpenOptions
	backends    []*backendData
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
		Backends:   []string{"mongo", "search"},
		OwnerCount: 2, DiscoveryOnly: true, StoreConcurrency: 2,
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
	if len(cluster.Nodes) != 3 || !cluster.Nodes[0].Owner || !cluster.Nodes[1].Owner || cluster.Nodes[2].Owner {
		t.Fatal("fixture requires two owners followed by one discovery-only node")
	}
	backends := provisionBackends(t, ctx, cluster)
	openOptions := weir.OpenOptions{
		Seed: cluster.Seed(), Stores: []string{"mongo", "search"},
		RefreshInterval: 100 * time.Millisecond, ResolveTimeout: time.Second,
	}
	opening, stopOpening := context.WithTimeout(ctx, 15*time.Second)
	client, err := weir.Open(opening, openOptions)
	stopOpening()
	if err != nil {
		t.Fatal("initialize through discovery-only node:", err)
	}
	t.Cleanup(func() {
		if err := client.Close(); err != nil {
			t.Error("close SDK:", err)
		}
	})
	suite := &system{ctx: ctx, cluster: cluster, client: client, openOptions: openOptions, backends: backends}
	if !t.Run("native_reset_and_write_change_evidence", func(t *testing.T) { testNativeWriteChangeEvidence(t, ctx, cluster) }) {
		return
	}
	for _, backend := range backends {
		if !t.Run(backend.name+"_lifecycle", func(t *testing.T) {
			testLifecycle(t, ctx, client, backend)
			testCrossOwnerScan(t, ctx, cluster, backend)
			testPublicBatch(t, ctx, client, backend)
			suite.testLargeDistinctBatch(t, backend)
			suite.testStreamMemory(t, backend)
		}) {
			return
		}
	}
	if !t.Run("any_node_discovery_and_no_relay", func(t *testing.T) {
		suite.testDiscovery(t)
	}) {
		return
	}
	if !t.Run("cancellation", func(t *testing.T) {
		suite.testCancellation(t)
	}) {
		return
	}
	t.Run("owner_withdrawal_restart_and_seed_loss", func(t *testing.T) {
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

func (s *system) testDiscovery(t *testing.T) {
	t.Helper()
	ctx, cluster, backends := s.ctx, s.cluster, s.backends
	want := []string{cluster.Nodes[0].Application, cluster.Nodes[1].Application}
	for _, node := range cluster.Nodes {
		wire := transport(t, node.Application)
		for _, backend := range backends {
			s.waitResolve(t, wire, backend.name, want)
		}
		options := weir.OpenOptions{Seed: node.Application, Stores: []string{"mongo", "search"}, ResolveTimeout: time.Second}
		opening, cancel := context.WithTimeout(ctx, 10*time.Second)
		client, err := weir.Open(opening, options)
		cancel()
		if err != nil {
			t.Fatalf("initialize through %s: %v", node.Application, err)
		}
		for _, backend := range backends {
			want := record{id: "primary", n: 5}
			backend.assertRead(t, ctx, client, want)
		}
		if err := client.Close(); err != nil {
			t.Fatal(err)
		}
	}
	wire := transport(t, cluster.Nodes[2].Application)
	request := &pb.ResolveStoreRequest{StoreName: "unconfigured"}
	rpc, cancel := context.WithTimeout(ctx, rpcTimeout)
	response, err := wire.ResolveStore(rpc, request)
	cancel()
	// An eventually synchronized directory cannot prove a Store does not exist
	// globally. Missing live announcements are retryable and expose no endpoint.
	if status.Code(err) != codes.Unavailable || len(response.GetEndpoints()) != 0 {
		t.Fatalf("unannounced store ResolveStore: want Unavailable and no endpoints, got response=%v err=%v", response, err)
	}
	for _, backend := range backends {
		s.testServerPreflight(t, backend)
		s.testBatchOwnerRouting(t, backend)
		read := &pb.ReadRequest{Resource: backend.resource("primary")}
		operation := &pb.Command_Read{Read: read}
		command := &pb.Command{Operation: operation}
		request := &pb.ExecuteRequest{StoreName: backend.name, Index: 1, Command: command}
		rpc, cancel := context.WithTimeout(ctx, rpcTimeout)
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
		cancel()
		if response != nil || status.Code(err) != codes.Unavailable {
			t.Fatalf("non-owner %s streamed Read: want Unavailable and no business response, got response=%v err=%v", backend.name, response, err)
		}
	}
}

func (s *system) waitResolve(t *testing.T, wire pb.StoreServiceClient, store string, want []string) {
	t.Helper()
	expected := slices.Clone(want)
	slices.Sort(expected)
	poll(t, s.ctx, "ResolveStore "+store, func(attempt context.Context) error {
		request := &pb.ResolveStoreRequest{StoreName: store}
		response, err := wire.ResolveStore(attempt, request)
		if err != nil {
			return err
		}
		actual := slices.Clone(response.GetEndpoints())
		slices.Sort(actual)
		if response.GetStoreName() != store || response.GetCacheTtlMs() == 0 || !slices.Equal(actual, expected) {
			return fmt.Errorf("got store=%q ttl=%d endpoints=%v; want %v", response.GetStoreName(), response.GetCacheTtlMs(), actual, expected)
		}
		return nil
	})
}

func (s *system) testCancellation(t *testing.T) {
	t.Helper()
	ctx, client, options, backend := s.ctx, s.client, s.openOptions, s.backends[0]
	canceled, cancel := context.WithCancel(ctx)
	cancel()
	read := &weir.ReadRequest{Resource: backend.resource("primary")}
	request := weir.ReadOneOptions{StoreName: backend.name, Request: read}
	result, err := client.ReadOne(canceled, request)
	if result != nil || !cancellation(err) {
		t.Fatalf("canceled Read: result=%v err=%v", result, err)
	}
	opened, err := weir.Open(canceled, options)
	if opened != nil {
		_ = opened.Close()
		t.Fatal("canceled initialization returned a live SDK")
	}
	if !cancellation(err) {
		t.Fatal("canceled initialization:", err)
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
	control, cancel := context.WithTimeout(ctx, 20*time.Second)
	err := cluster.StopNode(control, 0)
	cancel()
	if err != nil {
		t.Fatal("withdraw owner:", err)
	}
	wire := transport(t, cluster.Nodes[2].Application)
	for _, backend := range backends {
		s.waitResolve(t, wire, backend.name, []string{cluster.Nodes[1].Application})
		want := record{id: "durable", n: 99}
		backend.waitRead(t, ctx, client, want)
	}
	options := weir.OpenOptions{Seed: cluster.Seed(), Stores: []string{"mongo", "search"}, ResolveTimeout: time.Second}
	opening, stopOpening := context.WithTimeout(ctx, 10*time.Second)
	recreated, err := weir.Open(opening, options)
	stopOpening()
	if err != nil {
		t.Fatal("initialize with one surviving owner:", err)
	}
	defer func() {
		if err := recreated.Close(); err != nil {
			t.Error("close recreated SDK:", err)
		}
	}()
	for _, backend := range backends {
		want := record{id: "durable", n: 99}
		backend.assertRead(t, ctx, recreated, want)
	}
	control, cancel = context.WithTimeout(ctx, 25*time.Second)
	err = cluster.RestartNode(control, 0)
	cancel()
	if err != nil {
		t.Fatal("restart owner:", err)
	}
	want := []string{cluster.Nodes[0].Application, cluster.Nodes[1].Application}
	for _, backend := range backends {
		s.waitResolve(t, wire, backend.name, want)
		backend.assertPersisted(t, ctx, "durable", 99)
		owner := transport(t, cluster.Nodes[0].Application)
		read := &weir.ReadRequest{Resource: backend.resource("durable")}
		readOptions := weir.ReadOneOptions{StoreName: backend.name, Request: read}
		rpc, cancel := context.WithTimeout(ctx, rpcTimeout)
		result, err := weir.ReadOne(rpc, owner, readOptions)
		cancel()
		backend.assertReadResult(t, result, err, 99)
	}
	control, cancel = context.WithTimeout(ctx, 20*time.Second)
	err = cluster.StopNode(control, 2)
	cancel()
	if err != nil {
		t.Fatal("stop initialization node:", err)
	}
	for _, backend := range backends {
		want := record{id: "durable", n: 99}
		backend.waitRead(t, ctx, client, want)
	}
	t.Log("graceful owner withdrawal converged; existing and recreated SDK clients read persisted data; restarted owner recovered; losing the seed did not interrupt direct business reads")
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
	var owners int
	var rpcCount float64
	labels := map[string]string{"store": evidence.Store, "operation": evidence.Method}
	storeLabels := map[string]string{"store": evidence.Store}
	methodLabels := map[string]string{"method": "execute"}
	for index, before := range evidence.Before {
		after := evidence.After[index]
		left, beforeFound := before.Sum("weir_store_records_total", labels)
		right, afterFound := after.Sum("weir_store_records_total", labels)
		if index < len(evidence.Before)-1 && (!beforeFound || !afterFound) {
			t.Fatalf("owner %d terminal-record observation unavailable", index)
		}
		if index == len(evidence.Before)-1 && (beforeFound || afterFound) {
			t.Fatal("discovery-only node unexpectedly exposes a local Store")
		}
		if right < left {
			t.Fatal("owned store metrics reset")
		}
		delta := right - left
		if delta != 0 {
			if index == len(evidence.Before)-1 || delta != float64(evidence.Records) {
				t.Fatalf("record stream relayed or split across owners: node=%d delta=%v", index, delta)
			}
			owners++
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
	if owners != 1 || rpcCount != 1 {
		t.Fatalf("single owner streamed batch: owners=%d completed RPCs=%v", owners, rpcCount)
	}
}

func (s *system) testBatchOwnerRouting(t *testing.T, backend *backendData) {
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
		t.Fatalf("routed streamed batch: results=%d err=%v", len(results), err)
	}
	for _, result := range results {
		backend.assertReadResult(t, result, nil, 5)
	}
	evidence := streamBatchEvidence{Before: before, After: s.nodeMetrics(t), Store: backend.name, Method: "read", Records: len(reads)}
	assertStreamBatchMetrics(t, evidence)
}
