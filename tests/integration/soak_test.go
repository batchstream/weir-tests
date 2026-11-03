//go:build integration

package integration_test

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/batchstream/weir-tests/internal/fixture"
	"github.com/batchstream/weir-tests/internal/soak"
)

// TestSoakEverySDKCall runs the soak workload with its default profiles against
// real backends without faults: every SDK call must succeed, without an error,
// and every outcome must match the databases.
func TestSoakEverySDKCall(t *testing.T) {
	binary := os.Getenv("WEIR_TEST_BINARY")
	if binary == "" {
		t.Fatal("integration tests require WEIR_TEST_BINARY pointing to a built Weir binary")
	}
	ctx, cancel := context.WithTimeout(t.Context(), 4*time.Minute)
	defer cancel()
	options := fixture.Options{WeirBinary: binary, MongoBinary: os.Getenv("WEIR_TEST_MONGODB_BINARY"), Backends: []string{"mongo", "search"}, Replicas: 2}
	cluster, err := fixture.Start(ctx, options)
	if err != nil {
		t.Fatal("start owned blackbox cluster:", err)
	}
	t.Cleanup(func() {
		cleanup, stop := context.WithTimeout(context.Background(), 75*time.Second)
		defer stop()
		if err := cluster.Close(cleanup); err != nil {
			t.Error("close owned fixtures:", err)
		}
	})
	shares, err := soak.ParseProfiles(soak.DefaultProfiles)
	if err != nil {
		t.Fatal(err)
	}
	mongo := soak.Backend{Kind: "mongo", Store: "mongo", URI: cluster.MongoURI, Workers: 20, KeysPerWorker: 64, Rate: 800}
	search := soak.Backend{Kind: "search", Store: "search", URI: cluster.SearchURL, Workers: 20, KeysPerWorker: 64, Rate: 800}
	output := filepath.Join(t.TempDir(), "soak")
	config := soak.Config{
		WeirAddress:      cluster.Address(),
		WeirDiagnostics:  cluster.Nodes[0].Diagnostics,
		Backends:         []soak.Backend{mongo, search},
		Duration:         20 * time.Second,
		SampleInterval:   5 * time.Second,
		OperationTimeout: 10 * time.Second,
		Settle:           time.Second,
		Profiles:         shares,
		BatchSize:        16,
		StreamLifetime:   4 * time.Second,
		ReadPercent:      30,
		VerifyPercent:    5,
		PaddingBytes:     256,
		NoErrors:         true,
		Output:           output,
	}
	runErr := soak.Run(ctx, config)
	raw, err := os.ReadFile(output + ".json")
	if err != nil {
		t.Fatal(runErr, err)
	}
	var report soak.Report
	if err := json.Unmarshal(raw, &report); err != nil {
		t.Fatal(err)
	}
	for kind, backend := range report.Backends {
		for api, counters := range backend.Totals.APIs {
			t.Logf("%s %s: calls=%d writes=%d applied=%d reads=%d read_failures=%d sdk_errors=%d timeouts=%d last_error=%q last_failure=%q",
				kind, api, counters.Calls, counters.Writes, counters.Applied, counters.Reads, counters.ReadFailures, counters.SDKErrors, counters.Timeouts, counters.LastSDKError, counters.LastFailure)
		}
	}
	if runErr != nil {
		t.Fatalf("%v; violations %+v", runErr, report.Kept)
	}
}
