package benchmark

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"math"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"
)

// Host counters cover every process on the runner, not only the database.
// Tick deltas share a kernel clock; no assumed CLK_TCK conversion is needed.
type HostResources struct {
	AtUnixNS       int64                     `json:"at_unix_ns"`
	ReadElapsedNS  int64                     `json:"read_elapsed_ns"`
	Platform       string                    `json:"platform"`
	RuntimeCPUs    int                       `json:"runtime_cpu_count"`
	GoMaxProcs     int                       `json:"gomaxprocs"`
	CPU            *HostCPUCounters          `json:"cpu,omitempty"`
	AllowedCPUList string                    `json:"sampler_allowed_cpu_list,omitempty"`
	AllowedCPUs    *int                      `json:"sampler_allowed_cpus,omitempty"`
	Pressure       map[string]PressureSample `json:"pressure"`
	Unavailable    string                    `json:"unavailable,omitempty"`
}

type HostCPUCounters struct {
	TotalTicks  uint64 `json:"total_ticks"`
	BusyTicks   uint64 `json:"busy_ticks"`
	IdleTicks   uint64 `json:"idle_ticks"`
	IOWaitTicks uint64 `json:"iowait_ticks"`
	StealTicks  uint64 `json:"steal_ticks"`
	CPUsListed  int    `json:"cpus_listed"`
}

type PressureSample struct {
	Some        *PressureWindow `json:"some,omitempty"`
	Full        *PressureWindow `json:"full,omitempty"`
	Unavailable string          `json:"unavailable,omitempty"`
}

type PressureWindow struct {
	Average10  float64 `json:"avg10"`
	Average60  float64 `json:"avg60"`
	Average300 float64 `json:"avg300"`
	TotalUS    uint64  `json:"total_us"`
}

func sampleHostResources(procRoot string) HostResources {
	started := time.Now()
	sample := HostResources{AtUnixNS: started.UnixNano(), Platform: runtime.GOOS, RuntimeCPUs: runtime.NumCPU(), GoMaxProcs: runtime.GOMAXPROCS(0), Pressure: make(map[string]PressureSample)}
	var unavailable []string
	raw, err := readResourceFile(filepath.Join(procRoot, "stat"))
	if err == nil {
		sample.CPU, err = parseHostCPU(raw)
	}
	if err != nil {
		unavailable = append(unavailable, "host CPU: "+err.Error())
	}
	raw, err = readResourceFile(filepath.Join(procRoot, "self", "status"))
	if err == nil {
		sample.AllowedCPUList, err = hostAllowedCPUList(raw)
		if err == nil {
			var count int
			count, err = countCPUList(sample.AllowedCPUList)
			if err == nil {
				sample.AllowedCPUs = &count
			}
		}
	}
	if err != nil {
		unavailable = append(unavailable, "sampler CPU affinity: "+err.Error())
	}
	for _, resource := range []string{"cpu", "io", "memory"} {
		pressure := PressureSample{}
		raw, err := readResourceFile(filepath.Join(procRoot, "pressure", resource))
		if err == nil {
			pressure, err = parsePressure(raw)
		}
		if err != nil {
			pressure.Unavailable = err.Error()
		}
		sample.Pressure[resource] = pressure
	}
	sample.Unavailable = strings.Join(unavailable, "; ")
	sample.ReadElapsedNS = time.Since(started).Nanoseconds()
	return sample
}

func readResourceFile(path string) ([]byte, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	raw, err := io.ReadAll(io.LimitReader(file, (1<<20)+1))
	if err != nil {
		return nil, err
	}
	if len(raw) > 1<<20 {
		return nil, errors.New("resource file exceeds 1 MiB")
	}
	return raw, nil
}

func parseHostCPU(raw []byte) (*HostCPUCounters, error) {
	var counters *HostCPUCounters
	listed := 0
	scanner := bufio.NewScanner(strings.NewReader(string(raw)))
	for scanner.Scan() {
		fields := strings.Fields(scanner.Text())
		if len(fields) == 0 {
			continue
		}
		if fields[0] != "cpu" {
			if strings.HasPrefix(fields[0], "cpu") {
				if _, err := strconv.ParseUint(strings.TrimPrefix(fields[0], "cpu"), 10, 32); err == nil {
					listed++
				}
			}
			continue
		}
		if counters != nil || len(fields) < 9 {
			return nil, errors.New("missing or duplicate aggregate CPU tick fields")
		}
		var ticks [8]uint64
		var total uint64
		for index := range ticks {
			value, err := strconv.ParseUint(fields[index+1], 10, 64)
			if err != nil || value > math.MaxUint64-total {
				return nil, errors.New("invalid aggregate CPU ticks")
			}
			ticks[index], total = value, total+value
		}
		counters = &HostCPUCounters{TotalTicks: total, IdleTicks: ticks[3], IOWaitTicks: ticks[4], StealTicks: ticks[7]}
		counters.BusyTicks = total - counters.IdleTicks - counters.IOWaitTicks - counters.StealTicks
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	if counters == nil || listed == 0 {
		return nil, errors.New("aggregate CPU or per-CPU entries unavailable")
	}
	counters.CPUsListed = listed
	return counters, nil
}

func hostAllowedCPUList(raw []byte) (string, error) {
	for _, line := range strings.Split(string(raw), "\n") {
		if value, found := strings.CutPrefix(line, "Cpus_allowed_list:"); found {
			return strings.TrimSpace(value), nil
		}
	}
	return "", errors.New("Cpus_allowed_list unavailable")
}

func countCPUList(list string) (int, error) {
	count, previous := 0, int64(-1)
	for _, interval := range strings.Split(list, ",") {
		parts := strings.Split(interval, "-")
		if len(parts) > 2 {
			return 0, errors.New("invalid CPU affinity range")
		}
		first, err := strconv.ParseInt(parts[0], 10, 32)
		if err != nil || first < 0 || first <= previous {
			return 0, errors.New("invalid or overlapping CPU affinity range")
		}
		last := first
		if len(parts) == 2 {
			last, err = strconv.ParseInt(parts[1], 10, 32)
			if err != nil || last < first {
				return 0, errors.New("invalid CPU affinity range end")
			}
		}
		if last-first+1 > 65536-int64(count) {
			return 0, errors.New("CPU affinity exceeds measurement bound")
		}
		count += int(last - first + 1)
		previous = last
	}
	return count, nil
}

func parsePressure(raw []byte) (PressureSample, error) {
	sample := PressureSample{}
	for _, line := range strings.Split(strings.TrimSpace(string(raw)), "\n") {
		fields := strings.Fields(line)
		if len(fields) != 5 || fields[0] != "some" && fields[0] != "full" {
			return sample, errors.New("invalid pressure row")
		}
		if fields[0] == "some" && sample.Some != nil || fields[0] == "full" && sample.Full != nil {
			return sample, errors.New("duplicate pressure row")
		}
		values := make(map[string]string)
		for _, field := range fields[1:] {
			key, value, found := strings.Cut(field, "=")
			if !found || values[key] != "" {
				return sample, errors.New("invalid pressure field")
			}
			values[key] = value
		}
		window := &PressureWindow{}
		for key, target := range map[string]*float64{"avg10": &window.Average10, "avg60": &window.Average60, "avg300": &window.Average300} {
			value, err := strconv.ParseFloat(values[key], 64)
			if err != nil || math.IsNaN(value) || math.IsInf(value, 0) || value < 0 || value > 100 {
				return sample, fmt.Errorf("invalid pressure %s", key)
			}
			*target = value
		}
		var err error
		window.TotalUS, err = strconv.ParseUint(values["total"], 10, 64)
		if err != nil {
			return sample, errors.New("invalid cumulative pressure total")
		}
		if fields[0] == "some" {
			sample.Some = window
		} else {
			sample.Full = window
		}
	}
	if sample.Some == nil {
		return sample, errors.New("some pressure row unavailable")
	}
	if sample.Full == nil {
		sample.Unavailable = "full pressure row unavailable"
	}
	return sample, nil
}
