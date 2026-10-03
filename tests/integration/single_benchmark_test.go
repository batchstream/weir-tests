//go:build integration

package integration_test

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/batchstream/weir-tests/internal/benchmark"
	"github.com/batchstream/weir-tests/internal/fixture"
	"github.com/batchstream/weir-tests/internal/workload"
)

// This opt-in test proves actual client OS processes, independent native/SDK
// pools, one-record requests and persisted final state against both databases.
// It intentionally makes no assertion that Weir wins or saturates a database.
func TestIndependentSingleRequestClients(t *testing.T) {
	serverBinary := os.Getenv("WEIR_TEST_BINARY")
	if serverBinary == "" {
		t.Fatal("integration requires WEIR_TEST_BINARY")
	}
	ctx, cancel := context.WithTimeout(t.Context(), 4*time.Minute)
	defer cancel()
	repo, err := filepath.Abs("../..")
	if err != nil {
		t.Fatal(err)
	}
	binary := filepath.Join(t.TempDir(), "weir-lab")
	build := exec.CommandContext(ctx, filepath.Join(runtime.GOROOT(), "bin", "go"), "build", "-o", binary, "./cmd/weir-lab")
	build.Dir = repo
	if output, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build real client executable: %v\n%s", err, output)
	}
	options := fixture.Options{WeirBinary: serverBinary, MongoBinary: os.Getenv("WEIR_TEST_MONGODB_BINARY"), Backends: []string{"mongo", "search"}, OwnerCount: 1, StoreConcurrency: 2, BatchSize: 32, IngressSessions: 64, DatabaseCPUs: 1}
	cluster, err := fixture.Start(ctx, options)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("single-request fixture artifacts: %s", cluster.Directory)
	t.Cleanup(func() {
		cleanup, cancel := context.WithTimeout(context.Background(), 75*time.Second)
		defer cancel()
		if err := cluster.Close(cleanup); err != nil {
			t.Error(err)
		}
	})
	for _, backend := range options.Backends {
		t.Run(backend, func(t *testing.T) {
			namespace, err := workload.NewNamespace()
			if err != nil {
				t.Fatal(err)
			}
			config := workload.Config{Backend: backend, MongoURI: cluster.MongoURI, SearchURL: cluster.SearchURL, WeirSeed: cluster.Seed(), StoreName: backend, Namespace: namespace, Records: 32, PayloadBytes: 64, Concurrency: 4}
			dataset, err := workload.New(config)
			if err != nil {
				t.Fatal(err)
			}
			paths, err := workload.Open(ctx, dataset)
			if err != nil {
				t.Fatal(err)
			}
			defer func() {
				cleanup, cancel := context.WithTimeout(context.Background(), 15*time.Second)
				defer cancel()
				if err := paths.Cleanup(cleanup); err != nil {
					t.Error(err)
				}
				if err := paths.Close(); err != nil {
					t.Error(err)
				}
			}()
			params := benchmark.SaturationOptions{Dataset: dataset, Paths: paths, Concurrency: []int{2, 4}, BatchSizes: []int{1}, Warmup: time.Second, Duration: 5 * time.Second, Rounds: 1, WritePercent: 10, OperationTimeout: 10 * time.Second, Resources: cluster.Resources(backend), CPUThreshold: 90, ClientProcesses: 2, ClientExecutable: binary}
			report, err := benchmark.RunSaturation(ctx, params)
			if err != nil || report == nil || !report.Verified() {
				t.Fatal("real multiprocess single-request workload failed", err)
			}
			if len(report.BusinessComparisons) != 2 {
				t.Fatal("observed business performance missing")
			}
			for _, pair := range report.Pairs {
				for _, result := range []benchmark.SaturationResult{pair.Direct, pair.Weir} {
					if len(result.ClientProcesses) != 2 || result.Requests != result.Attempted || result.Latency.Samples != result.Attempted {
						t.Fatal("one-record request or latency accounting failed")
					}
					pids := make(map[int]bool)
					for _, client := range result.ClientProcesses {
						if client.PID == os.Getpid() || client.PID <= 0 || pids[client.PID] || !client.Exited || client.ExitError != "" || client.Requests != client.Result.Attempted {
							t.Fatal("independent owned process identity/cleanup failed", client)
						}
						pids[client.PID] = true
					}
				}
			}
		})
	}
}
