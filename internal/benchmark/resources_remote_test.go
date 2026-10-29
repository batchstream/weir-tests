package benchmark

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/batchstream/weir-tests/internal/fixture"
)

func TestProcStatReadsCPUTicksAndResidentPages(t *testing.T) {
	// Field 2 may contain spaces and parentheses; fields 14, 15 and 24 follow it.
	raw := []byte("42 (weir (bench)) S 1 42 42 0 -1 4194560 100 0 0 0 250 50 0 0 20 0 9 0 100 1000000 300 18446744073709551615\n")
	cpu, rss, err := parseProcStat(raw, 4096)
	if err != nil || cpu != int64(3*time.Second) || rss != 300*4096 {
		t.Fatal("utime+stime ticks or RSS pages misread", cpu, rss, err)
	}
	for _, invalid := range []string{"", "42 weir S 1", "42 (weir) S 1 2 3"} {
		if _, _, err := parseProcStat([]byte(invalid), 4096); err == nil {
			t.Fatal("accepted malformed /proc stat", invalid)
		}
	}
}

func TestRemoteTargetSamplesDatabaseAndWeirCPUThroughTheirAPIs(t *testing.T) {
	processCPU := true
	search := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		process := ""
		if processCPU {
			process = `,"process":{"cpu":{"total_in_millis":1500}}`
		}
		fmt.Fprintf(writer, `{"nodes":{"remote":{"http":{"current_open":3},"transport":{"rx_size_in_bytes":1,"tx_size_in_bytes":2}%s}}}`, process)
	}))
	defer search.Close()
	weir := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		fmt.Fprint(writer, "process_cpu_seconds_total 2.5\nprocess_resident_memory_bytes 1e+06\nweir_store_records_total{store=\"search\"} 7\n")
	}))
	defer weir.Close()
	target := fixture.ResourceTarget{AllocatedCPUs: 2, Diagnostics: strings.TrimPrefix(weir.URL, "http://"), Remote: true}
	options := resourceSampleOptions{Target: target, Started: time.Now(), SearchURL: search.URL, SearchClient: search.Client()}
	sample := sampleResources(t.Context(), options)
	if sample.Error != "" || sample.CPUTimeNS == nil || *sample.CPUTimeNS != int64(1500*time.Millisecond) || sample.CPUClockNS == 0 {
		t.Fatal("remote database process CPU was not sampled with its read clock", sample.Error, sample.CPUTimeNS)
	}
	if sample.WeirCPUTimeNS == nil || *sample.WeirCPUTimeNS != int64(2500*time.Millisecond) || sample.WeirMemoryBytes != 1000000 {
		t.Fatal("Weir process CPU/RSS were not read from diagnostics", sample.WeirCPUTimeNS, sample.WeirMemoryBytes)
	}
	processCPU = false
	sample = sampleResources(t.Context(), options)
	if sample.CPUTimeNS != nil || !strings.Contains(sample.Error, "remote database process CPU") {
		t.Fatal("missing remote CPU counter became a valid interval endpoint", sample.Error)
	}
	owned := fixture.ResourceTarget{AllocatedCPUs: 2, Diagnostics: target.Diagnostics}
	options.Target = owned
	sample = sampleResources(t.Context(), options)
	if sample.WeirCPUTimeNS != nil || !strings.Contains(sample.Error, "identity unavailable") {
		t.Fatal("owned fixture target adopted remote CPU evidence", sample.Error)
	}
}
