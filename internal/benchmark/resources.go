package benchmark

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"net"
	"net/http"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"github.com/batchstream/weir-tests/internal/fixture"
	"github.com/batchstream/weir-tests/internal/workload"
)

type ResourceSample struct {
	At               string                    `json:"at_utc"`
	ElapsedNS        int64                     `json:"elapsed_ns"`
	CPUBudgetPercent *float64                  `json:"database_cpu_budget_percent,omitempty"`
	CPUTimeNS        *int64                    `json:"database_cpu_time_ns,omitempty"`
	CPUClockNS       int64                     `json:"database_cpu_counter_clock_unix_ns,omitempty"`
	MemoryBytes      uint64                    `json:"database_memory_bytes"`
	BlockReadBytes   uint64                    `json:"container_block_read_bytes"`
	BlockWriteBytes  uint64                    `json:"container_block_write_bytes"`
	NetworkInBytes   uint64                    `json:"container_network_in_bytes"`
	NetworkOutBytes  uint64                    `json:"container_network_out_bytes"`
	ClientCPUTimeNS  *int64                    `json:"client_cpu_time_ns,omitempty"`
	WeirCPUTimeNS    *int64                    `json:"weir_cpu_time_ns,omitempty"`
	Database         workload.DatabaseCounters `json:"database_counters"`
	Error            string                    `json:"error,omitempty"`
}

type Resources struct {
	AllocatedCPUs   float64          `json:"database_allocated_cpus"`
	Source          string           `json:"source"`
	Samples         []ResourceSample `json:"samples"`
	Intervals       int              `json:"valid_cpu_intervals"`
	MeanCPUPercent  float64          `json:"mean_database_cpu_budget_percent"`
	FullCPUFraction float64          `json:"fraction_intervals_at_cpu_threshold"`
	MeasurementNS   int64            `json:"measurement_elapsed_ns"`
	ValidCPUTimeNS  int64            `json:"valid_cpu_sampling_elapsed_ns"`
	Coverage        float64          `json:"cpu_sampling_time_coverage"`
	CPUSaturated    bool             `json:"sustained_cpu_saturation"`
	Missing         string           `json:"unavailable,omitempty"`
}

type DatabaseCPUQuota struct {
	ContainerID   string  `json:"container_id"`
	NanoCPUs      int64   `json:"host_config_nano_cpus"`
	RequestedCPUs float64 `json:"requested_cpus"`
	ObservedCPUs  float64 `json:"observed_cpus"`
}

func verifyResourceBudget(ctx context.Context, target fixture.ResourceTarget) (*DatabaseCPUQuota, error) {
	if target.Container == "" {
		return nil, nil
	}
	if target.AllocatedCPUs <= 0 || math.IsNaN(target.AllocatedCPUs) || math.IsInf(target.AllocatedCPUs, 0) {
		return nil, errors.New("invalid requested database CPU quota")
	}
	args := []string{"--config", target.DockerConfig, "--host", target.DockerHost, "inspect", "--format", "{{.Id}} {{.HostConfig.NanoCpus}}", target.Container}
	commandCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	raw, err := exec.CommandContext(commandCtx, "docker", args...).Output()
	if err != nil {
		return nil, fmt.Errorf("cannot verify owned database CPU quota: %w", err)
	}
	fields := strings.Fields(string(raw))
	if len(fields) != 2 || fields[0] != target.Container {
		return nil, errors.New("owned database Docker inspection identity differs from benchmark target")
	}
	nanoCPUs, err := strconv.ParseInt(fields[1], 10, 64)
	if err != nil || nanoCPUs <= 0 || math.Abs(float64(nanoCPUs)/1e9-target.AllocatedCPUs) > 1e-6 {
		return nil, errors.New("owned database Docker CPU quota differs from benchmark CPU denominator")
	}
	quota := &DatabaseCPUQuota{ContainerID: fields[0], NanoCPUs: nanoCPUs, RequestedCPUs: target.AllocatedCPUs, ObservedCPUs: float64(nanoCPUs) / 1e9}
	return quota, nil
}

func resourceHTTPClient(target fixture.ResourceTarget) (*http.Client, error) {
	if target.Container == "" {
		return nil, nil
	}
	if !strings.HasPrefix(target.DockerHost, "unix://") {
		return nil, errors.New("resource collection requires the fixture's explicit local Docker Unix socket")
	}
	socket := strings.TrimPrefix(target.DockerHost, "unix://")
	if socket == "" {
		return nil, errors.New("empty Docker Unix socket")
	}
	transport := &http.Transport{Proxy: nil, MaxConnsPerHost: 1, MaxIdleConnsPerHost: 1}
	transport.DialContext = func(ctx context.Context, _, _ string) (net.Conn, error) {
		dialer := net.Dialer{}
		return dialer.DialContext(ctx, "unix", socket)
	}
	client := &http.Client{Transport: transport, Timeout: 3 * time.Second}
	return client, nil
}

type resourceSampleOptions struct {
	Target  fixture.ResourceTarget
	Paths   *workload.Paths
	Client  *http.Client
	Started time.Time
}

func sampleResources(ctx context.Context, options resourceSampleOptions) ResourceSample {
	target, paths, client := options.Target, options.Paths, options.Client
	sample := ResourceSample{At: time.Now().UTC().Format(time.RFC3339Nano)}
	if target.Container != "" {
		var err error
		sample, err = sampleDocker(ctx, client, target.Container)
		if err != nil {
			sample.Error = "Docker Engine resource sampling: " + err.Error()
		}
	} else if target.ProcessID > 0 {
		cpu, memory, err := processStats(ctx, target.ProcessID)
		if err != nil {
			sample.Error = "native database CPU sampling: " + err.Error()
		} else {
			sample.CPUTimeNS, sample.MemoryBytes = &cpu, memory
		}
	} else {
		sample.Error = "database resource identity unavailable"
	}
	sample.ElapsedNS = time.Since(options.Started).Nanoseconds()
	clientCPU, _, err := processStats(ctx, os.Getpid())
	if err == nil {
		sample.ClientCPUTimeNS = &clientCPU
	}
	if target.WeirProcessID > 0 {
		weirCPU, _, err := processStats(ctx, target.WeirProcessID)
		if err == nil {
			sample.WeirCPUTimeNS = &weirCPU
		}
	}
	if paths != nil {
		counters, err := paths.DatabaseCounters(ctx)
		if err == nil {
			sample.Database = counters
		} else {
			sample.Error = strings.TrimSpace(sample.Error + " database counters: " + err.Error())
		}
	}
	return sample
}

func sampleDocker(ctx context.Context, client *http.Client, container string) (ResourceSample, error) {
	sample := ResourceSample{At: time.Now().UTC().Format(time.RFC3339Nano)}
	if client == nil || len(container) != 64 || strings.Trim(container, "abcdef0123456789") != "" {
		return sample, errors.New("missing owned container identity or explicit Docker Engine connection")
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, "http://docker/containers/"+container+"/stats?stream=false&one-shot=true", nil)
	if err != nil {
		return sample, err
	}
	response, err := client.Do(request)
	if err != nil {
		return sample, err
	}
	defer response.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(response.Body, (4<<20)+1))
	if err != nil || len(raw) > 4<<20 || response.StatusCode != http.StatusOK {
		return sample, errors.New("invalid Docker Engine stats response")
	}
	var stats struct {
		Read string `json:"read"`
		CPU  struct {
			Usage struct {
				Total *int64 `json:"total_usage"`
			} `json:"cpu_usage"`
		} `json:"cpu_stats"`
		Memory struct {
			Usage uint64 `json:"usage"`
		} `json:"memory_stats"`
		Block struct {
			Bytes []struct {
				Op    string `json:"op"`
				Value uint64 `json:"value"`
			} `json:"io_service_bytes_recursive"`
		} `json:"blkio_stats"`
		Networks map[string]struct {
			RX uint64 `json:"rx_bytes"`
			TX uint64 `json:"tx_bytes"`
		} `json:"networks"`
	}
	if err := json.Unmarshal(raw, &stats); err != nil {
		return sample, err
	}
	if stats.CPU.Usage.Total == nil || *stats.CPU.Usage.Total < 0 {
		return sample, errors.New("Docker Engine cumulative CPU counter unavailable")
	}
	sample.CPUTimeNS = stats.CPU.Usage.Total
	sample.MemoryBytes = stats.Memory.Usage
	for _, entry := range stats.Block.Bytes {
		switch strings.ToLower(entry.Op) {
		case "read":
			sample.BlockReadBytes += entry.Value
		case "write":
			sample.BlockWriteBytes += entry.Value
		}
	}
	for _, network := range stats.Networks {
		sample.NetworkInBytes += network.RX
		sample.NetworkOutBytes += network.TX
	}
	clock, err := time.Parse(time.RFC3339Nano, stats.Read)
	if err != nil {
		return sample, errors.New("Docker Engine cumulative CPU timestamp unavailable")
	}
	sample.At = stats.Read
	sample.CPUClockNS = clock.UnixNano()
	return sample, nil
}

func processStats(ctx context.Context, pid int) (int64, uint64, error) {
	raw, err := exec.CommandContext(ctx, "ps", "-p", strconv.Itoa(pid), "-o", "time=", "-o", "rss=").Output()
	if err != nil {
		return 0, 0, err
	}
	fields := strings.Fields(string(raw))
	if len(fields) != 2 {
		return 0, 0, errors.New("process CPU/RSS fields unavailable")
	}
	cpu, err := parseCPUTime(fields[0])
	if err != nil {
		return 0, 0, err
	}
	memory, err := strconv.ParseUint(fields[1], 10, 64)
	return cpu, memory * 1024, err
}

func parseCPUTime(raw string) (int64, error) {
	var days float64
	if prefix, suffix, ok := strings.Cut(raw, "-"); ok {
		value, err := strconv.ParseFloat(prefix, 64)
		if err != nil {
			return 0, err
		}
		days, raw = value, suffix
	}
	parts := strings.Split(raw, ":")
	if len(parts) < 2 || len(parts) > 3 || days < 0 {
		return 0, errors.New("invalid cumulative process CPU time")
	}
	seconds := 0.0
	for _, part := range parts {
		value, err := strconv.ParseFloat(part, 64)
		if err != nil || value < 0 {
			return 0, errors.New("invalid process CPU time field")
		}
		seconds = seconds*60 + value
	}
	seconds += days * 86400
	return int64(seconds * 1e9), nil
}

func (r *Resources) qualify(threshold float64) {
	var sum, full float64
	for index := 1; index < len(r.Samples); index++ {
		current, previous := r.Samples[index], r.Samples[index-1]
		span := min(current.ElapsedNS, r.MeasurementNS) - max(previous.ElapsedNS, int64(0))
		if span <= 0 {
			continue
		}
		var cpu *float64
		clockSpan := current.ElapsedNS - previous.ElapsedNS
		if current.CPUClockNS > 0 && previous.CPUClockNS > 0 {
			clockSpan = current.CPUClockNS - previous.CPUClockNS
		}
		if current.CPUTimeNS != nil && previous.CPUTimeNS != nil && clockSpan > 0 && *current.CPUTimeNS >= *previous.CPUTimeNS {
			value := float64(*current.CPUTimeNS-*previous.CPUTimeNS) / float64(clockSpan) / r.AllocatedCPUs * 100
			cpu = &value
			r.Samples[index].CPUBudgetPercent = cpu
		}
		if cpu == nil || current.Error != "" || previous.Error != "" {
			continue
		}
		r.Intervals++
		r.ValidCPUTimeNS += span
		sum += *cpu * float64(span)
		if *cpu >= threshold {
			full += float64(span)
		}
	}
	if r.ValidCPUTimeNS > 0 && r.MeasurementNS > 0 {
		r.MeanCPUPercent = sum / float64(r.ValidCPUTimeNS)
		r.FullCPUFraction = full / float64(r.MeasurementNS)
		r.Coverage = float64(r.ValidCPUTimeNS) / float64(r.MeasurementNS)
	}
	r.CPUSaturated = r.Intervals >= 5 && r.Coverage >= 0.8 && r.MeanCPUPercent >= threshold && r.FullCPUFraction >= 0.8
	if !r.CPUSaturated {
		r.Missing = "sustained database CPU saturation not demonstrated; I/O/network counters alone cannot establish saturation"
	}
}
