package soak

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/batchstream/weir-tests/internal/observe"
)

// PodSample is one Weir replica's diagnostics at a sample.
type PodSample struct {
	Address    string  `json:"address"`
	Goroutines float64 `json:"goroutines"`
	LiveBytes  float64 `json:"live_bytes"`
	RSSBytes   float64 `json:"rss_bytes"`
	OpenFDs    float64 `json:"open_fds"`
	Sessions   float64 `json:"sessions"`
	Overloaded float64 `json:"memory_overloaded"`
	CPUSeconds float64 `json:"cpu_seconds_total"`
	Error      string  `json:"error,omitempty"`
}

// ClientSample is this process's own runtime at a sample.
type ClientSample struct {
	Goroutines int    `json:"goroutines"`
	HeapBytes  uint64 `json:"heap_inuse_bytes"`
}

// Sample is one interval of the timeline.
type Sample struct {
	At          string              `json:"at_utc"`
	Elapsed     float64             `json:"elapsed_seconds"`
	Backends    map[string]Counters `json:"backends"`
	Connections map[string]int64    `json:"database_connections"`
	Weir        []PodSample         `json:"weir"`
	Client      ClientSample        `json:"client"`
	Violations  int                 `json:"violations"`
	Errors      []string            `json:"errors,omitempty"`
}

type sampler struct {
	config     Config
	runs       []*backendRun
	violations *violationLog
	output     io.Writer
}

func (s *sampler) run(ctx context.Context, stop <-chan struct{}) {
	started := time.Now()
	ticker := time.NewTicker(s.config.SampleInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-stop:
			return
		case <-ticker.C:
		}
		sample := s.sample(ctx, started)
		raw, err := json.Marshal(sample)
		if err == nil {
			_, err = s.output.Write(append(raw, '\n'))
		}
		if err != nil {
			fmt.Println("timeline write failed:", err)
		}
		fmt.Println(summarize(sample))
	}
}

func (s *sampler) sample(ctx context.Context, started time.Time) Sample {
	now := time.Now()
	sample := Sample{
		At:          now.UTC().Format(time.RFC3339),
		Elapsed:     now.Sub(started).Seconds(),
		Backends:    make(map[string]Counters),
		Connections: make(map[string]int64),
	}
	probe, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	for _, run := range s.runs {
		sample.Backends[run.config.Kind] = run.stats.rotate()
		connections, err := run.store.connections(probe)
		if err != nil {
			sample.Errors = append(sample.Errors, run.config.Kind+" connections: "+err.Error())
			continue
		}
		sample.Connections[run.config.Kind] = connections
	}
	pods, err := s.pods(probe)
	if err != nil {
		sample.Errors = append(sample.Errors, "Weir diagnostics: "+err.Error())
	}
	sample.Weir = pods
	var memory runtime.MemStats
	runtime.ReadMemStats(&memory)
	sample.Client = ClientSample{Goroutines: runtime.NumGoroutine(), HeapBytes: memory.HeapInuse}
	sample.Violations, _ = s.violations.snapshot()
	return sample
}

// pods resolves the diagnostics name to every Ready replica and scrapes each.
func (s *sampler) pods(ctx context.Context) ([]PodSample, error) {
	host, port, err := net.SplitHostPort(s.config.WeirDiagnostics)
	if err != nil {
		return nil, err
	}
	addresses, err := net.DefaultResolver.LookupHost(ctx, host)
	if err != nil {
		return nil, err
	}
	slices.Sort(addresses)
	pods := make([]PodSample, 0, len(addresses))
	for _, address := range addresses {
		endpoint := net.JoinHostPort(address, port)
		snapshot := observe.Fetch(ctx, endpoint, true)
		pod := PodSample{Address: endpoint, Error: snapshot.Error}
		if snapshot.Raw != "" {
			pod.Error = ""
			pod.Goroutines = metricSum(snapshot.Raw, "go_goroutines")
			pod.LiveBytes = metricSum(snapshot.Raw, "weir_memory_live_bytes")
			pod.RSSBytes = metricSum(snapshot.Raw, "process_resident_memory_bytes")
			pod.OpenFDs = metricSum(snapshot.Raw, "process_open_fds")
			pod.Sessions = metricSum(snapshot.Raw, "weir_ingress_sessions")
			pod.Overloaded = metricSum(snapshot.Raw, "weir_memory_overloaded")
			pod.CPUSeconds = metricSum(snapshot.Raw, "process_cpu_seconds_total")
		}
		pods = append(pods, pod)
	}
	return pods, nil
}

// metricSum adds every sample of one metric name in Prometheus text format.
func metricSum(raw, name string) float64 {
	var total float64
	for line := range strings.Lines(raw) {
		rest, ok := strings.CutPrefix(line, name)
		if !ok || rest == "" || rest[0] != ' ' && rest[0] != '{' {
			continue
		}
		fields := strings.Fields(rest[strings.LastIndexByte(rest, '}')+1:])
		if len(fields) == 0 {
			continue
		}
		value, err := strconv.ParseFloat(fields[0], 64)
		if err == nil {
			total += value
		}
	}
	return total
}

func summarize(sample Sample) string {
	var line strings.Builder
	fmt.Fprintf(&line, "%s +%.0fs", sample.At, sample.Elapsed)
	for _, kind := range []string{"mongo", "search"} {
		counters, ok := sample.Backends[kind]
		if !ok {
			continue
		}
		fmt.Fprintf(&line, " | %s w=%d r=%d applied=%d unknown=%d refused=%d errors=%d p99=%.0fms conns=%d",
			kind, counters.Writes, counters.Reads, counters.Applied, counters.Unknown,
			counters.NotApplied+counters.NotStarted, counters.SDKErrors, counters.WriteP99, sample.Connections[kind])
		if counters.LastSDKError != "" {
			fmt.Fprintf(&line, " (%s)", counters.LastSDKError)
		}
	}
	for _, pod := range sample.Weir {
		fmt.Fprintf(&line, " | %s g=%.0f live=%.0fMiB fds=%.0f", pod.Address, pod.Goroutines, pod.LiveBytes/(1<<20), pod.OpenFDs)
	}
	fmt.Fprintf(&line, " | violations=%d", sample.Violations)
	return line.String()
}
