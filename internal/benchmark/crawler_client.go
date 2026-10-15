package benchmark

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	weir "github.com/batchstream/weir-go"
)

type crawlerClientConfig struct {
	Path, Profile, SearchURL, Seed, Namespace                      string
	Offset, Threads, PageSize, Processes, ProcessIndex, QueuePages int
	TimeoutNS                                                      int64
}
type crawlerPhase struct {
	Phase               string
	StartNS, DurationNS int64
	ProductsPerSecond   int
}
type crawlerJob struct {
	Page        int
	ScheduledNS int64
}
type crawlerWorkerResult struct {
	RequestedMutations                                                                uint64 `json:"requested_logical_mutations"`
	AppliedPages                                                                      []int  `json:"applied_pages"`
	PID                                                                               int    `json:"pid"`
	Worker                                                                            int    `json:"worker"`
	Offered, Rejected, Completed, Failed, Indeterminate, TimedOut, Late, WithinWindow uint64
	QueuePeak                                                                         int                       `json:"queue_peak_pages"`
	MaxGeneratorLagNS                                                                 int64                     `json:"max_generator_lag_ns"`
	MaxDispatchLagNS                                                                  int64                     `json:"max_dispatch_lag_ns"`
	ElapsedNS                                                                         int64                     `json:"elapsed_ns"`
	ErrorExamples                                                                     []string                  `json:"error_examples,omitempty"`
	Latency                                                                           Latency                   `json:"planned_to_visible_latency"`
	ServiceLatency                                                                    Latency                   `json:"dispatch_to_visible_latency"`
	Buckets                                                                           [256]uint64               `json:"planned_to_visible_buckets"`
	ServiceBuckets                                                                    [256]uint64               `json:"dispatch_to_visible_buckets"`
	Expected                                                                          map[string]map[string]any `json:"expected_documents,omitempty"`
}
type crawlerReply struct {
	Phase   string
	PID     int
	Workers []crawlerWorkerResult
}
type crawlerClient struct {
	config crawlerClientConfig
	http   *http.Client
	sdk    *weir.Client
}

func newCrawlerClient(ctx context.Context, config crawlerClientConfig) (*crawlerClient, error) {
	transport := &http.Transport{Proxy: nil, DisableCompression: true, MaxConnsPerHost: config.Threads, MaxIdleConnsPerHost: config.Threads, MaxIdleConns: config.Threads}
	client := &crawlerClient{config: config, http: &http.Client{Transport: transport}}
	if config.Path != "direct" {
		opts := weir.OpenOptions{Seed: config.Seed, Stores: []string{"search"}}
		sdk, err := weir.Open(ctx, opts)
		if err != nil {
			transport.CloseIdleConnections()
			return nil, err
		}
		client.sdk = sdk
	}
	return client, nil
}
func (c *crawlerClient) close() {
	c.http.CloseIdleConnections()
	if c.sdk != nil {
		_ = c.sdk.Close()
	}
}

type crawlerHTTPRequest struct {
	Method, URL string
	Body        []byte
}

func crawlerHTTP(ctx context.Context, client *http.Client, requestOptions crawlerHTTPRequest) ([]byte, error) {
	request, err := http.NewRequestWithContext(ctx, requestOptions.Method, requestOptions.URL, bytes.NewReader(requestOptions.Body))
	if err != nil {
		return nil, err
	}
	request.Header.Set("Content-Type", "application/json")
	response, err := client.Do(request)
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(response.Body, 32<<20))
	if err != nil {
		return nil, err
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return raw, fmt.Errorf("HTTP %d: %.1000s", response.StatusCode, raw)
	}
	return raw, nil
}

func (c *crawlerClient) execute(ctx context.Context, mutations []crawlerMutation) (bool, error) {
	products := make([]crawlerMutation, 0, c.config.PageSize)
	sellers := make([]crawlerMutation, 0, len(mutations)-c.config.PageSize)
	for _, mutation := range mutations {
		if mutation.Product {
			products = append(products, mutation)
		} else {
			sellers = append(sellers, mutation)
		}
	}
	if c.config.Profile == "two-step" {
		first := append([]crawlerMutation{}, products...)
		for index := range first {
			first[index].ReconcileOnly = true
			products[index].Reconcile = false
		}
		unknown, err := c.executeMutations(ctx, first)
		if err != nil {
			return unknown, err
		}
	}
	unknown, err := c.executeMutations(ctx, products)
	if err != nil {
		return unknown, err
	}
	return c.executeMutations(ctx, sellers)
}

func (c *crawlerClient) executeMutations(ctx context.Context, mutations []crawlerMutation) (bool, error) {
	if c.sdk != nil {
		requests := make([]*weir.MutateRequest, len(mutations))
		for index, mutation := range mutations {
			source := crawlerCommon + crawlerOffer
			if mutation.Product {
				source = crawlerCommon + crawlerProduct
				if mutation.ReconcileOnly {
					source = strings.Replace(source, "        reconcile(current, incoming)", "        return reconcile(current, incoming)", 1)
				}
			}
			raw, err := json.Marshal(mutation)
			if err != nil {
				return false, err
			}
			input := &weir.Document{ContentType: "application/json", Data: raw}
			program := &weir.LuaTransform{Source: []byte(source), Input: input}
			visibility := &weir.MutationOptions{Visibility: weir.MutationQueryVisible}
			request := &weir.MutateRequest{Resource: crawlerIndex(c.config.Namespace, mutation.Product) + "/s:" + mutation.ID, Action: weir.MutationAtomicTransform, Lua: program, Options: visibility}
			requests[index] = request
		}
		opts := weir.MutateOptions{StoreName: "search", Requests: requests}
		replies, err := c.sdk.Mutate(ctx, opts)
		unknown := len(replies) != len(requests)
		var failures []error
		if err != nil {
			failures = append(failures, err)
		}
		for _, reply := range replies {
			if reply == nil || reply.GetOutcome() == weir.MutationUnknown {
				unknown = true
			}
			if reply == nil || reply.GetOutcome() != weir.MutationApplied || reply.GetFailure() != nil {
				failures = append(failures, fmt.Errorf("Weir outcome=%v failure=%v", reply.GetOutcome(), reply.GetFailure()))
			}
		}
		if len(replies) != len(requests) {
			failures = append(failures, errors.New("Weir result count mismatch"))
		}
		return unknown, errors.Join(failures...)
	}
	var body bytes.Buffer
	for _, mutation := range mutations {
		metadata := map[string]any{"_index": crawlerIndex(c.config.Namespace, mutation.Product), "_id": mutation.ID, "retry_on_conflict": 3}
		action := map[string]any{"update": metadata}
		script := map[string]any{"lang": "painless", "source": crawlerNative, "params": mutation}
		payload := map[string]any{"script": script}
		_ = json.NewEncoder(&body).Encode(action)
		_ = json.NewEncoder(&body).Encode(payload)
	}
	requestOptions := crawlerHTTPRequest{Method: http.MethodPost, URL: c.config.SearchURL + "/_bulk?refresh=wait_for&wait_for_active_shards=1&timeout=10s", Body: body.Bytes()}
	raw, err := crawlerHTTP(ctx, c.http, requestOptions)
	if err != nil {
		return true, err
	}
	var result struct {
		Errors bool `json:"errors"`
		Items  []map[string]struct {
			Status int             `json:"status"`
			Result string          `json:"result"`
			Error  json.RawMessage `json:"error"`
			Shards struct {
				Failed     int `json:"failed"`
				Successful int `json:"successful"`
			} `json:"_shards"`
		} `json:"items"`
	}
	if err := json.Unmarshal(raw, &result); err != nil {
		return true, err
	}
	if len(result.Items) != len(mutations) {
		return true, errors.New("native bulk result count mismatch")
	}
	for _, item := range result.Items {
		for _, reply := range item {
			if reply.Status < 200 || reply.Status >= 300 || len(reply.Error) > 0 || reply.Shards.Failed != 0 || reply.Result != "updated" && reply.Result != "noop" || reply.Result == "updated" && reply.Shards.Successful < 1 {
				return false, fmt.Errorf("native bulk item status=%d result=%s error=%s", reply.Status, reply.Result, reply.Error)
			}
		}
	}
	if result.Errors {
		return false, errors.New("native bulk errors")
	}
	return false, nil
}

// RunCrawlerWorker is entered only by the explicit crawler CLI's owned child processes.
func RunCrawlerWorker(ctx context.Context, input io.Reader, output io.Writer) error {
	decoder := json.NewDecoder(io.LimitReader(input, 1<<20))
	decoder.DisallowUnknownFields()
	encoder := json.NewEncoder(output)
	var config crawlerClientConfig
	if err := decoder.Decode(&config); err != nil {
		return err
	}
	if config.Threads < 1 || config.Threads > 128 || config.Processes < 1 || config.PageSize < 1 || config.PageSize > 32 || config.QueuePages < 1 || config.QueuePages > 16 || config.TimeoutNS <= 0 || config.TimeoutNS > int64(time.Minute) || config.Offset < 0 {
		return errors.New("invalid crawler client bounds")
	}
	client, err := newCrawlerClient(ctx, config)
	if err != nil {
		return err
	}
	defer client.close()
	ready := crawlerReply{Phase: "ready", PID: os.Getpid()}
	if err := encoder.Encode(ready); err != nil {
		return err
	}
	for _, phase := range []string{"warmup", "timed"} {
		var command crawlerPhase
		if err := decoder.Decode(&command); err != nil {
			return err
		}
		if command.Phase != phase || command.DurationNS <= 0 || command.DurationNS > int64(10*time.Minute) || command.StartNS <= 0 || command.ProductsPerSecond < 0 {
			return errors.New("invalid crawler phase")
		}
		replies := client.measure(ctx, command)
		expected := crawlerReply{Phase: phase + "-expected", PID: os.Getpid(), Workers: make([]crawlerWorkerResult, len(replies))}
		for index := range replies {
			expected.Workers[index].Worker = replies[index].Worker
			expected.Workers[index].Expected = replies[index].Expected
			replies[index].Expected = nil
		}
		reply := crawlerReply{Phase: phase, PID: os.Getpid(), Workers: replies}
		if err := encoder.Encode(reply); err != nil {
			return err
		}
		if err := encoder.Encode(expected); err != nil {
			return err
		}
	}
	var extra crawlerPhase
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		return errors.New("unexpected crawler phase")
	}
	return nil
}

func (c *crawlerClient) measure(ctx context.Context, phase crawlerPhase) []crawlerWorkerResult {
	results := make([]crawlerWorkerResult, c.config.Threads)
	queues := make([]chan crawlerJob, len(results))
	var workers sync.WaitGroup
	for index := range results {
		queue := make(chan crawlerJob, c.config.QueuePages)
		queues[index] = queue
		workers.Add(1)
		go func() {
			defer workers.Done()
			result := crawlerWorkerResult{PID: os.Getpid(), Worker: c.config.Offset + index, Expected: map[string]map[string]any{}}
			var latency, service histogram
			for job := range queue {
				dispatch := time.Now()
				lag := dispatch.UnixNano() - job.ScheduledNS
				result.MaxDispatchLagNS = max(result.MaxDispatchLagNS, lag)
				mutations := crawlerPage(result.Worker, job.Page, c.config.PageSize)
				deadline := time.Unix(0, job.ScheduledNS).Add(time.Duration(c.config.TimeoutNS))
				operationCtx, cancel := context.WithDeadline(ctx, deadline)
				unknown, err := false, operationCtx.Err()
				if err == nil {
					result.RequestedMutations += uint64(len(mutations))
					if c.config.Profile == "two-step" {
						result.RequestedMutations += uint64(c.config.PageSize)
					}
					unknown, err = c.execute(operationCtx, mutations)
				}
				cancel()
				finished := time.Now()
				latency.add(time.Duration(finished.UnixNano() - job.ScheduledNS))
				service.add(finished.Sub(dispatch))
				result.ElapsedNS = max(result.ElapsedNS, finished.UnixNano()-phase.StartNS)
				if err != nil {
					result.Failed += uint64(c.config.PageSize)
					if unknown {
						result.Indeterminate += uint64(c.config.PageSize)
					}
					if errors.Is(err, context.DeadlineExceeded) || finished.After(deadline) {
						result.TimedOut += uint64(c.config.PageSize)
					}
					if len(result.ErrorExamples) < 3 {
						result.ErrorExamples = append(result.ErrorExamples, err.Error())
					}
					continue
				}
				result.AppliedPages = append(result.AppliedPages, job.Page)
				result.Completed += uint64(c.config.PageSize)
				if finished.UnixNano() <= phase.StartNS+phase.DurationNS {
					result.WithinWindow += uint64(c.config.PageSize)
				} else {
					result.Late += uint64(c.config.PageSize)
				}
				for _, mutation := range mutations {
					current := result.Expected[mutation.ID]
					if current == nil {
						current = crawlerDocument(mutation.ID, mutation.Product)
					}
					result.Expected[mutation.ID] = crawlerMerge(current, mutation)
				}
			}
			result.Latency = latency.summary()
			result.ServiceLatency = service.summary()
			result.Buckets = latency.buckets
			result.ServiceBuckets = service.buckets
			results[index] = result
		}()
	}
	start := time.Unix(0, phase.StartNS)
	if wait := time.Until(start); wait > 0 {
		timer := time.NewTimer(wait)
		select {
		case <-timer.C:
		case <-ctx.Done():
			timer.Stop()
		}
	}
	// Open-loop arrivals use absolute deadlines, independent of worker completions.
	offered := make([]uint64, len(results))
	rejected := make([]uint64, len(results))
	peak := make([]int, len(results))
	lags := make([]int64, len(results))
	if phase.ProductsPerSecond > 0 {
		for ordinal := 0; ctx.Err() == nil; ordinal++ {
			worker := ordinal % len(results)
			scheduled := phase.StartNS + int64(ordinal*c.config.Processes+c.config.ProcessIndex)*int64(c.config.PageSize)*int64(time.Second)/int64(phase.ProductsPerSecond)
			if scheduled >= phase.StartNS+phase.DurationNS {
				break
			}
			wait := time.Until(time.Unix(0, scheduled))
			if wait > 0 {
				timer := time.NewTimer(wait)
				select {
				case <-timer.C:
				case <-ctx.Done():
					timer.Stop()
				}
			}
			lags[worker] = max(lags[worker], time.Now().UnixNano()-scheduled)
			offered[worker] += uint64(c.config.PageSize)
			job := crawlerJob{Page: ordinal / len(results), ScheduledNS: scheduled}
			select {
			case queues[worker] <- job:
				peak[worker] = max(peak[worker], len(queues[worker]))
			default:
				rejected[worker] += uint64(c.config.PageSize)
			}
		}
	} else {
		// Saturation is deliberately closed-loop, without hiding queueing in the rate experiment.
		var generators sync.WaitGroup
		for index, queue := range queues {
			generators.Add(1)
			go func() {
				defer generators.Done()
				for page := 0; time.Now().UnixNano() < phase.StartNS+phase.DurationNS && ctx.Err() == nil; page++ {
					job := crawlerJob{Page: page, ScheduledNS: time.Now().UnixNano()}
					select {
					case queue <- job:
						offered[index] += uint64(c.config.PageSize)
						peak[index] = max(peak[index], len(queue))
					case <-ctx.Done():
						return
					}
				}
			}()
		}
		generators.Wait()
	}
	for _, queue := range queues {
		close(queue)
	}
	workers.Wait()
	for index := range results {
		results[index].Offered = offered[index]
		results[index].Rejected = rejected[index]
		results[index].QueuePeak = peak[index]
		results[index].MaxGeneratorLagNS = lags[index]
	}
	return results
}
