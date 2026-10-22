package benchmark

import (
	"context"
	"errors"
	"time"

	"github.com/batchstream/weir-tests/internal/fixture"
	"github.com/batchstream/weir-tests/internal/workload"
)

type SaturationOptions struct {
	Dataset          *workload.Dataset
	Paths            *workload.Paths
	Concurrency      []int
	BatchSizes       []int
	Warmup           time.Duration
	Duration         time.Duration
	Rounds           int
	WritePercent     int
	OperationTimeout time.Duration
	Resources        fixture.ResourceTarget
	CPUThreshold     float64
	Provenance       map[string]string
	ClientProcesses  int
	ClientExecutable string
}

type SaturationParameters struct {
	Dataset              workload.Config `json:"dataset"`
	Concurrency          []int           `json:"concurrency_levels"`
	BatchSizes           []int           `json:"batch_sizes"`
	Warmup               string          `json:"warmup_duration"`
	Duration             string          `json:"measurement_duration"`
	Rounds               int             `json:"paired_rounds"`
	WritePercent         int             `json:"requested_write_percent"`
	OperationTimeout     string          `json:"operation_timeout"`
	CPUThreshold         float64         `json:"database_cpu_budget_threshold_percent"`
	ClientProcesses      int             `json:"client_process_count,omitempty"`
	ClientRequestRecords int             `json:"records_per_business_request,omitempty"`
}

type SaturationResult struct {
	DatabaseBefore             *workload.DatabaseCounters `json:"timed_database_counters_before,omitempty"`
	DatabaseAfter              *workload.DatabaseCounters `json:"timed_database_counters_after,omitempty"`
	PhysicalCommands           map[string]uint64          `json:"timed_physical_database_command_deltas,omitempty"`
	PhysicalCommandUnavailable string                     `json:"physical_database_command_observation_unavailable,omitempty"`
	Result
	Reads           uint64                `json:"reads"`
	Writes          uint64                `json:"writes"`
	Requests        uint64                `json:"client_requests"`
	Resources       Resources             `json:"resources"`
	ServerMetrics   *ServerMetrics        `json:"server_metrics,omitempty"`
	ClientProcesses []ClientProcessResult `json:"client_processes,omitempty"`
}

type SaturationPair struct {
	Round       int              `json:"round"`
	Concurrency int              `json:"concurrency"`
	BatchSize   int              `json:"batch_size"`
	Order       []string         `json:"order"`
	Direct      SaturationResult `json:"direct"`
	Weir        SaturationResult `json:"weir"`
}

type SaturationPoint struct {
	Path                  string  `json:"path"`
	Concurrency           int     `json:"concurrency"`
	BatchSize             int     `json:"batch_size"`
	OperationsPerSec      float64 `json:"successful_operations_per_second"`
	MeanCPUPercent        float64 `json:"mean_database_cpu_budget_percent"`
	AllRoundsVerified     bool    `json:"all_rounds_successful_verified"`
	AllRoundsCPUSaturated bool    `json:"all_rounds_cpu_saturated"`
	Plateau               bool    `json:"throughput_plateau_at_next_concurrency"`
	Saturated             bool    `json:"database_saturation_demonstrated"`
}

type SaturationComparison struct {
	BatchSize    int              `json:"batch_size"`
	Direct       *SaturationPoint `json:"direct_database_saturated_point,omitempty"`
	Weir         *SaturationPoint `json:"weir_database_saturated_point,omitempty"`
	Ratio        *float64         `json:"weir_direct_saturated_throughput_ratio,omitempty"`
	DeltaPercent *float64         `json:"weir_saturated_throughput_delta_percent,omitempty"`
	Unavailable  string           `json:"unavailable,omitempty"`
}

// Saturation report kinds distinguish single-record business requests from
// supplemental client bulk calls.
const (
	SaturationKindSingle = "single"
	SaturationKindBulk   = "bulk"
)

type SaturationReport struct {
	BusinessPeaks       *BusinessPeaks         `json:"observed_business_peak_comparison,omitempty"`
	BusinessComparisons []BusinessComparison   `json:"observed_business_comparisons,omitempty"`
	Schema              int                    `json:"schema"`
	Kind                string                 `json:"kind"`
	Started             string                 `json:"started_utc"`
	Parameters          SaturationParameters   `json:"parameters"`
	Provenance          map[string]string      `json:"provenance"`
	CPUQuota            *DatabaseCPUQuota      `json:"database_cpu_quota,omitempty"`
	Pairs               []SaturationPair       `json:"pairs"`
	Points              []SaturationPoint      `json:"points"`
	Comparisons         []SaturationComparison `json:"database_saturated_comparisons"`
	Method              string                 `json:"method"`
	Incomplete          string                 `json:"incomplete,omitempty"`
}

// RunSaturation runs single-record client processes when ClientProcesses is set
// and in-process client bulk calls otherwise.
func RunSaturation(ctx context.Context, options SaturationOptions) (*SaturationReport, error) {
	if options.ClientProcesses > 0 {
		return runSingleSaturation(ctx, options)
	}
	return runBulkSaturation(ctx, options)
}

func validateSaturation(options SaturationOptions) error {
	if options.Dataset == nil || options.Paths == nil || options.Rounds < 1 || options.Rounds > 10 || options.Warmup < time.Second || options.Duration < 5*time.Second || options.Duration > 10*time.Minute || options.OperationTimeout <= 0 || options.OperationTimeout > time.Minute || options.WritePercent < 0 || options.WritePercent > 100 || options.CPUThreshold < 50 || options.CPUThreshold > 100 || options.Resources.AllocatedCPUs <= 0 {
		return errors.New("invalid saturation benchmark parameters")
	}
	if len(options.Concurrency) < 1 || len(options.Concurrency) > 12 || len(options.BatchSizes) < 1 || len(options.BatchSizes) > 8 {
		return errors.New("saturation requires 1 through 12 concurrency levels and 1 through 8 batch sizes; one level cannot demonstrate a plateau")
	}
	previous := 0
	for _, workers := range options.Concurrency {
		if workers <= previous || workers > options.Dataset.Config.Concurrency {
			return errors.New("concurrency ladder must strictly increase within the opened pool size")
		}
		previous = workers
	}
	seen := make(map[int]bool)
	for _, batch := range options.BatchSizes {
		if batch < 1 || batch > 64 || seen[batch] || options.Dataset.Config.Records < previous*batch {
			return errors.New("batch sizes must be distinct, 1 through 64, with at least max-workers*batch-size records")
		}
		seen[batch] = true
	}
	return nil
}

type durationWorker struct {
	sequence  int
	start     int
	end       int
	cursor    int
	revisions []int
}
type durationState struct {
	dataset *workload.Dataset
	workers []durationWorker
	batch   int
	writes  int
}

func newDurationState(dataset *workload.Dataset, workers, batch, writes int) *durationState {
	state := &durationState{dataset: dataset, workers: make([]durationWorker, workers), batch: batch, writes: writes}
	for index := range workers {
		start, end := index*dataset.Config.Records/workers, (index+1)*dataset.Config.Records/workers
		worker := durationWorker{start: start, end: end, revisions: make([]int, end-start)}
		state.workers[index] = worker
	}
	return state
}

func (s *durationState) next(workerIndex int) []workload.Operation {
	worker := &s.workers[workerIndex]
	x := uint64(worker.sequence)*0x9e3779b97f4a7c15 + uint64(workerIndex+1)*0xbf58476d1ce4e5b9
	x ^= x >> 30
	x *= 0xbf58476d1ce4e5b9
	x ^= x >> 27
	write := int((x>>32)%100) < s.writes
	operations := make([]workload.Operation, s.batch)
	for index := range s.batch {
		record := worker.start + worker.cursor
		if write {
			worker.revisions[worker.cursor] = 1 - worker.revisions[worker.cursor]
		}
		operation := workload.Operation{Record: record, Write: write, Revision: worker.revisions[worker.cursor]}
		operations[index] = operation
		worker.cursor = (worker.cursor + 1) % (worker.end - worker.start)
		worker.sequence++
	}
	return operations
}

func (s *durationState) plan() *workload.Plan {
	plan := &workload.Plan{Expected: make([]int, s.dataset.Config.Records)}
	for _, worker := range s.workers {
		copy(plan.Expected[worker.start:worker.end], worker.revisions)
	}
	return plan
}

func (r *SaturationReport) aggregate() {
	r.Points = nil
	r.Comparisons = nil
	for _, batch := range r.Parameters.BatchSizes {
		for _, workers := range r.Parameters.Concurrency {
			for _, path := range []string{"direct", "weir"} {
				point := SaturationPoint{Path: path, Concurrency: workers, BatchSize: batch, AllRoundsVerified: true, AllRoundsCPUSaturated: true}
				var count uint64
				var elapsed int64
				rounds := 0
				for _, pair := range r.Pairs {
					if pair.BatchSize != batch || pair.Concurrency != workers {
						continue
					}
					result := pair.Direct
					if path == "weir" {
						result = pair.Weir
					}
					rounds++
					count += result.Succeeded
					elapsed += result.ElapsedNS
					point.MeanCPUPercent += result.Resources.MeanCPUPercent
					point.AllRoundsVerified = point.AllRoundsVerified && result.complete() && result.Verified
					point.AllRoundsCPUSaturated = point.AllRoundsCPUSaturated && result.Resources.CPUSaturated
				}
				point.AllRoundsVerified = point.AllRoundsVerified && rounds == r.Parameters.Rounds
				point.AllRoundsCPUSaturated = point.AllRoundsCPUSaturated && rounds == r.Parameters.Rounds
				if elapsed > 0 {
					point.OperationsPerSec = float64(count) * 1e9 / float64(elapsed)
				}
				if rounds > 0 {
					point.MeanCPUPercent /= float64(rounds)
				}
				r.Points = append(r.Points, point)
			}
		}
	}
	for index := range r.Points {
		point := &r.Points[index]
		for nextIndex := range r.Points {
			next := r.Points[nextIndex]
			if next.Path != point.Path || next.BatchSize != point.BatchSize || next.Concurrency <= point.Concurrency {
				continue
			}
			point.Plateau = point.AllRoundsVerified && next.AllRoundsVerified && point.OperationsPerSec > 0 && next.OperationsPerSec <= point.OperationsPerSec*1.1
			break
		}
		point.Saturated = point.AllRoundsVerified && point.AllRoundsCPUSaturated && point.Plateau && r.Incomplete == ""
	}
	for _, batch := range r.Parameters.BatchSizes {
		comparison := SaturationComparison{BatchSize: batch}
		for index := range r.Points {
			point := &r.Points[index]
			if point.BatchSize != batch || !point.Saturated {
				continue
			}
			if point.Path == "direct" && (comparison.Direct == nil || point.OperationsPerSec > comparison.Direct.OperationsPerSec) {
				comparison.Direct = point
			}
			if point.Path == "weir" && (comparison.Weir == nil || point.OperationsPerSec > comparison.Weir.OperationsPerSec) {
				comparison.Weir = point
			}
		}
		if comparison.Direct != nil && comparison.Weir != nil {
			ratio := comparison.Weir.OperationsPerSec / comparison.Direct.OperationsPerSec
			delta := (ratio - 1) * 100
			comparison.Ratio, comparison.DeltaPercent = &ratio, &delta
		} else {
			comparison.Unavailable = "both paths must independently demonstrate sustained database CPU saturation and a verified next-level throughput plateau; increase concurrency or isolate client/Weir resources; I/O/network-bound saturation needs additional device/link capacity evidence"
		}
		r.Comparisons = append(r.Comparisons, comparison)
	}
}
