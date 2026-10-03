package benchmark

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"runtime"
	"sync"
	"time"

	"github.com/batchstream/weir-tests/internal/workload"
)

type clientConfig struct {
	Dataset      workload.Config
	MongoURI     string
	SearchURL    string
	Path         string
	Evidence     workload.Evidence
	WorkerOffset int
	Threads      int
	TotalWorkers int
	WritePercent int
	TimeoutNS    int64
}

type clientCommand struct {
	Phase       string
	StartUnixNS int64
	DurationNS  int64
}

type expectedRange struct {
	Start     int   `json:"start"`
	Revisions []int `json:"revisions"`
}

type ClientProcessResult struct {
	Phase        string          `json:"phase"`
	PID          int             `json:"pid"`
	Threads      int             `json:"concurrent_workers"`
	WorkerOffset int             `json:"worker_offset"`
	PoolLimit    int             `json:"native_connection_pool_limit"`
	GoMaxProcs   int             `json:"gomaxprocs"`
	StartLagNS   int64           `json:"start_lag_ns"`
	Result       Result          `json:"result"`
	Reads        uint64          `json:"reads"`
	Writes       uint64          `json:"writes"`
	Requests     uint64          `json:"business_requests"`
	Buckets      [256]uint64     `json:"latency_histogram_buckets"`
	Expected     []expectedRange `json:"expected_record_ranges"`
	Exited       bool            `json:"owned_process_exited"`
	ExitError    string          `json:"owned_process_exit_error,omitempty"`
}

type clientReply struct {
	Phase  string
	PID    int
	Result *ClientProcessResult
}

// RunClientWorker is entered only by the explicit client-worker CLI command.
// It owns a client pool, never a database or Weir execution worker.
func RunClientWorker(ctx context.Context, input io.Reader, output io.Writer) (resultErr error) {
	decoder := json.NewDecoder(io.LimitReader(input, 1<<20))
	decoder.DisallowUnknownFields()
	encoder := json.NewEncoder(output)
	var config clientConfig
	if err := decoder.Decode(&config); err != nil {
		return err
	}
	if err := validateClientConfig(config); err != nil {
		return err
	}
	config.Dataset.MongoURI, config.Dataset.SearchURL = config.MongoURI, config.SearchURL
	config.Dataset.Concurrency = config.Threads
	dataset, err := workload.New(config.Dataset)
	if err != nil {
		return err
	}
	opts := workload.ClientOptions{Dataset: dataset, Path: config.Path, Evidence: config.Evidence}
	client, err := workload.OpenClient(ctx, opts)
	if err != nil {
		return err
	}
	defer func() { resultErr = errors.Join(resultErr, client.Close()) }()
	state := newDurationState(dataset, config.TotalWorkers, 1, config.WritePercent)
	ready := clientReply{Phase: "ready", PID: os.Getpid()}
	if err := encoder.Encode(ready); err != nil {
		return err
	}
	for _, phase := range []string{"warmup", "timed"} {
		var command clientCommand
		if err := decoder.Decode(&command); err != nil {
			return err
		}
		if command.Phase != phase || command.DurationNS <= 0 || command.DurationNS > int64(10*time.Minute) || command.StartUnixNS <= 0 {
			return errors.New("invalid client phase barrier")
		}
		opts := singleMeasureOptions{Executor: client.Executor, State: state, WorkerOffset: config.WorkerOffset, Threads: config.Threads, Timeout: time.Duration(config.TimeoutNS), Command: command}
		result := measureSingleRequests(ctx, opts)
		reply := clientReply{Phase: phase, PID: os.Getpid(), Result: &result}
		if err := encoder.Encode(reply); err != nil {
			return err
		}
		if !result.Result.complete() || ctx.Err() != nil {
			return errors.New("single-request client phase failed")
		}
	}
	// Keep the initialized pool idle for postflight and final CPU sampling.
	// Closing stdin ends the owned process without another business request.
	var extra clientCommand
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		return errors.New("unexpected third client phase")
	}
	return nil
}

func validateClientConfig(c clientConfig) error {
	if (c.Path != "direct" && c.Path != "weir") || c.Threads < 1 || c.Threads > 1024 || c.TotalWorkers < c.Threads || c.TotalWorkers > 1024 || c.WorkerOffset < 0 || c.WorkerOffset+c.Threads > c.TotalWorkers || c.Dataset.Records < c.TotalWorkers || c.WritePercent < 0 || c.WritePercent > 100 || c.TimeoutNS <= 0 || c.TimeoutNS > int64(time.Minute) {
		return errors.New("invalid independent client configuration")
	}
	return nil
}

type singleMeasureOptions struct {
	Executor     workload.Executor
	State        *durationState
	WorkerOffset int
	Threads      int
	Timeout      time.Duration
	Command      clientCommand
}

func measureSingleRequests(ctx context.Context, opts singleMeasureOptions) ClientProcessResult {
	workers := make([]workerResult, opts.Threads)
	reads, writes := make([]uint64, opts.Threads), make([]uint64, opts.Threads)
	barrier := make(chan struct{})
	var ready, joined sync.WaitGroup
	ready.Add(opts.Threads)
	joined.Add(opts.Threads)
	var deadline time.Time
	for index := range opts.Threads {
		go func() {
			defer joined.Done()
			ready.Done()
			<-barrier
			worker := &workers[index]
			global := opts.WorkerOffset + index
			for ctx.Err() == nil && time.Now().Before(deadline) {
				operation := opts.State.nextSingle(global)
				callCtx, cancel := context.WithTimeout(ctx, opts.Timeout)
				began := time.Now()
				outcome := opts.Executor.Execute(callCtx, operation)
				worker.hist.add(time.Since(began))
				if outcome.Status != workload.Success && errors.Is(callCtx.Err(), context.DeadlineExceeded) {
					worker.result.Timeouts++
				}
				cancel()
				recordOutcome(&worker.result, outcome)
				if operation.Write {
					writes[index]++
				} else {
					reads[index]++
				}
				if outcome.Status != workload.Success {
					break
				}
			}
		}()
	}
	ready.Wait()
	wait := time.Until(time.Unix(0, opts.Command.StartUnixNS))
	if wait > 0 {
		timer := time.NewTimer(wait)
		select {
		case <-timer.C:
		case <-ctx.Done():
			timer.Stop()
		}
	}
	started := time.Now()
	lag := time.Duration(started.UnixNano() - opts.Command.StartUnixNS)
	commonStart := started.Add(-lag)
	deadline = commonStart.Add(time.Duration(opts.Command.DurationNS))
	if lag > 100*time.Millisecond {
		deadline = started
	}
	close(barrier)
	joined.Wait()
	base := Result{Path: opts.Executor.Name(), ElapsedNS: time.Since(commonStart).Nanoseconds(), Evidence: opts.Executor.Evidence()}
	if lag > 100*time.Millisecond {
		base.ErrorExamples = append(base.ErrorExamples, "client phase start lag exceeded 100ms; measurement invalid")
	}
	result := ClientProcessResult{PID: os.Getpid(), Threads: opts.Threads, PoolLimit: opts.Threads, WorkerOffset: opts.WorkerOffset, GoMaxProcs: runtime.GOMAXPROCS(0), StartLagNS: started.UnixNano() - opts.Command.StartUnixNS, Result: base}
	result.Phase = opts.Command.Phase
	if opts.Executor.Name() != "direct" {
		result.PoolLimit = 0
	}
	var combined histogram
	for index, worker := range workers {
		mergeResult(&result.Result, worker.result)
		result.Reads += reads[index]
		result.Writes += writes[index]
		combined.merge(&worker.hist)
		global := &opts.State.workers[opts.WorkerOffset+index]
		expected := expectedRange{Start: global.start, Revisions: global.revisions}
		result.Expected = append(result.Expected, expected)
	}
	result.Requests = result.Result.Attempted
	result.Result.Planned = result.Result.Attempted
	result.Result.OperationsPerSec = float64(result.Result.Succeeded) * 1e9 / float64(result.Result.ElapsedNS)
	result.Result.Latency = combined.summary()
	result.Buckets = combined.buckets
	return result
}

func (s *durationState) nextSingle(index int) workload.Operation {
	worker := &s.workers[index]
	x := uint64(worker.sequence)*0x9e3779b97f4a7c15 + uint64(index+1)*0xbf58476d1ce4e5b9
	x ^= x >> 30
	x *= 0xbf58476d1ce4e5b9
	x ^= x >> 27
	write := int((x>>32)%100) < s.writes
	record := worker.start + worker.cursor
	if write {
		worker.revisions[worker.cursor] = 1 - worker.revisions[worker.cursor]
	}
	operation := workload.Operation{Worker: index, Sequence: worker.sequence, Record: record, Write: write, Revision: worker.revisions[worker.cursor]}
	worker.cursor = (worker.cursor + 1) % (worker.end - worker.start)
	worker.sequence++
	return operation
}

func recordOutcome(result *Result, outcome workload.Outcome) {
	result.Attempted++
	result.RequestBytes += outcome.RequestBytes
	result.ResponseBytes += outcome.ResponseBytes
	switch outcome.Status {
	case workload.Success:
		result.Succeeded++
	case workload.Indeterminate:
		result.Indeterminate++
	default:
		result.Errors++
	}
	if outcome.Status != workload.Success {
		if outcome.Applied {
			result.AppliedWithError++
		}
		if len(result.ErrorExamples) < 3 {
			result.ErrorExamples = append(result.ErrorExamples, outcome.Error)
		}
	}
}

func mergeResult(target *Result, value Result) {
	target.Timeouts += value.Timeouts
	target.Attempted += value.Attempted
	target.Succeeded += value.Succeeded
	target.Errors += value.Errors
	target.Indeterminate += value.Indeterminate
	target.NotAttempted += value.NotAttempted
	target.AppliedWithError += value.AppliedWithError
	target.RequestBytes += value.RequestBytes
	target.ResponseBytes += value.ResponseBytes
	for _, example := range value.ErrorExamples {
		if len(target.ErrorExamples) < 5 {
			target.ErrorExamples = append(target.ErrorExamples, example)
		}
	}
}

func validateClientReply(reply clientReply, phase string, config clientConfig, pid int) error {
	if reply.Phase != phase || reply.PID != pid {
		return fmt.Errorf("owned client %d invalid %s acknowledgement", pid, phase)
	}
	if phase == "ready" {
		return nil
	}
	r := reply.Result
	pool := 0
	if config.Path == "direct" {
		pool = config.Threads
	}
	if r == nil || r.Phase != phase || r.PID != pid || r.WorkerOffset != config.WorkerOffset || r.Threads != config.Threads || r.PoolLimit != pool || r.Result.Path != config.Path || r.Requests != r.Result.Attempted || r.Reads+r.Writes != r.Requests {
		return errors.New("independent client result identity or single-request counts invalid")
	}
	var count uint64
	for _, value := range r.Buckets {
		count += value
	}
	if count != r.Result.Attempted || r.Result.Latency.Samples != count {
		return errors.New("independent client latency count mismatch")
	}
	return nil
}
