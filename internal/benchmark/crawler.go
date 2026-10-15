package benchmark

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"reflect"
	"strings"
	"sync"
	"time"

	"github.com/batchstream/weir-tests/internal/fixture"
	"github.com/batchstream/weir-tests/internal/observe"
)

type CrawlerOptions struct {
	Before, After, BeforeReceipt, AfterReceipt, Executable, Output string
	Rates, Concurrency                                             []int
	Profiles                                                       []string
	Processes, PageSize, QueuePages, Rounds                        int
	AfterMaxInFlight                                               int
	CPUs                                                           float64
	Warmup, Duration, Timeout                                      time.Duration
}
type crawlerServerReceipt struct {
	Path    string          `json:"path"`
	Receipt json.RawMessage `json:"receipt"`
	SHA256  string          `json:"sha256"`
}
type CrawlerReport struct {
	BusinessSources          map[string]string
	Schema                   int `json:"schema"`
	Started, Method, Fixture string
	Parameters               CrawlerOptions
	Sources                  map[string]string
	Servers                  map[string]crawlerServerReceipt
	Stages                   []crawlerStage
	Incomplete               string `json:"incomplete,omitempty"`
	CleanupError             string `json:"cleanup_error,omitempty"`
}
type crawlerStage struct {
	Profile                                                                           string
	RequestedMutations                                                                uint64
	ExpectedDocumentsSHA256                                                           string
	VerifiedDocuments                                                                 int
	ClientsExited                                                                     bool
	Path, Mode                                                                        string
	Round, Concurrency, ProductsPerSecond                                             int
	CPUQuota                                                                          *DatabaseCPUQuota
	ServerPID                                                                         int
	ServerSHA256                                                                      string
	StartedNS, ElapsedNS, DrainNS                                                     int64
	Offered, Rejected, Completed, Failed, Indeterminate, TimedOut, Late, WithinWindow uint64
	CompletedProductsPerSecond                                                        float64
	ScheduledToVisible, DispatchToVisible                                             Latency
	GeneratorLagNS, DispatchLagNS                                                     int64
	Clients                                                                           []crawlerReply
	Resources                                                                         Resources
	DatabaseBefore, DatabaseAfter                                                     json.RawMessage
	HTTPBefore, HTTPAfter, HTTPDelta                                                  map[string]crawlerHTTPCount
	ServerBefore, ServerAfter                                                         observe.Snapshot
	Verified                                                                          bool
	VerificationError                                                                 string   `json:"verification_error,omitempty"`
	ClientExitErrors                                                                  []string `json:"client_exit_errors,omitempty"`
}
type crawlerProcess struct {
	command *exec.Cmd
	input   io.WriteCloser
	decoder *json.Decoder
	log     bytes.Buffer
}
type crawlerCase struct {
	Profile       string
	Mode          string
	Workers, Rate int
}

func RunCrawler(ctx context.Context, opts CrawlerOptions) (report *CrawlerReport, resultErr error) {
	if opts.AfterMaxInFlight < 0 || opts.AfterMaxInFlight > 4096 || opts.Warmup > time.Minute || opts.Processes < 1 || opts.Processes > 8 || opts.PageSize < 1 || opts.PageSize > 32 || opts.QueuePages < 1 || opts.QueuePages > 16 || opts.Rounds < 1 || opts.Rounds > 5 || opts.Warmup < time.Second || opts.Duration < time.Second || opts.Duration > 10*time.Minute || opts.Timeout < time.Second || opts.Timeout > time.Minute || len(opts.Concurrency) == 0 || len(opts.Concurrency) > 4 || len(opts.Rates) > 4 {
		return nil, errors.New("invalid bounded crawler benchmark parameters")
	}
	for _, workers := range opts.Concurrency {
		if workers%opts.Processes != 0 || workers < opts.Processes || workers > 256 {
			return nil, errors.New("crawler concurrency must divide between processes and be <=256")
		}
	}
	for _, rate := range opts.Rates {
		if rate < 1 || rate > 100000 {
			return nil, errors.New("crawler target rate must be positive and <=100000 products/s")
		}
	}
	if len(opts.Profiles) < 1 || len(opts.Profiles) > 2 {
		return nil, errors.New("one or two explicit crawler profiles required")
	}
	for _, profile := range opts.Profiles {
		if profile != "combined" && profile != "two-step" {
			return nil, errors.New("crawler profile must be combined or two-step")
		}
	}
	servers := map[string]crawlerServerReceipt{}
	for name, pair := range map[string][2]string{"current": {opts.Before, opts.BeforeReceipt}, "optimized": {opts.After, opts.AfterReceipt}} {
		data, err := os.ReadFile(pair[0])
		if err != nil {
			return nil, err
		}
		raw, err := os.ReadFile(pair[1])
		if err != nil {
			return nil, err
		}
		var receipt struct {
			SHA256, Source string
			SourceDirty    bool `json:"source_dirty"`
		}
		if err := json.Unmarshal(raw, &receipt); err != nil {
			return nil, err
		}
		sum := sha256.Sum256(data)
		digest := hex.EncodeToString(sum[:])
		if receipt.SHA256 != digest || len(receipt.Source) != 40 || receipt.SourceDirty {
			return nil, errors.New("crawler binary receipt source or hash mismatch")
		}
		server := crawlerServerReceipt{Path: pair[0], Receipt: raw, SHA256: digest}
		servers[name] = server
	}
	origins := map[string]string{"original_pse": "7f36b501cded06a1bcc7484575346bb9c4d6311b", "optimized_pse": "8725e4743c6ddba66132874e0e0fe9d393ab4a85", "weir_lua_translation_base": "283940ef442b479440e45b39c86988351b92d7b5"}
	report = &CrawlerReport{BusinessSources: origins, Schema: 1, Started: time.Now().UTC().Format(time.RFC3339Nano), Parameters: opts, Sources: crawlerSources(), Servers: servers, Method: "Three-way same-runner rotation; native atomic Elasticsearch scripted bulk versus the same frozen PSE Lua on current and optimized Weir; 1 product plus 2 seller documents per successful product; per-process pools remain open across warmup/reset/timed phases; QUERY_VISIBLE / refresh=wait_for, two PSE indices, each with one primary shard and zero replicas, request translog durability; bounded per-worker open-loop queues with absolute arrival deadlines; latency starts at scheduled arrival, all drains included in completed throughput and database CPU; full independent Go domain oracle versus normal searchable persisted sources; 1 shared no-change seller per 100 pages, no measured production distribution claimed; native retry_on_conflict=3 and both Weir retry_attempts=4 (same four total attempts); native script supports every generated value, not arbitrary user Lua; no Sink baseline"}
	defer func() {
		if resultErr != nil {
			report.Incomplete = resultErr.Error()
		}
		raw, err := json.MarshalIndent(report, "", "  ")
		if err == nil {
			err = writeAtomic(opts.Output, append(raw, '\n'))
		}
		resultErr = errors.Join(resultErr, err)
	}()
	fixtureOptions := fixture.Options{WeirBinary: opts.Before, Backends: []string{"search"}, OwnerCount: 1, BatchSize: 32, SearchRetryAttempts: 4, PendingRecords: 4096, DatabaseCPUs: opts.CPUs}
	cluster, err := fixture.Start(ctx, fixtureOptions)
	if err != nil {
		return report, err
	}
	report.Fixture = cluster.Directory
	defer func() {
		cleanupCtx, cancel := context.WithTimeout(context.Background(), 75*time.Second)
		defer cancel()
		err := cluster.Close(cleanupCtx)
		if err != nil {
			report.CleanupError = err.Error()
		}
		resultErr = errors.Join(resultErr, err)
	}()
	observer, err := startCrawlerObserver(cluster.SearchURL)
	if err != nil {
		return report, err
	}
	defer func() {
		cleanupCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		resultErr = errors.Join(resultErr, observer.close(cleanupCtx))
	}()
	cluster.SearchURL = observer.URL
	namespace := fmt.Sprintf("weircrawler_%d", time.Now().UnixNano())
	client := &http.Client{Timeout: time.Minute}
	defer client.CloseIdleConnections()
	for _, product := range []bool{true, false} {
		create := crawlerHTTPRequest{Method: http.MethodPut, URL: cluster.SearchURL + "/" + crawlerIndex(namespace, product), Body: []byte(`{"settings":{"number_of_shards":1,"number_of_replicas":0,"refresh_interval":"1s","translog.durability":"request"}}`)}
		if _, err := crawlerHTTP(ctx, client, create); err != nil {
			return report, err
		}
	}
	var cases []crawlerCase
	for _, profile := range opts.Profiles {
		for _, rate := range opts.Rates {
			test := crawlerCase{Profile: profile, Mode: "fixed-rate", Workers: opts.Concurrency[len(opts.Concurrency)-1], Rate: rate}
			cases = append(cases, test)
		}
		for _, workers := range opts.Concurrency {
			test := crawlerCase{Profile: profile, Mode: "saturation", Workers: workers}
			cases = append(cases, test)
		}
	}
	paths := []string{"direct", "current", "optimized"}
	for caseIndex, test := range cases {
		for round := range opts.Rounds {
			for offset := range paths {
				path := paths[(caseIndex+round+offset)%len(paths)]
				binary := opts.Before
				if path == "optimized" {
					binary = opts.After
				}
				replacement := fixture.ServerReplacement{Binary: binary}
				if path == "optimized" {
					replacement.MaxInFlight = opts.AfterMaxInFlight
				}
				if err := cluster.ReplaceServer(ctx, replacement); err != nil {
					return report, err
				}
				reset := crawlerResetOptions{URL: cluster.SearchURL, Namespace: namespace, Workers: opts.Concurrency[len(opts.Concurrency)-1], PageSize: opts.PageSize}
				if err := resetCrawler(ctx, client, reset); err != nil {
					return report, err
				}
				stageOpts := crawlerStageOptions{Options: opts, Case: test, Path: path, Round: round + 1, Namespace: namespace, Cluster: cluster, Observer: observer, HTTP: client}
				stage, err := runCrawlerStage(ctx, stageOpts)
				report.Stages = append(report.Stages, stage)
				raw, _ := json.MarshalIndent(report, "", "  ")
				if writeErr := writeAtomic(opts.Output, append(raw, '\n')); writeErr != nil {
					return report, writeErr
				}
				fmt.Printf("crawler mode=%s rate=%d workers=%d round=%d path=%s success=%d rejected=%d errors=%d completed/s=%.2f verified=%t\n", test.Mode, test.Rate, test.Workers, round+1, path, stage.Completed, stage.Rejected, stage.Failed, stage.CompletedProductsPerSecond, stage.Verified)
				if err != nil {
					if ctx.Err() != nil {
						return report, err
					}
					report.Incomplete += fmt.Sprintf("%s/%s round %d: %v; ", path, test.Mode, round+1, err)
				}
			}
		}
	}
	if report.Incomplete != "" {
		return report, errors.New(report.Incomplete)
	}
	return report, nil
}

type crawlerResetOptions struct {
	URL, Namespace    string
	Workers, PageSize int
}

func resetCrawler(ctx context.Context, client *http.Client, opts crawlerResetOptions) error {
	base, namespace, workers, pageSize := opts.URL, opts.Namespace, opts.Workers, opts.PageSize
	var body bytes.Buffer
	for worker := range workers {
		for record := range pageSize * 2 {
			for _, suffix := range []string{"product", "seller-0", "seller-1"} {
				id := crawlerID(worker, record, suffix)
				meta := map[string]any{"_index": crawlerIndex(namespace, suffix == "product"), "_id": id}
				action := map[string]any{"index": meta}
				_ = json.NewEncoder(&body).Encode(action)
				_ = json.NewEncoder(&body).Encode(crawlerDocument(id, suffix == "product"))
			}
		}
	}
	id := "shared-seller"
	meta := map[string]any{"_index": crawlerIndex(namespace, false), "_id": id}
	action := map[string]any{"index": meta}
	_ = json.NewEncoder(&body).Encode(action)
	_ = json.NewEncoder(&body).Encode(crawlerDocument(id, false))
	request := crawlerHTTPRequest{Method: http.MethodPost, URL: base + "/_bulk?refresh=true", Body: body.Bytes()}
	raw, err := crawlerHTTP(ctx, client, request)
	if err != nil {
		return err
	}
	var reply struct {
		Errors bool `json:"errors"`
	}
	if err := json.Unmarshal(raw, &reply); err != nil {
		return err
	}
	if reply.Errors {
		return errors.New("crawler setup bulk failed")
	}
	return nil
}

type crawlerStageOptions struct {
	Options         CrawlerOptions
	Case            crawlerCase
	Path, Namespace string
	Round           int
	Cluster         *fixture.Cluster
	Observer        *crawlerObserver
	HTTP            *http.Client
}

func runCrawlerStage(ctx context.Context, opts crawlerStageOptions) (stage crawlerStage, resultErr error) {
	config := opts.Options
	stage = crawlerStage{Profile: opts.Case.Profile, Path: opts.Path, Mode: opts.Case.Mode, Round: opts.Round, Concurrency: opts.Case.Workers, ProductsPerSecond: opts.Case.Rate, ServerPID: opts.Cluster.Nodes[0].ProcessID}
	binary := config.Before
	if opts.Path == "optimized" {
		binary = config.After
	}
	binaryRaw, err := os.ReadFile(binary)
	if err != nil {
		return stage, err
	}
	binarySum := sha256.Sum256(binaryRaw)
	stage.ServerSHA256 = hex.EncodeToString(binarySum[:])
	var clients []*crawlerProcess
	defer func() {
		for _, client := range clients {
			_ = client.input.Close()
			if err := client.command.Wait(); err != nil {
				stage.ClientExitErrors = append(stage.ClientExitErrors, err.Error()+": "+client.log.String())
				resultErr = errors.Join(resultErr, err)
			}
		}
		stage.ClientsExited = len(clients) == config.Processes && len(stage.ClientExitErrors) == 0
	}()
	for process := range config.Processes {
		clientConfig := crawlerClientConfig{Profile: opts.Case.Profile, Path: opts.Path, SearchURL: opts.Cluster.SearchURL, Seed: opts.Cluster.Nodes[0].Application, Namespace: opts.Namespace, Offset: process * (opts.Case.Workers / config.Processes), Threads: opts.Case.Workers / config.Processes, PageSize: config.PageSize, Processes: config.Processes, ProcessIndex: process, QueuePages: config.QueuePages, TimeoutNS: int64(config.Timeout)}
		client, err := startCrawlerProcess(ctx, config.Executable, clientConfig)
		if err != nil {
			return stage, err
		}
		clients = append(clients, client)
	}
	warm := crawlerPhase{Phase: "warmup", StartNS: time.Now().Add(200 * time.Millisecond).UnixNano(), DurationNS: int64(config.Warmup), ProductsPerSecond: opts.Case.Rate}
	warmReplies, err := crawlerPhaseAll(clients, warm)
	if err != nil {
		return stage, err
	}
	for _, reply := range warmReplies {
		for _, worker := range reply.Workers {
			if worker.Failed > 0 {
				return stage, fmt.Errorf("crawler warmup failed: %v", worker.ErrorExamples)
			}
		}
	}
	reset := crawlerResetOptions{URL: opts.Cluster.SearchURL, Namespace: opts.Namespace, Workers: config.Concurrency[len(config.Concurrency)-1], PageSize: config.PageSize}
	if err := resetCrawler(ctx, opts.HTTP, reset); err != nil {
		return stage, err
	}
	target := opts.Cluster.Resources("search")
	quota, err := verifyResourceBudget(ctx, target)
	if err != nil {
		return stage, err
	}
	stage.CPUQuota = quota
	monitor, err := resourceHTTPClient(target)
	if err != nil {
		return stage, err
	}
	defer monitor.CloseIdleConnections()
	stage.DatabaseBefore, err = crawlerStats(ctx, opts.HTTP, opts.Cluster.SearchURL, opts.Namespace)
	if err != nil {
		return stage, err
	}
	stage.HTTPBefore = opts.Observer.snapshot()
	stage.ServerBefore = fetchServerMetrics(ctx, target.Diagnostics)
	phase := crawlerPhase{Phase: "timed", StartNS: time.Now().Add(200 * time.Millisecond).UnixNano(), DurationNS: int64(config.Duration), ProductsPerSecond: opts.Case.Rate}
	stage.StartedNS = phase.StartNS
	for _, client := range clients {
		if err := json.NewEncoder(client.input).Encode(phase); err != nil {
			return stage, err
		}
	}
	start := time.Unix(0, phase.StartNS)
	if delay := time.Until(start); delay > 0 {
		timer := time.NewTimer(delay)
		select {
		case <-timer.C:
		case <-ctx.Done():
			timer.Stop()
			return stage, ctx.Err()
		}
	}
	resources := Resources{AllocatedCPUs: target.AllocatedCPUs, Source: "Owned Docker cumulative cgroup CPU, network and block bytes; every timed stage from scheduled start through completion drain"}
	stop := make(chan struct{})
	var sampling sync.WaitGroup
	sampling.Add(1)
	go func() {
		defer sampling.Done()
		for {
			sampleCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
			sampleOpts := resourceSampleOptions{Target: target, Client: monitor, Started: start}
			for _, client := range clients {
				sampleOpts.ClientPIDs = append(sampleOpts.ClientPIDs, client.command.Process.Pid)
			}
			sample := sampleResources(sampleCtx, sampleOpts)
			cancel()
			resources.Samples = append(resources.Samples, sample)
			select {
			case <-stop:
				return
			case <-time.After(time.Second):
			}
		}
	}()
	replies := make([]crawlerReply, len(clients))
	for index, client := range clients {
		if err := client.decoder.Decode(&replies[index]); err != nil {
			resultErr = errors.Join(resultErr, fmt.Errorf("crawler timed decode: %w; %s", err, client.log.String()))
			continue
		}
		if replies[index].Phase != "timed" || replies[index].PID != client.command.Process.Pid || len(replies[index].Workers) != opts.Case.Workers/config.Processes {
			resultErr = errors.Join(resultErr, errors.New("crawler timed child identity mismatch"))
		}
	}
	if wait := time.Until(start.Add(config.Duration)); wait > 0 {
		timer := time.NewTimer(wait)
		select {
		case <-timer.C:
		case <-ctx.Done():
			timer.Stop()
		}
	}
	stage.ElapsedNS = time.Since(start).Nanoseconds()
	stage.DrainNS = max(0, stage.ElapsedNS-phase.DurationNS)
	close(stop)
	sampling.Wait()
	lastOpts := resourceSampleOptions{Target: target, Client: monitor, Started: start}
	resources.Samples = append(resources.Samples, sampleResources(ctx, lastOpts))
	resources.MeasurementNS = stage.ElapsedNS
	resources.qualify(90)
	stage.Resources = resources
	stage.HTTPAfter = opts.Observer.snapshot()
	stage.HTTPDelta = crawlerHTTPDelta(stage.HTTPBefore, stage.HTTPAfter)
	stage.DatabaseAfter, err = crawlerStats(ctx, opts.HTTP, opts.Cluster.SearchURL, opts.Namespace)
	resultErr = errors.Join(resultErr, err)
	stage.ServerAfter = fetchServerMetrics(ctx, target.Diagnostics)
	for index := range replies {
		if err := readCrawlerExpected(clients[index], &replies[index]); err != nil {
			resultErr = errors.Join(resultErr, err)
		}
	}
	stage.Clients = replies
	var latency, service histogram
	expected := map[string]map[string]any{}
	// Verify every seeded record, including untouched records and records rejected at admission.
	for worker := range config.Concurrency[len(config.Concurrency)-1] {
		for record := range config.PageSize * 2 {
			for _, suffix := range []string{"product", "seller-0", "seller-1"} {
				id := crawlerID(worker, record, suffix)
				expected[id] = crawlerDocument(id, suffix == "product")
			}
		}
	}
	expected["shared-seller"] = crawlerDocument("shared-seller", false)
	for _, reply := range replies {
		for _, worker := range reply.Workers {
			stage.RequestedMutations += worker.RequestedMutations
			stage.Offered += worker.Offered
			stage.Rejected += worker.Rejected
			stage.Completed += worker.Completed
			stage.Failed += worker.Failed
			stage.Indeterminate += worker.Indeterminate
			stage.TimedOut += worker.TimedOut
			stage.Late += worker.Late
			stage.WithinWindow += worker.WithinWindow
			stage.GeneratorLagNS = max(stage.GeneratorLagNS, worker.MaxGeneratorLagNS)
			stage.DispatchLagNS = max(stage.DispatchLagNS, worker.MaxDispatchLagNS)
			childLatency := histogram{buckets: worker.Buckets, count: worker.Latency.Samples, max: worker.Latency.MaxNS}
			childService := histogram{buckets: worker.ServiceBuckets, count: worker.ServiceLatency.Samples, max: worker.ServiceLatency.MaxNS}
			latency.merge(&childLatency)
			service.merge(&childService)
			for id, document := range worker.Expected {
				if old := expected[id]; id == "shared-seller" && !reflect.DeepEqual(old, document) {
					resultErr = errors.Join(resultErr, errors.New("shared seller changed"))
				}
				expected[id] = document
			}
		}
	}
	stage.ScheduledToVisible = latency.summary()
	stage.DispatchToVisible = service.summary()
	stage.CompletedProductsPerSecond = float64(stage.Completed) / (float64(stage.ElapsedNS) / 1e9)
	if stage.Offered != stage.Rejected+stage.Completed+stage.Failed {
		resultErr = errors.Join(resultErr, errors.New("crawler offered accounting mismatch"))
	}
	expectedRaw, _ := json.Marshal(expected)
	expectedSum := sha256.Sum256(expectedRaw)
	stage.ExpectedDocumentsSHA256 = hex.EncodeToString(expectedSum[:])
	verify := crawlerVerifyOptions{URL: opts.Cluster.SearchURL, Namespace: opts.Namespace, Expected: expected}
	if err := verifyCrawler(ctx, opts.HTTP, verify); err != nil {
		stage.VerificationError = err.Error()
		resultErr = errors.Join(resultErr, err)
	} else {
		stage.Verified = true
		stage.VerifiedDocuments = len(expected)
	}
	for replyIndex := range stage.Clients {
		for workerIndex := range stage.Clients[replyIndex].Workers {
			stage.Clients[replyIndex].Workers[workerIndex].Expected = nil
		}
	}
	// Overload is retained as a measured outcome; semantics/unknown results invalidate the experiment.
	if stage.Failed > 0 || stage.Indeterminate > 0 {
		resultErr = errors.Join(resultErr, errors.New("crawler business write failure or unknown outcome"))
	}
	return stage, resultErr
}

func startCrawlerProcess(ctx context.Context, executable string, config crawlerClientConfig) (*crawlerProcess, error) {
	command := exec.CommandContext(ctx, executable, "client-worker")
	client := &crawlerProcess{command: command}
	input, err := command.StdinPipe()
	if err != nil {
		return nil, err
	}
	output, err := command.StdoutPipe()
	if err != nil {
		return nil, err
	}
	client.input = input
	client.decoder = json.NewDecoder(output)
	command.Stderr = &client.log
	if err := command.Start(); err != nil {
		return nil, err
	}
	if err := json.NewEncoder(input).Encode(config); err != nil {
		_ = input.Close()
		_ = command.Wait()
		return nil, err
	}
	var ready crawlerReply
	if err := client.decoder.Decode(&ready); err != nil {
		_ = input.Close()
		_ = command.Wait()
		return nil, fmt.Errorf("crawler client ready: %w; %s", err, client.log.String())
	}
	if ready.Phase != "ready" || ready.PID != command.Process.Pid {
		_ = input.Close()
		_ = command.Wait()
		return nil, errors.New("crawler child identity mismatch")
	}
	return client, nil
}
func crawlerPhaseAll(clients []*crawlerProcess, phase crawlerPhase) ([]crawlerReply, error) {
	for _, client := range clients {
		if err := json.NewEncoder(client.input).Encode(phase); err != nil {
			return nil, err
		}
	}
	replies := make([]crawlerReply, len(clients))
	for index, client := range clients {
		if err := client.decoder.Decode(&replies[index]); err != nil {
			return nil, err
		}
		if err := readCrawlerExpected(client, &replies[index]); err != nil {
			return nil, err
		}
	}
	return replies, nil
}
func readCrawlerExpected(client *crawlerProcess, summary *crawlerReply) error {
	var reply crawlerReply
	if err := client.decoder.Decode(&reply); err != nil {
		return err
	}
	if reply.Phase != summary.Phase+"-expected" || reply.PID != summary.PID || len(reply.Workers) != len(summary.Workers) {
		return errors.New("crawler expectation receipt identity mismatch")
	}
	for index := range summary.Workers {
		if reply.Workers[index].Worker != summary.Workers[index].Worker {
			return errors.New("crawler expectation worker mismatch")
		}
		summary.Workers[index].Expected = reply.Workers[index].Expected
	}
	return nil
}

func crawlerStats(ctx context.Context, client *http.Client, base, namespace string) (json.RawMessage, error) {
	request := crawlerHTTPRequest{Method: http.MethodGet, URL: base + "/" + crawlerIndex(namespace, true) + "," + crawlerIndex(namespace, false) + "/_stats/indexing,get,search,refresh,flush,merge,translog?level=indices"}
	raw, err := crawlerHTTP(ctx, client, request)
	return raw, err
}

type crawlerVerifyOptions struct {
	URL, Namespace string
	Expected       map[string]map[string]any
}

func verifyCrawler(ctx context.Context, client *http.Client, opts crawlerVerifyOptions) error {
	ids := make([]string, 0, len(opts.Expected))
	for id := range opts.Expected {
		ids = append(ids, id)
	}
	for offset := 0; offset < len(ids); offset += 100 {
		part := ids[offset:min(offset+100, len(ids))]
		idsQuery := map[string]any{"values": part}
		query := map[string]any{"ids": idsQuery}
		payload := map[string]any{"query": query, "size": len(part), "track_total_hits": true}
		body, _ := json.Marshal(payload)
		request := crawlerHTTPRequest{Method: http.MethodPost, URL: opts.URL + "/" + crawlerIndex(opts.Namespace, true) + "," + crawlerIndex(opts.Namespace, false) + "/_search", Body: body}
		raw, err := crawlerHTTP(ctx, client, request)
		if err != nil {
			return err
		}
		var reply struct {
			Hits struct {
				Total struct {
					Value int `json:"value"`
				} `json:"total"`
				Hits []struct {
					ID     string         `json:"_id"`
					Index  string         `json:"_index"`
					Source map[string]any `json:"_source"`
				} `json:"hits"`
			} `json:"hits"`
		}
		if err := json.Unmarshal(raw, &reply); err != nil {
			return err
		}
		if reply.Hits.Total.Value != len(part) || len(reply.Hits.Hits) != len(part) {
			return errors.New("crawler normal-query visibility count mismatch")
		}
		for _, hit := range reply.Hits.Hits {
			if hit.Index != crawlerIndex(opts.Namespace, strings.HasSuffix(hit.ID, "-product")) || !reflect.DeepEqual(hit.Source, opts.Expected[hit.ID]) {
				actual, _ := json.Marshal(hit.Source)
				want, _ := json.Marshal(opts.Expected[hit.ID])
				return fmt.Errorf("crawler full-field mismatch %s actual=%.5000s expected=%.5000s", hit.ID, actual, want)
			}
		}
	}
	return nil
}
