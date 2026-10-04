package benchmark

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/batchstream/weir-tests/internal/workload"
)

type ownedClient struct {
	config  clientConfig
	command *exec.Cmd
	input   io.WriteCloser
	decoder *json.Decoder
	cancel  context.CancelFunc
	log     limitedClientLog
}

type limitedClientLog struct {
	mu   sync.Mutex
	data bytes.Buffer
}

func (l *limitedClientLog) Write(raw []byte) (int, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	size := min(len(raw), max(0, 8192-l.data.Len()))
	_, _ = l.data.Write(raw[:size])
	return len(raw), nil
}
func (l *limitedClientLog) String() string {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.data.String()
}

func runSingleSaturation(ctx context.Context, options SaturationOptions) (*SaturationReport, error) {
	if len(options.BatchSizes) != 1 || options.BatchSizes[0] != 1 {
		return nil, errors.New("independent single-request clients require batch size 1")
	}
	if err := validateSaturation(options); err != nil {
		return nil, err
	}
	if options.ClientExecutable == "" || options.ClientProcesses < 1 || options.ClientProcesses > 32 {
		return nil, errors.New("single-request benchmark requires 1 through 32 independent client processes and an executable")
	}
	for _, workers := range options.Concurrency {
		if workers%options.ClientProcesses != 0 {
			return nil, errors.New("every concurrency level must divide evenly between client processes")
		}
	}
	quota, err := verifyResourceBudget(ctx, options.Resources)
	if err != nil {
		return nil, err
	}
	params := SaturationParameters{Dataset: options.Dataset.Config, Concurrency: options.Concurrency, BatchSizes: []int{1}, Warmup: options.Warmup.String(), Duration: options.Duration.String(), Rounds: options.Rounds, WritePercent: options.WritePercent, OperationTimeout: options.OperationTimeout.String(), CPUThreshold: options.CPUThreshold, ClientProcesses: options.ClientProcesses, ClientRequestRecords: 1}
	provenance := make(map[string]string, len(options.Provenance)+3)
	for key, value := range options.Provenance {
		provenance[key] = value
	}
	provenance["go"], provenance["client_os_arch"] = runtime.Version(), runtime.GOOS+"/"+runtime.GOARCH
	provenance["client_process_model"] = "independent OS client processes; one SDK/driver pool each; sequential single-request goroutine workers; total native pool limit equals total client concurrency"
	report := &SaturationReport{Schema: 3, Started: time.Now().UTC().Format(time.RFC3339Nano), Parameters: params, Provenance: provenance, CPUQuota: quota, Method: "independent OS client processes send one record per native call or streaming SDK RPC; native FindOne/ReplaceOne or single-ID mget/PUT; no client aggregation; worker-owned disjoint IDs; common phase barriers and unchanged pools across warmup and measurement; alternating AB/BA; latency is each business request including validation, queueing and any server aggregation; independent persisted postflight; sustained database CPU requires >=5 intervals, >=80% measurement coverage, mean CPU>=threshold and CPU>=threshold during >=80% measurement duration in every round, plus <=10% next-level throughput gain"}
	if options.Dataset.Config.LuaMutations {
		report.Method = strings.Replace(report.Method, "native FindOne/ReplaceOne or single-ID mget/PUT", "native single-record snapshot transaction FindOne/compute/ReplaceOne/commit or real-time single-ID mget/compute/version-conditional PUT; SDK Lua AtomicTransform computes the same revision toggle from the current source", 1)
	}
	for level, workers := range options.Concurrency {
		for round := range options.Rounds {
			pair := SaturationPair{Round: round + 1, Concurrency: workers, BatchSize: 1, Order: pairOrder(level + round)}
			for _, path := range pair.Order {
				if err := options.Paths.Prepare(ctx, 64); err != nil {
					report.Incomplete = "prepare: " + err.Error()
					return report, err
				}
				stageOptions := singleStageOptions{Options: options, Path: path, Workers: workers}
				result, err := runSingleStage(ctx, stageOptions)
				if path == "direct" {
					pair.Direct = result
				} else {
					pair.Weir = result
				}
				if err != nil || !result.complete() || !result.Verified {
					report.Pairs = append(report.Pairs, pair)
					report.Incomplete = fmt.Sprintf("%s single-request stage failed: %v %s", path, err, result.VerificationError)
					return report, errors.New(report.Incomplete)
				}
			}
			report.Pairs = append(report.Pairs, pair)
			fmt.Printf("single-request backend=%s processes=%d workers=%d round=%d direct=%.1f weir=%.1f DB-CPU direct=%.1f%% weir=%.1f%%\n", options.Dataset.Config.Backend, options.ClientProcesses, workers, round+1, pair.Direct.OperationsPerSec, pair.Weir.OperationsPerSec, pair.Direct.Resources.MeanCPUPercent, pair.Weir.Resources.MeanCPUPercent)
		}
	}
	report.aggregate()
	report.aggregateBusiness()
	return report, nil
}

type singleStageOptions struct {
	Options SaturationOptions
	Path    string
	Workers int
}

func runSingleStage(ctx context.Context, opts singleStageOptions) (result SaturationResult, resultErr error) {
	options := opts.Options
	executor := options.Paths.Direct
	if opts.Path == "weir" {
		executor = options.Paths.Weir
	}
	base := Result{Path: opts.Path, Evidence: executor.Evidence()}
	base.Evidence.ClientBatch = "exactly one logical record per business call; no client batching"
	result.Result = base
	clients := make([]*ownedClient, 0, options.ClientProcesses)
	defer func() {
		for index, client := range clients {
			err := client.close()
			resultErr = errors.Join(resultErr, err)
			if index < len(result.ClientProcesses) {
				result.ClientProcesses[index].Exited = err == nil
				if err != nil {
					result.ClientProcesses[index].ExitError = err.Error()
				}
			}
		}
	}()
	for index := range options.ClientProcesses {
		config := clientConfig{Dataset: options.Dataset.Config, MongoURI: options.Dataset.Config.MongoURI, SearchURL: options.Dataset.Config.SearchURL, Path: opts.Path, Evidence: base.Evidence, WorkerOffset: index * (opts.Workers / options.ClientProcesses), Threads: opts.Workers / options.ClientProcesses, TotalWorkers: opts.Workers, WritePercent: options.WritePercent, TimeoutNS: int64(options.OperationTimeout)}
		client, err := startOwnedClient(ctx, options.ClientExecutable, config)
		if err != nil {
			return result, err
		}
		clients = append(clients, client)
		identity := ClientProcessResult{Phase: "startup", PID: client.command.Process.Pid, Threads: config.Threads, WorkerOffset: config.WorkerOffset}
		result.ClientProcesses = append(result.ClientProcesses, identity)
	}
	warmup := clientCommand{Phase: "warmup", StartUnixNS: time.Now().Add(200 * time.Millisecond).UnixNano(), DurationNS: int64(options.Warmup)}
	warm, err := runClientPhase(ctx, clients, warmup)
	if err != nil {
		return result, err
	}
	for _, reply := range warm {
		if !reply.Result.complete() {
			result.ClientProcesses = warm
			return result, fmt.Errorf("independent client %d warmup failed: %v", reply.PID, reply.Result.ErrorExamples)
		}
	}
	var metricsBefore ServerMetrics
	beforeDatabase, counterErr := options.Paths.DatabaseCounters(ctx)
	if counterErr == nil {
		result.DatabaseBefore = &beforeDatabase
	} else {
		result.PhysicalCommandUnavailable = counterErr.Error()
	}
	if opts.Path == "weir" {
		metricsBefore.Before = fetchServerMetrics(ctx, options.Resources.Diagnostics)
	}
	resourceClient, err := resourceHTTPClient(options.Resources)
	if err != nil {
		return result, err
	}
	if resourceClient != nil {
		defer resourceClient.CloseIdleConnections()
	}
	command := clientCommand{Phase: "timed", StartUnixNS: time.Now().Add(200 * time.Millisecond).UnixNano(), DurationNS: int64(options.Duration)}
	for _, client := range clients {
		if err := json.NewEncoder(client.input).Encode(command); err != nil {
			return result, err
		}
	}
	timer := time.NewTimer(time.Until(time.Unix(0, command.StartUnixNS)))
	select {
	case <-timer.C:
	case <-ctx.Done():
		timer.Stop()
		return result, ctx.Err()
	}
	now := time.Now()
	started := now.Add(-time.Duration(now.UnixNano() - command.StartUnixNS))
	var monitor sync.WaitGroup
	stop := make(chan struct{})
	resources := Resources{AllocatedCPUs: options.Resources.AllocatedCPUs, Source: "owned Docker Engine cumulative CPU and timestamps; DB counters; individual owned client PID ps CPU/RSS and Weir CPU/RSS; 1s samples; common timed barrier"}
	monitor.Add(1)
	go func() {
		defer monitor.Done()
		for {
			sampleCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
			pids := make([]int, len(clients))
			for i, client := range clients {
				pids[i] = client.command.Process.Pid
			}
			sampleOptions := resourceSampleOptions{Target: options.Resources, Paths: options.Paths, Client: resourceClient, Started: started, ClientPIDs: pids}
			sample := sampleResources(sampleCtx, sampleOptions)
			cancel()
			resources.Samples = append(resources.Samples, sample)
			timer := time.NewTimer(time.Second)
			select {
			case <-stop:
				timer.Stop()
				return
			case <-ctx.Done():
				timer.Stop()
				return
			case <-timer.C:
			}
		}
	}()
	replies, readErr := readClientPhase(ctx, clients, command)
	for index, reply := range replies {
		if reply.PID > 0 {
			result.ClientProcesses[index] = reply
		}
	}
	elapsed := time.Since(started)
	close(stop)
	monitor.Wait()
	result.Resources = resources
	result.Resources.MeasurementNS = elapsed.Nanoseconds()
	result.Resources.qualify(options.CPUThreshold)
	result.ElapsedNS = elapsed.Nanoseconds()
	if opts.Path == "weir" {
		after := fetchServerMetrics(ctx, options.Resources.Diagnostics)
		result.ServerMetrics = serverMetricDelta(metricsBefore.Before, after, options.Dataset.Config.StoreName)
	}
	if readErr != nil {
		return result, readErr
	}
	afterDatabase, counterErr := options.Paths.DatabaseCounters(ctx)
	if counterErr == nil {
		result.DatabaseAfter = &afterDatabase
	} else {
		result.PhysicalCommandUnavailable = mergeCommandUnavailable(result.PhysicalCommandUnavailable, counterErr.Error())
	}
	if result.DatabaseBefore != nil && result.DatabaseAfter != nil {
		result.PhysicalCommandUnavailable = mergeCommandUnavailable(result.DatabaseBefore.CommandUnavailable, result.DatabaseAfter.CommandUnavailable)
		result.PhysicalCommands = make(map[string]uint64)
		for name, after := range result.DatabaseAfter.Commands {
			before, found := result.DatabaseBefore.Commands[name]
			if !found || after < before {
				result.PhysicalCommandUnavailable = mergeCommandUnavailable(result.PhysicalCommandUnavailable, "unavailable or reset "+name)
				continue
			}
			result.PhysicalCommands[name] = after - before
		}
	}
	plan := &workload.Plan{Expected: make([]int, options.Dataset.Config.Records)}
	covered := make([]bool, len(plan.Expected))
	var histogram histogram
	for _, reply := range replies {
		mergeResult(&result.Result, reply.Result)
		result.Reads += reply.Reads
		result.Writes += reply.Writes
		result.Requests += reply.Requests
		part := histogramFromClient(reply)
		histogram.merge(&part)
		for _, expected := range reply.Expected {
			if expected.Start < 0 || expected.Start+len(expected.Revisions) > len(plan.Expected) {
				return result, errors.New("child expected record range out of bounds")
			}
			for index, revision := range expected.Revisions {
				record := expected.Start + index
				if covered[record] || revision < 0 || revision > 1 {
					return result, errors.New("overlapping child IDs or invalid expected revision")
				}
				covered[record] = true
				plan.Expected[record] = revision
			}
		}
	}
	for _, found := range covered {
		if !found {
			return result, errors.New("child postflight plan omitted records")
		}
	}
	result.Planned = result.Attempted
	result.OperationsPerSec = float64(result.Succeeded) / elapsed.Seconds()
	result.Latency = histogram.summary()
	if result.ServerMetrics != nil {
		result.ServerMetrics.qualifySingleRequests(result.Reads, result.Writes)
	}
	if err := options.Paths.Verify(ctx, plan, 64); err != nil {
		result.VerificationError = err.Error()
		return result, err
	}
	result.Verified = true
	return result, nil
}

func mergeCommandUnavailable(reasons ...string) string {
	var unique []string
	seen := make(map[string]bool)
	for _, reason := range reasons {
		for _, part := range strings.Split(reason, ";") {
			part = strings.TrimSpace(part)
			if part != "" && !seen[part] {
				seen[part] = true
				unique = append(unique, part)
			}
		}
	}
	return strings.Join(unique, "; ")
}

func histogramFromClient(value ClientProcessResult) histogram {
	h := histogram{buckets: value.Buckets, count: value.Result.Latency.Samples, max: value.Result.Latency.MaxNS}
	return h
}

func startOwnedClient(ctx context.Context, executable string, config clientConfig) (*ownedClient, error) {
	childCtx, cancel := context.WithCancel(ctx)
	child := &ownedClient{config: config, cancel: cancel}
	child.command = exec.CommandContext(childCtx, executable, "client-worker")
	child.command.Stderr = &child.log
	input, err := child.command.StdinPipe()
	if err != nil {
		cancel()
		return nil, err
	}
	output, err := child.command.StdoutPipe()
	if err != nil {
		cancel()
		_ = input.Close()
		return nil, err
	}
	child.input = input
	child.decoder = json.NewDecoder(io.LimitReader(output, 4<<20))
	child.decoder.DisallowUnknownFields()
	if err := child.command.Start(); err != nil {
		cancel()
		_ = input.Close()
		_ = output.Close()
		return nil, err
	}
	if err := json.NewEncoder(input).Encode(config); err != nil {
		_ = child.close()
		return nil, err
	}
	reply, err := child.read(ctx, "ready", 15*time.Second)
	if err != nil {
		_ = child.close()
		return nil, err
	}
	err = validateClientReply(reply, "ready", config, child.command.Process.Pid)
	if err != nil {
		_ = child.close()
		return nil, err
	}
	return child, nil
}

func (c *ownedClient) read(ctx context.Context, phase string, timeout time.Duration) (clientReply, error) {
	type readResult struct {
		reply clientReply
		err   error
	}
	channel := make(chan readResult, 1)
	go func() {
		var reply clientReply
		err := c.decoder.Decode(&reply)
		value := readResult{reply: reply, err: err}
		channel <- value
	}()
	timer := time.NewTimer(timeout)
	defer timer.Stop()
	var empty clientReply
	select {
	case value := <-channel:
		if value.err != nil {
			return value.reply, fmt.Errorf("owned client %d %s: %w; %s", c.command.Process.Pid, phase, value.err, c.log.String())
		}
		return value.reply, validateClientReply(value.reply, phase, c.config, c.command.Process.Pid)
	case <-timer.C:
		return empty, fmt.Errorf("owned client %d %s timed out", c.command.Process.Pid, phase)
	case <-ctx.Done():
		return empty, ctx.Err()
	}
}

func (c *ownedClient) close() error {
	_ = c.input.Close()
	done := make(chan error, 1)
	go func() { done <- c.command.Wait() }()
	timer := time.NewTimer(12 * time.Second)
	defer timer.Stop()
	select {
	case err := <-done:
		c.cancel()
		return err
	case <-timer.C:
		c.cancel()
		<-done
		return errors.New("owned benchmark client required forced termination")
	}
}

func runClientPhase(ctx context.Context, clients []*ownedClient, command clientCommand) ([]ClientProcessResult, error) {
	for _, client := range clients {
		if err := json.NewEncoder(client.input).Encode(command); err != nil {
			return nil, err
		}
	}
	return readClientPhase(ctx, clients, command)
}
func readClientPhase(ctx context.Context, clients []*ownedClient, command clientCommand) ([]ClientProcessResult, error) {
	results := make([]ClientProcessResult, len(clients))
	errorsByClient := make([]error, len(clients))
	var joined sync.WaitGroup
	for index, client := range clients {
		joined.Add(1)
		go func() {
			defer joined.Done()
			timeout := time.Duration(command.DurationNS) + time.Duration(client.config.TimeoutNS) + 20*time.Second
			reply, err := client.read(ctx, command.Phase, timeout)
			errorsByClient[index] = err
			if reply.Result != nil {
				results[index] = *reply.Result
			}
		}()
	}
	joined.Wait()
	return results, errors.Join(errorsByClient...)
}
