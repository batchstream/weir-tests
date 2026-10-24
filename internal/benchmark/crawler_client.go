package benchmark

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync"
	"time"

	weir "github.com/batchstream/weir-go"
)

type crawlerClientConfig struct {
	InputShape                                                     string
	Visibility                                                     string
	SeedPaddingBytes                                               int
	Path, Profile, SearchURL, Address, Namespace                   string
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
	ApplicationRequests                                                               crawlerApplicationCounts `json:"application_requests"`
	RequestedMutations                                                                uint64                   `json:"requested_logical_mutations"`
	AppliedPages                                                                      []int                    `json:"applied_pages"`
	PID                                                                               int                      `json:"pid"`
	Worker                                                                            int                      `json:"worker"`
	Offered, Rejected, Completed, Failed, Indeterminate, TimedOut, Late, WithinWindow uint64
	QueuePeak                                                                         int          `json:"queue_peak_pages"`
	MaxGeneratorLagNS                                                                 int64        `json:"max_generator_lag_ns"`
	MaxDispatchLagNS                                                                  int64        `json:"max_dispatch_lag_ns"`
	ElapsedNS                                                                         int64        `json:"elapsed_ns"`
	ErrorExamples                                                                     []string     `json:"error_examples,omitempty"`
	Latency                                                                           Latency      `json:"planned_to_visible_latency"`
	ServiceLatency                                                                    Latency      `json:"dispatch_to_visible_latency"`
	Buckets                                                                           [256]uint64  `json:"planned_to_visible_buckets"`
	ServiceBuckets                                                                    [256]uint64  `json:"dispatch_to_visible_buckets"`
	TimeBuckets                                                                       []TimeBucket `json:"time_buckets,omitempty"`
}
type crawlerReply struct {
	Visibility          string
	SeedPaddingBytes    int
	InputShape          string
	NativePoolLimit     int
	ApplicationRequests crawlerApplicationCounts
	Phase               string
	PID                 int
	Workers             []crawlerWorkerResult
}
type crawlerClient struct {
	config crawlerClientConfig
	http   *http.Client
	sdk    *weir.Client
}

func newCrawlerClient(ctx context.Context, config crawlerClientConfig) (*crawlerClient, error) {
	if config.Visibility == "" {
		config.Visibility = "query_visible"
	}
	if config.Visibility != "query_visible" && config.Visibility != "application_acknowledged" || config.Visibility == "application_acknowledged" && config.Profile != "partial-update" || config.SeedPaddingBytes < 0 || config.SeedPaddingBytes > 1<<20 {
		return nil, errors.New("invalid crawler client visibility or seed padding")
	}
	if config.InputShape == "" {
		config.InputShape = "page"
	}
	if config.InputShape != "single" && config.InputShape != "page" || config.InputShape == "single" && config.PageSize != 1 {
		return nil, errors.New("input-shape single requires page-size 1")
	}
	pool := config.Threads
	if config.InputShape == "single" {
		pool *= 3
	}
	transport := &http.Transport{Proxy: nil, DisableCompression: true, MaxConnsPerHost: pool, MaxIdleConnsPerHost: pool, MaxIdleConns: pool}
	client := &crawlerClient{config: config, http: &http.Client{Transport: transport}}
	if config.Path != "direct" {
		sdk, err := weir.New(config.Address)
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

func (c *crawlerClient) execute(ctx context.Context, mutations []crawlerMutation, ledger *crawlerApplicationLedger) (bool, error) {
	if c.config.Profile == "partial-update" {
		return c.executeMutations(ctx, mutations, ledger)
	}
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
		unknown, err := c.executeMutations(ctx, first, ledger)
		if err != nil {
			return unknown, err
		}
	}
	unknown, err := c.executeMutations(ctx, products, ledger)
	if err != nil {
		return unknown, err
	}
	if c.config.InputShape != "single" {
		return c.executeMutations(ctx, sellers, ledger)
	}
	type sellerResult struct {
		unknown bool
		err     error
	}
	results := make(chan sellerResult, len(sellers))
	for _, seller := range sellers {
		go func() {
			one := []crawlerMutation{seller}
			unknown, err := c.executeMutations(ctx, one, ledger)
			result := sellerResult{unknown: unknown, err: err}
			results <- result
		}()
	}
	var failures []error
	unknown = false
	for range sellers {
		result := <-results
		unknown = unknown || result.unknown
		if result.err != nil {
			failures = append(failures, result.err)
		}
	}
	return unknown, errors.Join(failures...)
}

func (c *crawlerClient) executeMutations(ctx context.Context, mutations []crawlerMutation, ledger *crawlerApplicationLedger) (bool, error) {
	if ledger == nil {
		return false, errors.New("application request ledger is required")
	}
	if err := ctx.Err(); err != nil {
		return false, err
	}
	if c.config.InputShape == "single" && len(mutations) != 1 {
		return false, errors.New("single-record application call contains a client batch")
	}
	if c.sdk == nil && c.config.InputShape == "single" {
		return c.executeSingleNative(ctx, mutations[0], ledger)
	}
	if c.sdk != nil {
		requests, err := crawlerWeirRequests(c.config, mutations)
		if err != nil {
			return false, err
		}
		opts := weir.MutateOptions{StoreName: "search", Requests: requests}
		ledger.record(len(requests))
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
		metadata := map[string]any{"_index": crawlerIndex(c.config.Namespace, mutation.Product), "_id": mutation.ID, "retry_on_conflict": 4}
		action := map[string]any{"update": metadata}
		script := map[string]any{"lang": "painless", "source": crawlerNative, "params": mutation}
		payload := map[string]any{"script": script}
		_ = json.NewEncoder(&body).Encode(action)
		_ = json.NewEncoder(&body).Encode(payload)
	}
	requestOptions := crawlerHTTPRequest{Method: http.MethodPost, URL: c.config.SearchURL + "/_bulk?refresh=wait_for&wait_for_active_shards=1&timeout=10s", Body: body.Bytes()}
	ledger.record(len(mutations))
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

func crawlerWeirRequests(config crawlerClientConfig, mutations []crawlerMutation) ([]*weir.MutateRequest, error) {
	if config.InputShape == "single" && len(mutations) != 1 {
		return nil, errors.New("single-record SDK call contains a client batch")
	}
	requests := make([]*weir.MutateRequest, len(mutations))
	for index, mutation := range mutations {
		if config.Profile == "partial-update" {
			request, err := partialUpdateRequest(config, mutation)
			if err != nil {
				return nil, err
			}
			requests[index] = request
			continue
		}
		source := crawlerCommon + crawlerOffer
		if mutation.Product {
			source = crawlerCommon + crawlerProduct
			if mutation.ReconcileOnly {
				source = strings.Replace(source, "        reconcile(current, incoming)", "        return reconcile(current, incoming)", 1)
			}
		}
		raw, err := json.Marshal(mutation)
		if err != nil {
			return nil, err
		}
		input := &weir.Document{ContentType: "application/json", Data: raw}
		program := &weir.LuaTransform{Source: []byte(source), Input: input}
		visibility := &weir.MutationOptions{Visibility: weir.MutationQueryVisible}
		request := &weir.MutateRequest{Resource: crawlerIndex(config.Namespace, mutation.Product) + "/s:" + mutation.ID, Action: weir.MutationAtomicTransform, Lua: program, Options: visibility}
		requests[index] = request
	}
	return requests, nil
}

func (c *crawlerClient) executeSingleNative(ctx context.Context, mutation crawlerMutation, ledger *crawlerApplicationLedger) (bool, error) {
	var payload map[string]any
	if c.config.Profile == "partial-update" {
		payload = map[string]any{"doc": mutation.Document}
	} else {
		script := map[string]any{"lang": "painless", "source": crawlerNative, "params": mutation}
		payload = map[string]any{"script": script}
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return false, err
	}
	query := url.Values{"refresh": {"wait_for"}, "wait_for_active_shards": {"1"}, "timeout": {"10s"}, "retry_on_conflict": {"4"}}
	if c.config.Profile == "partial-update" {
		query.Set("retry_on_conflict", "0")
		if c.config.Visibility == "application_acknowledged" {
			query.Set("refresh", "false")
		}
	}
	request := crawlerHTTPRequest{Method: http.MethodPost, URL: c.config.SearchURL + "/" + crawlerIndex(c.config.Namespace, mutation.Product) + "/_update/" + url.PathEscape(mutation.ID) + "?" + query.Encode(), Body: body}
	ledger.record(1)
	raw, err := crawlerHTTP(ctx, c.http, request)
	if err != nil {
		return true, err
	}
	var reply struct {
		ID     string `json:"_id"`
		Index  string `json:"_index"`
		Result string `json:"result"`
		Shards struct {
			Successful *int `json:"successful"`
			Failed     *int `json:"failed"`
		} `json:"_shards"`
	}
	if err := json.Unmarshal(raw, &reply); err != nil {
		return true, err
	}
	if reply.ID != mutation.ID || reply.Index != crawlerIndex(c.config.Namespace, mutation.Product) || reply.Result != "updated" && reply.Result != "noop" || reply.Shards.Successful == nil || reply.Shards.Failed == nil || *reply.Shards.Failed != 0 || reply.Result == "updated" && *reply.Shards.Successful < 1 {
		return true, errors.New("native single update lacks its complete durable acknowledgement")
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
	if config.InputShape == "" {
		config.InputShape = "page"
	}
	if config.InputShape != "single" && config.InputShape != "page" || config.InputShape == "single" && config.PageSize != 1 {
		return errors.New("invalid crawler input shape")
	}
	if config.Threads < 1 || config.Threads > 512 || config.Processes < 1 || config.PageSize < 1 || config.PageSize > 32 || config.QueuePages < 1 || config.QueuePages > 16 || config.TimeoutNS <= 0 || config.TimeoutNS > int64(time.Minute) || config.Offset < 0 {
		return errors.New("invalid crawler client bounds")
	}
	client, err := newCrawlerClient(ctx, config)
	if err != nil {
		return err
	}
	defer client.close()
	pool := 0
	if config.Path == "direct" {
		pool = config.Threads
		if config.InputShape == "single" {
			pool *= 3
		}
	}
	ready := crawlerReply{Phase: "ready", PID: os.Getpid(), Visibility: client.config.Visibility, SeedPaddingBytes: client.config.SeedPaddingBytes, InputShape: client.config.InputShape, NativePoolLimit: pool}
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
		var applicationCounts crawlerApplicationCounts
		for _, worker := range replies {
			mergeCrawlerApplicationCounts(&applicationCounts, worker.ApplicationRequests)
		}
		reply := crawlerReply{Phase: phase, PID: os.Getpid(), Workers: replies, Visibility: client.config.Visibility, SeedPaddingBytes: client.config.SeedPaddingBytes, InputShape: client.config.InputShape, NativePoolLimit: pool, ApplicationRequests: applicationCounts}
		if err := encoder.Encode(reply); err != nil {
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
	workerBuckets := make([][]TimeBucket, len(results))
	type arrivalCounts struct{ offered, rejected, invalidOffsets uint64 }
	arrivals := make([][]arrivalCounts, len(results))
	for index := range workerBuckets {
		workerBuckets[index] = newTimeBuckets(phase.DurationNS, phase.Phase == "timed")
		arrivals[index] = make([]arrivalCounts, len(workerBuckets[index]))
	}
	var workers sync.WaitGroup
	for index := range results {
		queue := make(chan crawlerJob, c.config.QueuePages)
		queues[index] = queue
		workers.Add(1)
		go func() {
			defer workers.Done()
			result := crawlerWorkerResult{PID: os.Getpid(), Worker: c.config.Offset + index, TimeBuckets: workerBuckets[index]}
			var latency, service histogram
			var applications crawlerApplicationLedger
			for job := range queue {
				dispatch := time.Now()
				lag := dispatch.UnixNano() - job.ScheduledNS
				result.MaxDispatchLagNS = max(result.MaxDispatchLagNS, lag)
				var mutations []crawlerMutation
				if c.config.Profile == "partial-update" {
					mutation := partialUpdateMutation(result.Worker, job.Page)
					mutations = []crawlerMutation{mutation}
				} else {
					mutations = crawlerPage(result.Worker, job.Page, c.config.PageSize)
				}
				deadline := time.Unix(0, job.ScheduledNS).Add(time.Duration(c.config.TimeoutNS))
				operationCtx, cancel := context.WithDeadline(ctx, deadline)
				unknown, err := false, operationCtx.Err()
				if err == nil {
					result.RequestedMutations += uint64(len(mutations))
					if c.config.Profile == "two-step" {
						result.RequestedMutations += uint64(c.config.PageSize)
					}
					unknown, err = c.execute(operationCtx, mutations, &applications)
				}
				cancel()
				finished := time.Now()
				latency.add(time.Duration(finished.UnixNano() - job.ScheduledNS))
				service.add(finished.Sub(dispatch))
				result.ElapsedNS = max(result.ElapsedNS, finished.UnixNano()-phase.StartNS)
				timedOut := err != nil && (errors.Is(err, context.DeadlineExceeded) || finished.After(deadline))
				event := timeBucketOutcome{OffsetNS: finished.UnixNano() - phase.StartNS, Latency: time.Duration(finished.UnixNano() - job.ScheduledNS), Items: uint64(c.config.PageSize), Failed: err != nil, Unknown: err != nil && unknown, TimedOut: timedOut}
				recordTimeBucketOutcome(result.TimeBuckets, event)
				if err != nil {
					result.Failed += uint64(c.config.PageSize)
					if unknown {
						result.Indeterminate += uint64(c.config.PageSize)
					}
					if timedOut {
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
			}
			result.ApplicationRequests = applications.snapshot()
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
		timer := time.NewTimer(time.Hour)
		timer.Stop()
		defer timer.Stop()
		for ordinal := 0; ctx.Err() == nil; ordinal++ {
			worker := ordinal % len(results)
			scheduled := phase.StartNS + int64(ordinal*c.config.Processes+c.config.ProcessIndex)*int64(c.config.PageSize)*int64(time.Second)/int64(phase.ProductsPerSecond)
			if scheduled >= phase.StartNS+phase.DurationNS {
				break
			}
			wait := time.Until(time.Unix(0, scheduled))
			if wait > 0 {
				timer.Reset(wait)
				select {
				case <-timer.C:
				case <-ctx.Done():
					timer.Stop()
				}
			}
			lags[worker] = max(lags[worker], time.Now().UnixNano()-scheduled)
			offered[worker] += uint64(c.config.PageSize)
			var arrival *arrivalCounts
			if len(arrivals[worker]) > 0 {
				arrival = &arrivals[worker][timeBucketIndex(workerBuckets[worker], scheduled-phase.StartNS)]
				arrival.offered += uint64(c.config.PageSize)
				if scheduled < phase.StartNS {
					arrival.invalidOffsets++
				}
			}
			job := crawlerJob{Page: ordinal / len(results), ScheduledNS: scheduled}
			select {
			case queues[worker] <- job:
				peak[worker] = max(peak[worker], len(queues[worker]))
			default:
				rejected[worker] += uint64(c.config.PageSize)
				if arrival != nil {
					arrival.rejected += uint64(c.config.PageSize)
				}
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
						if len(arrivals[index]) > 0 {
							arrival := &arrivals[index][timeBucketIndex(workerBuckets[index], job.ScheduledNS-phase.StartNS)]
							arrival.offered += uint64(c.config.PageSize)
							if job.ScheduledNS < phase.StartNS {
								arrival.invalidOffsets++
							}
						}
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
		for bin, arrival := range arrivals[index] {
			bucket := &results[index].TimeBuckets[bin]
			bucket.Offered = arrival.offered
			bucket.Rejected = arrival.rejected
			bucket.InvalidOffsets += arrival.invalidOffsets
		}
		finishTimeBuckets(results[index].TimeBuckets)
	}
	return results
}
