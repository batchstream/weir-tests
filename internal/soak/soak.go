package soak

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	mathrand "math/rand/v2"
	"os"
	"strings"
	"sync"
	"time"

	weir "github.com/batchstream/weir-go"
)

// Backend selects one database, the Weir Store connected to it and the load
// it receives.
type Backend struct {
	Kind          string  `json:"kind"`
	Store         string  `json:"store"`
	URI           string  `json:"-"`
	Workers       int     `json:"workers"`
	KeysPerWorker int     `json:"keys_per_worker"`
	Rate          float64 `json:"operations_per_second"`
}

// Config describes one soak run.
type Config struct {
	WeirAddress      string        `json:"weir_address"`
	WeirDiagnostics  string        `json:"weir_diagnostics"`
	Backends         []Backend     `json:"backends"`
	Duration         time.Duration `json:"duration"`
	SampleInterval   time.Duration `json:"sample_interval"`
	OperationTimeout time.Duration `json:"operation_timeout"`
	Settle           time.Duration `json:"settle"`
	ReadPercent      int           `json:"read_percent"`
	VerifyPercent    int           `json:"verify_percent"`
	PaddingBytes     int           `json:"padding_bytes"`
	Output           string        `json:"-"`
	KeepData         bool          `json:"keep_data"`
}

func (c Config) validate() error {
	if c.WeirAddress == "" || c.WeirDiagnostics == "" || c.Output == "" || len(c.Backends) == 0 {
		return errors.New("soak needs a Weir address, its diagnostics address, an output prefix and at least one backend")
	}
	if c.Duration <= 0 || c.SampleInterval <= 0 || c.OperationTimeout <= 0 || c.Settle < 0 {
		return errors.New("soak durations must be positive")
	}
	if c.ReadPercent < 0 || c.ReadPercent > 100 || c.VerifyPercent < 0 || c.VerifyPercent > 100 || c.PaddingBytes < 0 {
		return errors.New("soak percentages must lie in 0..100 and padding must not be negative")
	}
	for _, backend := range c.Backends {
		if backend.Kind != "mongo" && backend.Kind != "search" || backend.Store == "" || backend.URI == "" {
			return errors.New("each backend needs kind mongo or search, a Store and a URI")
		}
		if backend.Workers < 1 || backend.KeysPerWorker < 1 || backend.Rate <= 0 {
			return fmt.Errorf("%s needs positive workers, keys per worker and rate", backend.Kind)
		}
	}
	return nil
}

// FinalCheck is the end-of-run comparison of every key with the database.
type FinalCheck struct {
	Keys       int    `json:"keys"`
	Present    int    `json:"present"`
	Unknown    int    `json:"keys_with_unknown_writes"`
	Violations int    `json:"violations"`
	Error      string `json:"error,omitempty"`
}

// BackendReport summarizes one backend over the whole run.
type BackendReport struct {
	Namespace string     `json:"namespace"`
	Totals    Counters   `json:"totals"`
	Final     FinalCheck `json:"final"`
}

// Report is the soak outcome; Pass requires no violations, a complete final
// check of every backend and APPLIED writes on each.
type Report struct {
	Started    string                    `json:"started_utc"`
	Finished   string                    `json:"finished_utc"`
	Config     Config                    `json:"config"`
	Backends   map[string]*BackendReport `json:"backends"`
	Violations int                       `json:"violations"`
	Kept       []Violation               `json:"violation_details,omitempty"`
	Pass       bool                      `json:"pass"`
}

// lateWindow, in operation timeouts, bounds how long after being issued a
// write whose outcome was unknown may still land: the caller's deadline ends
// it in Weir's queue and the backend's own timeout ends it in flight.
const lateWindow = 4

type backendRun struct {
	config    Backend
	namespace string
	store     store
	stats     *stats
	keys      []*keyState
}

// Run drives the workload until the duration ends or ctx is canceled, then
// checks every key and writes <Output>.json, with per-interval samples in
// <Output>.jsonl.
func Run(ctx context.Context, config Config) (resultErr error) {
	if err := config.validate(); err != nil {
		return err
	}
	timeline, err := os.Create(config.Output + ".jsonl")
	if err != nil {
		return err
	}
	defer func() { resultErr = errors.Join(resultErr, timeline.Close()) }()
	runs, err := openBackends(ctx, config)
	defer func() { resultErr = errors.Join(resultErr, closeBackends(runs, config.KeepData)) }()
	if err != nil {
		return err
	}
	client, err := weir.New(config.WeirAddress)
	if err != nil {
		return err
	}
	defer func() { resultErr = errors.Join(resultErr, client.Close()) }()
	report := &Report{Started: time.Now().UTC().Format(time.RFC3339), Config: config, Backends: make(map[string]*BackendReport)}
	violations := &violationLog{}
	stop := make(chan struct{})
	var workers sync.WaitGroup
	padding := strings.Repeat("p", config.PaddingBytes)
	for _, run := range runs {
		tokens := make(chan struct{}, run.config.Workers)
		go pace(stop, run.config.Rate, tokens)
		for index := range run.config.Workers {
			seed := mathrand.Uint64()
			load := &worker{
				backend:       run.config.Kind,
				storeName:     run.config.Store,
				store:         run.store,
				client:        client,
				keys:          run.keys[index*run.config.KeysPerWorker : (index+1)*run.config.KeysPerWorker],
				padding:       padding,
				readPercent:   config.ReadPercent,
				verifyPercent: config.VerifyPercent,
				timeout:       config.OperationTimeout,
				lateWindow:    lateWindow * config.OperationTimeout,
				random:        mathrand.New(mathrand.NewPCG(seed, uint64(index))),
				stats:         run.stats,
				violations:    violations,
				stop:          stop,
			}
			workers.Go(func() { load.run(tokens) })
		}
	}
	sampling := sampler{config: config, runs: runs, violations: violations, output: timeline}
	sampled := make(chan struct{})
	go func() {
		defer close(sampled)
		sampling.run(ctx, stop)
	}()
	deadline := time.NewTimer(config.Duration)
	select {
	case <-ctx.Done():
	case <-deadline.C:
	}
	deadline.Stop()
	close(stop)
	workers.Wait()
	<-sampled
	// Let writes whose outcome is unknown finish before the final check.
	time.Sleep(config.Settle)
	for _, run := range runs {
		backend := &BackendReport{Namespace: run.namespace, Totals: run.stats.total()}
		backend.Final = finalCheck(run, violations)
		report.Backends[run.config.Kind] = backend
	}
	report.Finished = time.Now().UTC().Format(time.RFC3339)
	report.Violations, report.Kept = violations.snapshot()
	report.Pass = report.Violations == 0
	for _, backend := range report.Backends {
		report.Pass = report.Pass && backend.Final.Error == "" && backend.Totals.Applied > 0
	}
	if err := writeReport(config.Output+".json", report); err != nil {
		return err
	}
	if !report.Pass {
		return fmt.Errorf("soak failed: %d violations, or a backend without APPLIED writes or a final check; see %s.json", report.Violations, config.Output)
	}
	return nil
}

func openBackends(ctx context.Context, config Config) ([]*backendRun, error) {
	suffix := make([]byte, 4)
	if _, err := rand.Read(suffix); err != nil {
		return nil, err
	}
	namespace := "soak_" + hex.EncodeToString(suffix)
	var runs []*backendRun
	for _, backend := range config.Backends {
		var opened store
		var err error
		if backend.Kind == "mongo" {
			opened, err = openMongo(ctx, backend.URI, namespace)
		} else {
			opened, err = openSearch(ctx, backend.URI, namespace)
		}
		if err != nil {
			return runs, err
		}
		run := &backendRun{config: backend, namespace: namespace, store: opened, stats: &stats{}}
		if err := opened.create(ctx); err != nil {
			return runs, errors.Join(fmt.Errorf("create %s namespace %s: %w", backend.Kind, namespace, err), opened.close(ctx))
		}
		runs = append(runs, run)
		total := backend.Workers * backend.KeysPerWorker
		run.keys = make([]*keyState, total)
		for index := range total {
			key := &keyState{id: fmt.Sprintf("k-%07d", index)}
			run.keys[index] = key
		}
	}
	return runs, nil
}

func closeBackends(runs []*backendRun, keep bool) error {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	var errs []error
	for _, run := range runs {
		if !keep {
			errs = append(errs, run.store.drop(ctx))
		}
		errs = append(errs, run.store.close(ctx))
	}
	return errors.Join(errs...)
}

func finalCheck(run *backendRun, violations *violationLog) FinalCheck {
	check := FinalCheck{Keys: len(run.keys)}
	ids := make([]string, len(run.keys))
	written := make(map[string]bool, len(run.keys))
	for index, key := range run.keys {
		ids[index] = key.id
		written[key.id] = true
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	found, err := run.store.readAll(ctx, ids)
	if err != nil {
		check.Error = err.Error()
		return check
	}
	for _, key := range run.keys {
		observed := found[key.id]
		if observed != 0 {
			check.Present++
		}
		if len(key.unknown) > 0 {
			check.Unknown++
		}
		if !key.allows(observed) {
			check.Violations++
			violations.add(run.config.Kind, "final", key, observed)
		}
	}
	for id := range found {
		if !written[id] {
			check.Error = "the namespace holds a key no worker wrote: " + id
		}
	}
	return check
}

func writeReport(path string, report *Report) error {
	raw, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(raw, '\n'), 0o644)
}
