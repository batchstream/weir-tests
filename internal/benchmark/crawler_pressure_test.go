package benchmark

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestCrawlerPressureConcurrencyBoundsBeforeFixtureStartup(t *testing.T) {
	cases := []struct {
		name               string
		workers, processes int
		valid              bool
	}{
		{name: "512 across four", workers: 512, processes: 4, valid: true},
		{name: "2048 across four", workers: 2048, processes: 4, valid: true},
		{name: "2048 across eight", workers: 2048, processes: 8, valid: true},
		{name: "512 in one", workers: 512, processes: 1, valid: true},
		{name: "over total", workers: 2052, processes: 4},
		{name: "over child", workers: 2048, processes: 2},
		{name: "one over child", workers: 513, processes: 1},
		{name: "uneven", workers: 513, processes: 4},
		{name: "too few", workers: 3, processes: 4},
		{name: "zero", workers: 0, processes: 4},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			missing := filepath.Join(t.TempDir(), "missing-server")
			opts := CrawlerOptions{Before: missing, After: missing, InputShape: "single", PageSize: 1, HTTPObserver: "disabled", Processes: test.processes, QueuePages: 1, Rounds: 1, Warmup: time.Second, Duration: time.Second, Timeout: time.Second, Profiles: []string{"two-step"}, Modes: []string{"fixed-rate"}, Rates: []int{96}, Concurrency: []int{test.workers}}
			_, err := RunCrawler(t.Context(), opts)
			if test.valid {
				if !errors.Is(err, os.ErrNotExist) {
					t.Fatal("valid concurrency did not reach immutable server validation", err)
				}
			} else if err == nil || !strings.Contains(err.Error(), "crawler concurrency") {
				t.Fatal("invalid concurrency was not rejected before fixture startup", err)
			}
		})
	}
}

func TestCrawlerPressureChildBoundsAndNativePoolReport(t *testing.T) {
	for _, threads := range []int{0, 128, 512, 513} {
		config := crawlerClientConfig{InputShape: "single", Path: "direct", Threads: threads, Processes: 4, PageSize: 1, QueuePages: 1, TimeoutNS: int64(time.Second)}
		raw, err := json.Marshal(config)
		if err != nil {
			t.Fatal(err)
		}
		var output bytes.Buffer
		err = RunCrawlerWorker(t.Context(), bytes.NewReader(raw), &output)
		if threads < 1 || threads > 512 {
			if err == nil || !strings.Contains(err.Error(), "client bounds") || output.Len() != 0 {
				t.Fatal("invalid child allocated a client or emitted readiness", threads, err, output.String())
			}
			continue
		}
		if !errors.Is(err, io.EOF) {
			t.Fatal("valid child did not wait for its first phase", threads, err)
		}
		var ready crawlerReply
		if err := json.Unmarshal(output.Bytes(), &ready); err != nil {
			t.Fatal(err)
		}
		if ready.Phase != "ready" || ready.InputShape != "single" || ready.NativePoolLimit != threads*3 {
			t.Fatal("child report lost the full native seller pool", threads, ready)
		}
	}
}

func TestCrawlerPressureWorkersHaveDistinctProcessOffsetsAndBoundedQueues(t *testing.T) {
	for _, total := range []int{512, 2048} {
		seen := make(map[int]bool, total)
		ctx, cancel := context.WithCancel(t.Context())
		cancel()
		for process := range 4 {
			threads := total / 4
			config := crawlerClientConfig{InputShape: "single", Path: "direct", Threads: threads, Processes: 4, ProcessIndex: process, Offset: process * threads, PageSize: 1, QueuePages: 1, TimeoutNS: int64(time.Second)}
			client, err := newCrawlerClient(ctx, config)
			if err != nil {
				t.Fatal(err)
			}
			transport := client.http.Transport.(*http.Transport)
			if transport.MaxConnsPerHost != threads*3 || transport.MaxIdleConnsPerHost != threads*3 || transport.MaxIdleConns != threads*3 {
				t.Fatal("large native pool was silently capped", total, process, transport)
			}
			phase := crawlerPhase{Phase: "timed", StartNS: time.Now().UnixNano(), DurationNS: 1, ProductsPerSecond: 1}
			results := client.measure(ctx, phase)
			client.close()
			if len(results) != threads {
				t.Fatal("worker allocation was truncated", total, process, len(results))
			}
			for index, result := range results {
				if result.Worker != process*threads+index || seen[result.Worker] || result.Offered != 0 || result.Failed != 0 || result.QueuePeak != 0 {
					t.Fatal("worker identity, cancellation or queue bound changed", total, process, result)
				}
				seen[result.Worker] = true
			}
		}
		if len(seen) != total {
			t.Fatal("process offsets omitted workers", total, len(seen))
		}
	}
}
