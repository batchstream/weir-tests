//go:build integration

package integration_test

import (
	"context"
	"testing"

	"github.com/batchstream/weir-tests/internal/fixture"
	"github.com/batchstream/weir-tests/internal/workload"
)

func testNativeWriteChangeEvidence(t *testing.T, ctx context.Context, cluster *fixture.Cluster) {
	t.Helper()
	namespace, err := workload.NewNamespace()
	if err != nil {
		t.Fatal(err)
	}
	config := workload.Config{Backend: "mongo", MongoURI: cluster.MongoURI, WeirSeed: cluster.Seed(), StoreName: "mongo", Namespace: namespace, Records: 64, PayloadBytes: 64, Concurrency: 2}
	dataset, err := workload.New(config)
	if err != nil {
		t.Fatal(err)
	}
	paths, err := workload.Open(ctx, dataset)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		cleanup, cancel := context.WithTimeout(context.Background(), rpcTimeout)
		defer cancel()
		if err := paths.Cleanup(cleanup); err != nil {
			t.Error("cleanup exclusive benchmark namespace:", err)
		}
		if err := paths.Close(); err != nil {
			t.Error("close benchmark clients:", err)
		}
	}()
	// Initial seed, repeated single-row reset, and a 64-row reset all remain
	// valid even when the persisted document is already revision zero.
	for _, batchSize := range []int{1, 1, 64, 64} {
		if err := paths.Prepare(ctx, batchSize); err != nil {
			t.Fatal("idempotent untimed preparation:", err)
		}
	}
	operations := make([]workload.Operation, 32)
	for index := range operations {
		operation := workload.Operation{Record: index, Write: true}
		operations[index] = operation
	}
	for _, outcome := range paths.Direct.ExecuteBatch(ctx, operations) {
		if outcome.Status == workload.Success {
			t.Fatal("matched-but-unmodified timed Mongo batch counted as throughput", outcome)
		}
	}
	for index := range operations {
		operations[index].Revision = 1
	}
	for _, outcome := range paths.Direct.ExecuteBatch(ctx, operations) {
		if outcome.Status != workload.Success || !outcome.Applied {
			t.Fatal("real native batch modifications rejected", outcome)
		}
	}
	plan := &workload.Plan{Expected: make([]int, 64)}
	for index := range 32 {
		plan.Expected[index] = 1
	}
	if err := paths.Verify(ctx, plan, 64); err != nil {
		t.Fatal("native bulk changes not independently persisted:", err)
	}
	if err := paths.Prepare(ctx, 64); err != nil {
		t.Fatal("reset after real changes:", err)
	}
	t.Log("repeated 1/64-record reset succeeds; native Mongo matched-but-unmodified timed writes are rejected; real 32-record bulk changes persist")
}
