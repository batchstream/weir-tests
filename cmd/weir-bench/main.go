// weir-bench runs the matched workload against explicitly supplied services.
// weir-lab owns local fixtures; this command never starts or stops services.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"net"
	"net/netip"
	"net/url"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/batchstream/weir-tests/internal/benchmark"
	"github.com/batchstream/weir-tests/internal/fixture"
	"github.com/batchstream/weir-tests/internal/workload"
)

type settings struct {
	mode             string
	backend          string
	mongoURI         string
	searchURL        string
	weirAddress      string
	weirDiagnostics  string
	store            string
	operations       int
	warmup           int
	records          int
	padding          int
	workers          int
	writes           int
	rounds           int
	levels           string
	clientProcesses  int
	databaseCPUs     float64
	warmupDuration   time.Duration
	duration         time.Duration
	timeout          time.Duration
	operationTimeout time.Duration
	output           string
	allowRemote      bool
	keepData         bool
}

func main() {
	if len(os.Args) == 2 && os.Args[1] == "client-worker" {
		ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
		defer cancel()
		if err := benchmark.RunClientWorker(ctx, os.Stdin, os.Stdout); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		return
	}
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run() (resultErr error) {
	var input settings
	flag.StringVar(&input.mode, "mode", "fixed", "fixed runs in-process workers; saturation runs independent client processes over a concurrency ladder")
	flag.StringVar(&input.backend, "backend", "mongo", "mongo or search")
	flag.StringVar(&input.mongoURI, "mongo-uri", "", "explicit credential-free Mongo URI")
	flag.StringVar(&input.searchURL, "search-url", "", "explicit credential-free Search root URL")
	flag.StringVar(&input.weirAddress, "weir-address", "", "explicit Weir application IP:port")
	flag.StringVar(&input.weirDiagnostics, "weir-diagnostics", "", "explicit Weir diagnostics IP:port; required in saturation mode")
	flag.StringVar(&input.store, "store", "", "Weir Store connected to the same database")
	flag.IntVar(&input.operations, "operations", 10000, "fixed mode: total logical operations per path per round")
	flag.IntVar(&input.warmup, "warmup", 1000, "fixed mode: unmeasured warmup operations per path")
	flag.IntVar(&input.records, "records", 1000, "pre-created documents; at least the highest concurrency")
	flag.IntVar(&input.padding, "padding-bytes", 512, "document padding bytes; metadata adds BSON/JSON overhead")
	flag.IntVar(&input.workers, "concurrency", 8, "fixed mode: sequential workers per path")
	flag.IntVar(&input.writes, "write-percent", 10, "deterministic write probability; actual mix reported")
	flag.IntVar(&input.rounds, "rounds", 3, "paired alternating AB/BA rounds")
	flag.StringVar(&input.levels, "concurrency-levels", "64,256,1024", "saturation mode: ascending total client workers, at most 2048")
	flag.IntVar(&input.clientProcesses, "client-processes", 4, "saturation mode: independent client OS processes, each with its own pool")
	flag.Float64Var(&input.databaseCPUs, "database-cpus", 0, "saturation mode: CPU limit of the database, the denominator of its CPU utilization")
	flag.DurationVar(&input.warmupDuration, "warmup-duration", 10*time.Second, "saturation mode: untimed warmup per stage")
	flag.DurationVar(&input.duration, "duration", 20*time.Second, "saturation mode: timed duration per stage")
	flag.DurationVar(&input.timeout, "timeout", 0, "whole run timeout, including setup and postflight; 0 derives it from the workload")
	flag.DurationVar(&input.operationTimeout, "operation-timeout", 10*time.Second, "per-operation timeout without replay")
	flag.StringVar(&input.output, "output", "", "report path prefix; creates .json and .md")
	flag.BoolVar(&input.allowRemote, "allow-remote", false, "explicitly permit non-loopback IP addresses")
	flag.BoolVar(&input.keepData, "keep-data", false, "retain only this run's newly created namespace")
	flag.Parse()
	if err := validate(input); err != nil {
		return err
	}
	levels := []int{input.workers}
	if input.mode == "saturation" {
		levels, _ = parseLevels(input.levels)
	}
	namespace, err := workload.NewNamespace()
	if err != nil {
		return err
	}
	config := workload.Config{AllowRemote: input.allowRemote, Backend: input.backend, MongoURI: input.mongoURI, SearchURL: input.searchURL, WeirAddress: input.weirAddress, StoreName: input.store, Namespace: namespace, Records: input.records, PayloadBytes: input.padding, Concurrency: levels[len(levels)-1]}
	dataset, err := workload.New(config)
	if err != nil {
		return err
	}
	base, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	ctx, cancel := context.WithTimeout(base, runTimeout(input, len(levels)))
	defer cancel()
	paths, err := workload.Open(ctx, dataset)
	if err != nil {
		return err
	}
	defer func() { resultErr = errors.Join(resultErr, paths.Close()) }()
	defer func() {
		if input.keepData {
			return
		}
		cleanup, cleanupCancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cleanupCancel()
		if err := paths.Cleanup(cleanup); err != nil {
			resultErr = errors.Join(resultErr, fmt.Errorf("owned namespace cleanup: %w", err))
		}
	}()
	provenance := map[string]string{"fixture": "externally supplied services; freshly created workload-owned namespace", "weir_server_configuration": "not supplied; record effective server concurrency, batching and resource limits before interpreting results"}
	output := input.output
	if output == "" {
		output = filepath.Join(".testdata", "bench-"+input.mode+"-"+input.backend+"-"+namespace)
	}
	if input.mode == "saturation" {
		saturation := saturationRun{input: input, dataset: dataset, paths: paths, levels: levels, provenance: provenance, output: output}
		return runSaturation(ctx, saturation)
	}
	opts := benchmark.Options{Dataset: dataset, Paths: paths, Operations: input.operations, WarmupOperations: input.warmup, WritePercent: input.writes, Rounds: input.rounds, OperationTimeout: input.operationTimeout, Provenance: provenance}
	report, runErr := benchmark.Run(ctx, opts)
	if report == nil {
		return runErr
	}
	if err := report.Write(output+".json", output+".md"); err != nil {
		return err
	}
	fmt.Print(report.Markdown())
	fmt.Printf("\nReports: %s.json and %s.md\n", output, output)
	if runErr != nil {
		return runErr
	}
	if !report.Successful() {
		return errors.New("benchmark did not produce a fully verified comparison")
	}
	return nil
}

type saturationRun struct {
	input      settings
	dataset    *workload.Dataset
	paths      *workload.Paths
	levels     []int
	provenance map[string]string
	output     string
}

// runSaturation drives the weir-lab single-request matrix against the supplied
// services. Database CPU comes from the database's own API and Weir CPU from
// its diagnostics, so both may run on other hosts.
func runSaturation(ctx context.Context, run saturationRun) error {
	executable, err := os.Executable()
	if err != nil {
		return err
	}
	// Database CPU is the saturation evidence, so stop before the ladder without it.
	counters, err := run.paths.DatabaseCounters(ctx)
	if err != nil {
		return fmt.Errorf("database CPU preflight: %w", err)
	}
	if counters.ProcessCPUNS == nil || counters.ProcessCPUAtNS == nil {
		return errors.New("database CPU preflight: the database reports no process CPU; saturation needs a Linux mongod or a single Elasticsearch node")
	}
	run.provenance["database_cpu_limit"] = strconv.FormatFloat(run.input.databaseCPUs, 'f', -1, 64)
	run.provenance["weir_diagnostics"] = run.input.weirDiagnostics
	resources := fixture.ResourceTarget{AllocatedCPUs: run.input.databaseCPUs, Diagnostics: run.input.weirDiagnostics, Remote: true}
	opts := benchmark.SaturationOptions{Dataset: run.dataset, Paths: run.paths, Concurrency: run.levels, BatchSizes: []int{1}, Warmup: run.input.warmupDuration, Duration: run.input.duration, Rounds: run.input.rounds, WritePercent: run.input.writes, OperationTimeout: run.input.operationTimeout, Resources: resources, CPUThreshold: 90, Provenance: run.provenance, ClientProcesses: run.input.clientProcesses, ClientExecutable: executable}
	report, runErr := benchmark.RunSaturation(ctx, opts)
	if report == nil {
		return runErr
	}
	if err := report.Write(run.output+".json", run.output+".md"); err != nil {
		return errors.Join(runErr, err)
	}
	fmt.Print(report.Markdown())
	fmt.Printf("\nReports: %s.json and %s.md\n", run.output, run.output)
	if runErr != nil {
		return runErr
	}
	if !report.Verified() {
		return errors.New("saturation matrix workload failed or was incomplete")
	}
	return nil
}

// runTimeout bounds the whole run. Saturation derives a generous bound from
// its stages unless -timeout is set.
func runTimeout(input settings, levels int) time.Duration {
	if input.timeout > 0 {
		return input.timeout
	}
	if input.mode != "saturation" {
		return 3 * time.Minute
	}
	stages := time.Duration(levels * input.rounds * 2)
	return stages*(input.warmupDuration+input.duration+time.Minute) + 5*time.Minute
}

func validate(input settings) error {
	if input.backend != "mongo" && input.backend != "search" || input.weirAddress == "" || input.store == "" || input.timeout < 0 {
		return errors.New("requires --backend mongo|search, --weir-address, --store and a non-negative timeout")
	}
	if err := validateAddress(input.weirAddress, input.allowRemote); err != nil {
		return err
	}
	switch input.mode {
	case "fixed":
	case "saturation":
		if err := validateSaturation(input); err != nil {
			return err
		}
	default:
		return errors.New("mode must be fixed or saturation")
	}
	if input.backend == "mongo" {
		return workload.ValidateMongoURI(input.mongoURI, input.allowRemote)
	}
	raw := input.searchURL
	address, err := url.Parse(raw)
	if err != nil || address.User != nil || address.Host == "" || address.Fragment != "" {
		return errors.New("requires an explicit credential-free backend endpoint")
	}
	if (address.Scheme != "http" && address.Scheme != "https") || address.Path != "" && address.Path != "/" || address.RawQuery != "" {
		return errors.New("Search endpoint requires an http(s) root URL")
	}
	return validateAddress(address.Host, input.allowRemote)
}

func validateSaturation(input settings) error {
	diagnostics, err := netip.ParseAddrPort(input.weirDiagnostics)
	if err != nil || diagnostics.Port() == 0 {
		return errors.New("saturation mode requires --weir-diagnostics as an explicit IP:port")
	}
	if err := validateAddress(input.weirDiagnostics, input.allowRemote); err != nil {
		return err
	}
	if !(input.databaseCPUs > 0 && input.databaseCPUs <= 1024) {
		return errors.New("saturation mode requires --database-cpus, the database CPU limit")
	}
	levels, err := parseLevels(input.levels)
	if err != nil {
		return err
	}
	if input.clientProcesses < 1 || input.clientProcesses > 32 {
		return errors.New("saturation mode requires 1 through 32 client processes")
	}
	for _, level := range levels {
		if level%input.clientProcesses != 0 || level/input.clientProcesses > 1024 {
			return errors.New("concurrency levels must divide evenly into at most 1024 workers per client process")
		}
	}
	if input.records < levels[len(levels)-1] || input.writes < 0 || input.writes > 100 || input.rounds < 1 || input.rounds > 10 {
		return errors.New("saturation mode requires records >= highest concurrency, write-percent 0-100 and 1-10 rounds")
	}
	if input.operationTimeout <= 0 || input.operationTimeout > time.Minute {
		return errors.New("saturation mode requires an operation timeout above 0 and at most 1m")
	}
	if input.warmupDuration < time.Second || input.warmupDuration > 5*time.Minute || input.duration < 5*time.Second || input.duration > 10*time.Minute {
		return errors.New("saturation mode requires 1s-5m warmup and 5s-10m measurement per stage")
	}
	return nil
}

func parseLevels(raw string) ([]int, error) {
	fields := strings.Split(raw, ",")
	if len(fields) > 12 {
		return nil, errors.New("at most 12 concurrency levels")
	}
	levels := make([]int, 0, len(fields))
	previous := 0
	for _, field := range fields {
		level, err := strconv.Atoi(strings.TrimSpace(field))
		if err != nil || level <= previous || level > 2048 {
			return nil, errors.New("concurrency levels must strictly increase through 2048")
		}
		levels = append(levels, level)
		previous = level
	}
	return levels, nil
}

func validateAddress(address string, allowRemote bool) error {
	host, port, err := net.SplitHostPort(address)
	if err != nil || port == "" {
		return errors.New("endpoint must specify IP:port")
	}
	if host == "localhost" {
		return nil
	}
	ip, err := netip.ParseAddr(host)
	if err != nil || ip.IsUnspecified() || ip.IsMulticast() || ip.Zone() != "" {
		return errors.New("endpoint must use localhost or a concrete IP address")
	}
	if !allowRemote && !ip.IsLoopback() {
		return errors.New("non-loopback endpoint requires explicit --allow-remote")
	}
	return nil
}
