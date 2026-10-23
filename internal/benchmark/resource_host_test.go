package benchmark

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/batchstream/weir-tests/internal/fixture"
)

func TestHostSnapshotPreservesTicksAffinityAndPressureAvailability(t *testing.T) {
	proc := t.TempDir()
	for _, directory := range []string{"self", "pressure"} {
		if err := os.Mkdir(filepath.Join(proc, directory), 0700); err != nil {
			t.Fatal(err)
		}
	}
	files := map[string]string{
		"stat":            "cpu 10 2 3 20 4 5 6 7 100 200\ncpu0 1 0 0 0\ncpu1 1 0 0 0\ncpu2 1 0 0 0\ncpu3 1 0 0 0\n",
		"self/status":     "Name: sampler\nCpus_allowed_list:\t0-1,3\n",
		"pressure/cpu":    "some avg10=0.00 avg60=1.25 avg300=0.00 total=12\n",
		"pressure/io":     "some avg10=0.00 avg60=0.00 avg300=0.00 total=0\nfull avg10=0.00 avg60=0.00 avg300=0.00 total=0\n",
		"pressure/memory": "some avg10=NaN avg60=0 avg300=0 total=0\n",
	}
	for name, value := range files {
		if err := os.WriteFile(filepath.Join(proc, name), []byte(value), 0600); err != nil {
			t.Fatal(err)
		}
	}
	sample := sampleHostResources(proc)
	if sample.Unavailable != "" || sample.CPU == nil || sample.CPU.TotalTicks != 57 || sample.CPU.BusyTicks != 26 || sample.CPU.IdleTicks != 20 || sample.CPU.IOWaitTicks != 4 || sample.CPU.StealTicks != 7 || sample.CPU.CPUsListed != 4 || sample.AllowedCPUs == nil || *sample.AllowedCPUs != 3 || sample.AllowedCPUList != "0-1,3" || sample.AtUnixNS <= 0 || sample.ReadElapsedNS <= 0 {
		t.Fatal("host tick accounting double-counted guest work or lost affinity", sample)
	}
	if sample.Pressure["cpu"].Some == nil || sample.Pressure["cpu"].Some.TotalUS != 12 || sample.Pressure["cpu"].Full != nil || sample.Pressure["cpu"].Unavailable == "" || sample.Pressure["io"].Full == nil || sample.Pressure["io"].Full.TotalUS != 0 || sample.Pressure["io"].Unavailable != "" || sample.Pressure["memory"].Unavailable == "" {
		t.Fatal("missing and malformed pressure must differ from observed zero", sample.Pressure)
	}
	missing := sampleHostResources(t.TempDir())
	if missing.CPU != nil || missing.AllowedCPUs != nil || missing.Unavailable == "" || missing.Pressure["io"].Unavailable == "" {
		t.Fatal("missing Linux observations silently became measured zero", missing)
	}
}

func TestHostMeasurementRejectsMalformedCountersAndOverlappingAffinity(t *testing.T) {
	for _, value := range []string{"", "cpu 1 2\ncpu0 1\n", "cpu 18446744073709551615 1 0 0 0 0 0 0\ncpu0 1\n", "cpu 1 0 0 0 0 0 0 0\n"} {
		if _, err := parseHostCPU([]byte(value)); err == nil {
			t.Fatal("invalid host tick denominator accepted", value)
		}
	}
	for _, value := range []string{"", "0-2,2", "3-1", "-1", "0,0", "0-1000000", "0-1-2"} {
		if _, err := countCPUList(value); err == nil {
			t.Fatal("ambiguous CPU affinity accepted", value)
		}
	}
	for _, value := range []string{"full avg10=0 avg60=0 avg300=0 total=0", "some avg10=0 avg60=0 avg300=0 total=-1", "some avg10=0 avg60=0 avg300=0 total=1\nsome avg10=0 avg60=0 avg300=0 total=1", "some avg10=101 avg60=0 avg300=0 total=1"} {
		if _, err := parsePressure([]byte(value)); err == nil {
			t.Fatal("invalid cumulative pressure accepted", value)
		}
	}
}

func TestDockerThrottlingDistinguishesObservedZeroAndMissingCounters(t *testing.T) {
	for _, throttle := range []string{`"throttling_data":{"periods":8,"throttled_periods":0,"throttled_time":0},`, ""} {
		server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
			fmt.Fprintf(writer, `{"read":"2026-10-16T00:00:00Z","cpu_stats":{%s"cpu_usage":{"total_usage":123}}}`, throttle)
		}))
		client := server.Client()
		transport := client.Transport.(*http.Transport).Clone()
		transport.Proxy = func(*http.Request) (*url.URL, error) { return url.Parse(server.URL) }
		client.Transport = transport
		sample, err := sampleDocker(context.Background(), client, strings.Repeat("a", 64))
		client.CloseIdleConnections()
		server.Close()
		if err != nil || sample.CPUThrottling == nil {
			t.Fatal("CPU/throttling evidence missing", sample, err)
		}
		observed := sample.CPUThrottling
		if throttle != "" {
			if observed.Unavailable != "" || observed.Periods == nil || *observed.Periods != 8 || observed.ThrottledPeriods == nil || *observed.ThrottledPeriods != 0 || observed.ThrottledTimeNS == nil || *observed.ThrottledTimeNS != 0 {
				t.Fatal("observed zero throttling became unavailable", observed)
			}
		} else if observed.Unavailable == "" || observed.ThrottledTimeNS != nil {
			t.Fatal("absent throttling silently became zero", observed)
		}
	}
}

func TestResourceSampleRetainsOnlyFixedStoreGauges(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		fmt.Fprintln(writer, `weir_store_queued_operations{store="search"} 7`)
		fmt.Fprintln(writer, `weir_store_queued_bytes{store="search"} 512`)
		fmt.Fprintln(writer, `weir_store_active_executions{store="search"} 4`)
		fmt.Fprintln(writer, `weir_store_active_executions_limit{store="search"} 16`)
		fmt.Fprintln(writer, `weir_store_active_natives{store="search"} 1`)
		fmt.Fprintln(writer, `weir_store_active_natives_limit{store="search"} 64`)
		fmt.Fprintln(writer, `unrelated_counter 999`)
	}))
	defer server.Close()
	target := fixture.ResourceTarget{Diagnostics: strings.TrimPrefix(server.URL, "http://")}
	opts := resourceSampleOptions{Target: target, Started: time.Now()}
	sample := sampleResources(context.Background(), opts)
	if sample.StoreSampleError != "" || sample.StoreSampleAt == "" || len(sample.Store) != 6 {
		t.Fatal("fixed Store observations missing", sample.Store, sample.StoreSampleError)
	}
	for _, metric := range sample.Store {
		if metric.Name == "unrelated_counter" || metric.Labels["store"] != "search" {
			t.Fatal("unbounded/unowned metric projected into Store occupancy", metric)
		}
	}
}
