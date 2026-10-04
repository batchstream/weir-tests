// Command weir-lab runs throughput comparisons against exclusively owned local fixtures.
package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/batchstream/weir-tests/internal/benchmark"
	"github.com/batchstream/weir-tests/internal/fixture"
	"github.com/batchstream/weir-tests/internal/workload"
)

// Both paths use the same complete business-request budget.
const businessTimeout = 10 * time.Second

type labOptions struct {
	binary            string
	mongoBinary       string
	output            string
	backend           string
	operations        int
	warmup            int
	records           int
	payload           int
	concurrency       int
	rounds            int
	writePercent      int
	mode              string
	levels            string
	batches           string
	warmupDuration    time.Duration
	duration          time.Duration
	databaseCPUs      float64
	storeConcurrency  int
	workingMemoryMiB  int
	clientProcesses   int
	backendBatchLimit int
	luaMutations      bool
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
	var opts labOptions
	flag.StringVar(&opts.binary, "weir", ".tools/weir", "locked Weir executable from scripts/prepare_weir.py")
	flag.StringVar(&opts.mongoBinary, "mongod", os.Getenv("WEIR_TEST_MONGODB_BINARY"), "optional locked native MongoDB executable; empty uses Docker")
	flag.StringVar(&opts.output, "output", "results/local/benchmark", "new report directory")
	flag.StringVar(&opts.backend, "backend", "all", "mongo, search or all")
	flag.IntVar(&opts.operations, "operations", 10000, "logical operations per path per round")
	flag.IntVar(&opts.warmup, "warmup", 1000, "untimed operations per path")
	flag.IntVar(&opts.records, "records", 2048, "fixed working-set document count")
	flag.IntVar(&opts.payload, "payload-bytes", 1024, "deterministic document payload size")
	flag.IntVar(&opts.concurrency, "concurrency", 8, "matched client workers and Weir Store concurrency")
	flag.IntVar(&opts.rounds, "rounds", 3, "alternating paired rounds")
	flag.IntVar(&opts.writePercent, "write-percent", 10, "write fraction from 0 through 100")
	flag.BoolVar(&opts.luaMutations, "lua-mutations", false, "single-record read-modify-write: native transaction/OCC versus Weir Lua; no client batching")
	flag.StringVar(&opts.mode, "mode", "saturation", "saturation sends independent single requests from OS client processes; bulk-saturation measures bulk overhead; fixed is a finite diagnostic")
	flag.StringVar(&opts.levels, "concurrency-levels", "8,32,128", "ascending total client concurrency, at most 512 in single-request mode")
	flag.StringVar(&opts.batches, "batch-sizes", "1", "records per call: saturation requires 1; bulk-saturation accepts 1 through 64")
	flag.IntVar(&opts.clientProcesses, "client-processes", 4, "independent client OS processes; each level is evenly divided into concurrent workers and its own connection pool")
	flag.IntVar(&opts.backendBatchLimit, "backend-batch-limit", 32, "server adapter aggregation limit, independent of one-record client requests; 1 disables cross-request aggregation")
	flag.DurationVar(&opts.warmupDuration, "warmup-duration", 10*time.Second, "untimed warmup per saturation stage")
	flag.DurationVar(&opts.duration, "duration", 20*time.Second, "timed duration per saturation stage, including completion joins")
	flag.Float64Var(&opts.databaseCPUs, "database-cpus", 1, "enforced CPU quota for each owned Docker database; native Mongo uses all host CPUs")
	flag.IntVar(&opts.storeConcurrency, "store-concurrency", 32, "fixed Weir backend concurrency in saturation mode, independently of client workers")
	flag.IntVar(&opts.workingMemoryMiB, "store-working-memory-mib", 0, "backend working memory override; zero sizes each backend for the configured concurrency")
	flag.Parse()
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	if err := runLab(ctx, opts); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func runLab(ctx context.Context, opts labOptions) (resultErr error) {
	if opts.mode != "fixed" && opts.mode != "saturation" && opts.mode != "bulk-saturation" {
		return errors.New("mode must be fixed, saturation or bulk-saturation")
	}
	if opts.luaMutations && (opts.mode == "bulk-saturation" || opts.writePercent == 0) {
		return errors.New("Lua mutation comparison requires single-record calls and a positive write percentage")
	}
	// lua.v1 values are bounded to 256 KiB. Reserve 128 bytes for the fixed
	// fixture fields so an oversized comparison fails before services start.
	if opts.luaMutations && opts.payload > (256<<10)-128 {
		return errors.New("Lua fixture padding plus its fields exceeds the 256 KiB value budget")
	}
	levels, batches := []int{opts.concurrency}, []int{1}
	if opts.mode != "fixed" {
		var err error
		levels, err = parsePositiveList(opts.levels)
		if err != nil {
			return err
		}
		batches, err = parsePositiveList(opts.batches)
		if err != nil {
			return err
		}
		if len(levels) < 2 || len(levels) > 12 || len(batches) > 8 || opts.duration < 5*time.Second || opts.duration > 10*time.Minute || opts.warmupDuration < time.Second || opts.warmupDuration > time.Minute || opts.storeConcurrency < 1 || opts.storeConcurrency > 32 {
			return errors.New("invalid saturation matrix or durations")
		}
		previous := 0
		maxLevel := 64
		if opts.mode == "saturation" {
			maxLevel = 512
		}
		for _, workers := range levels {
			if workers <= previous || workers > maxLevel {
				return fmt.Errorf("owned single-owner concurrency levels must strictly increase through %d", maxLevel)
			}
			previous = workers
		}
		if opts.mode == "saturation" {
			if opts.clientProcesses < 1 || opts.clientProcesses > 32 || len(batches) != 1 || batches[0] != 1 {
				return errors.New("single-request saturation requires 1 through 32 client processes and batch-sizes 1")
			}
			for _, level := range levels {
				if level%opts.clientProcesses != 0 {
					return errors.New("concurrency levels must divide evenly into client processes")
				}
			}
		}
		for _, batch := range batches {
			if batch > 64 || opts.records < previous*batch {
				return errors.New("batch <=64 and records >= max-workers * batch-size required")
			}
		}
		opts.concurrency = levels[len(levels)-1]
	}
	if opts.databaseCPUs <= 0 || opts.databaseCPUs > float64(runtime.NumCPU()) {
		return errors.New("database CPU quota exceeds host budget")
	}
	if opts.backendBatchLimit < 1 || opts.backendBatchLimit > 1024 {
		return errors.New("backend-batch-limit must be 1 through 1024")
	}
	maxWorkers := 32
	if opts.mode != "fixed" {
		maxWorkers = 64
		if opts.mode == "saturation" {
			maxWorkers = 512
		}
	}
	if opts.operations < 1 || opts.operations > 10000000 || opts.warmup < 0 || opts.warmup > 10000000 || opts.rounds < 1 || opts.rounds > 100 || opts.writePercent < 0 || opts.writePercent > 100 || opts.concurrency < 1 || opts.concurrency > maxWorkers || opts.records < opts.concurrency || opts.payload < 1 {
		return errors.New("invalid bounded workload parameters")
	}
	backends := []string{opts.backend}
	if opts.backend == "all" {
		backends = []string{"mongo", "search"}
	} else if opts.backend != "mongo" && opts.backend != "search" {
		return errors.New("backend must be mongo, search or all")
	}
	binary, err := filepath.Abs(opts.binary)
	if err != nil {
		return err
	}
	data, err := os.ReadFile(binary)
	if err != nil {
		return fmt.Errorf("prepare the locked Weir executable first: %w", err)
	}
	digest := sha256.Sum256(data)
	provenance := map[string]string{
		"weir_binary_sha256":           hex.EncodeToString(digest[:]),
		"topology":                     "native client and Weir on one host; dedicated database Docker containers with loopback ports",
		"client_operation_timeout":     businessTimeout.String(),
		"weir_backend_timeout":         businessTimeout.String(),
		"weir_store_concurrency":       fmt.Sprint(opts.concurrency),
		"weir_max_batch_operations":    fmt.Sprint(opts.backendBatchLimit),
		"weir_memory_budget":           "8GiB (declared process admission budget; not an OS reservation)",
		"weir_ingress_max_sessions":    "64",
		"weir_ingress_max_connections": "64",
		"host_cpus":                    fmt.Sprint(runtime.NumCPU()),
	}
	identityRaw, err := exec.CommandContext(ctx, binary, "version").Output()
	if err != nil {
		return fmt.Errorf("read Weir identity: %w", err)
	}
	var identity struct {
		Revision string `json:"revision"`
	}
	if err := json.Unmarshal(identityRaw, &identity); err != nil {
		return err
	}
	provenance["weir_source"] = identity.Revision
	if opts.mongoBinary != "" {
		provenance["topology"] = "native client, Weir and isolated MongoDB on one host; Elasticsearch in a dedicated loopback Docker container"
	}
	if pinsRaw, err := os.ReadFile("versions.json"); err == nil {
		var pins map[string]string
		if err := json.Unmarshal(pinsRaw, &pins); err != nil {
			return err
		}
		if identity.Revision != pins["weir_source"] {
			return errors.New("Weir binary does not match versions.json")
		}
		for _, key := range []string{"sdk", "protocol", "mongodb_image", "elasticsearch_image"} {
			provenance[key] = pins[key]
		}
	} else {
		return errors.New("run the lab from the repository root with its versions.json lock")
	}
	if revision, err := exec.CommandContext(ctx, "git", "rev-parse", "HEAD").Output(); err == nil {
		provenance["test_source"] = strings.TrimSpace(string(revision))
	}
	if status, err := exec.CommandContext(ctx, "git", "status", "--porcelain").Output(); err == nil {
		provenance["test_source_dirty"] = fmt.Sprint(len(status) > 0)
	}
	if err := os.MkdirAll(filepath.Dir(opts.output), 0o755); err != nil {
		return err
	}
	if err := os.Mkdir(opts.output, 0o755); err != nil {
		return fmt.Errorf("requires a new output directory; preserving existing reports: %w", err)
	}
	storeWorkers := opts.concurrency
	if opts.mode != "fixed" {
		storeWorkers = opts.storeConcurrency
	}
	provenance["weir_store_concurrency"] = fmt.Sprint(storeWorkers)
	provenance["weir_ingress_max_sessions"] = fmt.Sprint(max(64, opts.concurrency))
	provenance["database_docker_cpu_quota"] = fmt.Sprint(opts.databaseCPUs)
	workspace, memoryMiB, err := fixtureMemoryBudget(opts, backends, storeWorkers)
	if err != nil {
		return err
	}
	for backend, memory := range workspace {
		provenance["weir_"+backend+"_working_memory_mib"] = fmt.Sprint(memory)
	}
	provenance["weir_memory_budget"] = fmt.Sprintf("%dMiB (declared process admission budget; not an OS reservation)", memoryMiB)
	start := fixture.Options{WeirBinary: binary, MongoBinary: opts.mongoBinary, Backends: backends, OwnerCount: 1, StoreConcurrency: storeWorkers, BatchSize: opts.backendBatchLimit, BackendTimeout: businessTimeout, IngressSessions: max(64, opts.concurrency), DatabaseCPUs: opts.databaseCPUs, ProcessMemoryMiB: memoryMiB, WorkingMemoryMiB: workspace}
	cluster, err := fixture.Start(ctx, start)
	if err != nil {
		return err
	}
	defer func() {
		cleanup, cancel := context.WithTimeout(context.Background(), 45*time.Second)
		defer cancel()
		resultErr = errors.Join(resultErr, cluster.Close(cleanup))
	}()
	provenance["fixture_logs"] = cluster.Directory
	fmt.Println("Owned fixture logs:", cluster.Directory)
	for _, backend := range backends {
		namespace, err := workload.NewNamespace()
		if err != nil {
			return err
		}
		config := workload.Config{Backend: backend, MongoURI: cluster.MongoURI, SearchURL: cluster.SearchURL, WeirSeed: cluster.Seed(), StoreName: backend, Namespace: namespace, Records: opts.records, PayloadBytes: opts.payload, Concurrency: opts.concurrency}
		config.LuaMutations = opts.luaMutations
		dataset, err := workload.New(config)
		if err != nil {
			return err
		}
		paths, err := workload.Open(ctx, dataset)
		if err != nil {
			return err
		}
		benchOptions := benchmark.Options{Dataset: dataset, Paths: paths, Operations: opts.operations, WarmupOperations: opts.warmup, WritePercent: opts.writePercent, Rounds: opts.rounds, OperationTimeout: businessTimeout, Provenance: provenance}
		var caseErr error
		if opts.mode != "fixed" {
			saturation := benchmark.SaturationOptions{Dataset: dataset, Paths: paths, Concurrency: levels, BatchSizes: batches, Warmup: opts.warmupDuration, Duration: opts.duration, Rounds: opts.rounds, WritePercent: opts.writePercent, OperationTimeout: businessTimeout, Resources: cluster.Resources(backend), CPUThreshold: 90, Provenance: provenance}
			if opts.mode == "saturation" {
				executable, err := os.Executable()
				if err != nil {
					return err
				}
				saturation.ClientExecutable = executable
				saturation.ClientProcesses = opts.clientProcesses
			}
			caseErr = runSaturationCase(ctx, saturation, opts.output)
		} else {
			caseErr = runCase(ctx, paths, benchOptions, opts.output)
		}
		if caseErr != nil {
			return caseErr
		}
	}
	return nil
}

func fixtureMemoryBudget(opts labOptions, backends []string, workers int) (map[string]int, int, error) {
	if workers < 1 || workers > 65536/2 || opts.workingMemoryMiB < 0 || opts.workingMemoryMiB > 65536 {
		return nil, 0, errors.New("invalid backend memory parameters")
	}
	workspace := make(map[string]int, len(backends))
	// The pinned adapters bound one 2MiB-read batch by at most 41MiB for
	// MongoDB (native reply, guard and scratch) and 96MiB for Search.
	// Add ingress envelopes and 1GiB for Store input/results and other framing.
	memoryMiB := max(64, opts.concurrency)*96 + 1024
	for _, backend := range backends {
		perBatchMiB := 41
		if opts.luaMutations {
			perBatchMiB = 64
		}
		if backend == "search" {
			perBatchMiB = 96
		}
		memory := opts.workingMemoryMiB
		if memory == 0 {
			if workers > 65536/perBatchMiB {
				return nil, 0, errors.New("backend concurrency cannot fit the process memory envelope")
			}
			memory = max(384, workers*perBatchMiB)
		}
		workspace[backend] = memory
		memoryMiB += memory + workers*2
	}
	memoryMiB = (memoryMiB + 1023) / 1024 * 1024
	if memoryMiB > 65536 {
		return nil, 0, errors.New("combined ingress and backend memory cannot fit the process envelope")
	}
	return workspace, memoryMiB, nil
}

func parsePositiveList(raw string) ([]int, error) {
	values := strings.Split(raw, ",")
	result := make([]int, 0, len(values))
	seen := make(map[int]bool, len(values))
	for _, rawValue := range values {
		value, err := strconv.Atoi(strings.TrimSpace(rawValue))
		if err != nil || value < 1 || seen[value] {
			return nil, errors.New("requires a comma-separated list of distinct positive integers")
		}
		seen[value] = true
		result = append(result, value)
	}
	return result, nil
}

func runSaturationCase(ctx context.Context, opts benchmark.SaturationOptions, output string) (resultErr error) {
	defer func() {
		cleanup, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		resultErr = errors.Join(resultErr, opts.Paths.Cleanup(cleanup), opts.Paths.Close())
	}()
	report, err := benchmark.RunSaturation(ctx, opts)
	if report != nil {
		name := opts.Dataset.Config.Backend
		err = errors.Join(err, report.Write(filepath.Join(output, name+".json"), filepath.Join(output, name+".md")))
		fmt.Print(report.Markdown())
		if !report.Verified() {
			err = errors.Join(err, errors.New("saturation matrix workload failed or was incomplete"))
		}
	}
	return err
}

func runCase(ctx context.Context, paths *workload.Paths, opts benchmark.Options, output string) (resultErr error) {
	defer func() {
		cleanup, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		resultErr = errors.Join(resultErr, paths.Cleanup(cleanup), paths.Close())
	}()
	report, err := benchmark.Run(ctx, opts)
	if report != nil {
		name := opts.Dataset.Config.Backend
		writeErr := report.Write(filepath.Join(output, name+".json"), filepath.Join(output, name+".md"))
		err = errors.Join(err, writeErr)
		if report.Successful() {
			fmt.Printf("%s direct=%.1f ops/s weir=%.1f ops/s ratio=%.3f\n", name, report.Aggregate.DirectOpsPerSec, report.Aggregate.WeirOpsPerSec, *report.Aggregate.WeirDirectRatio)
		} else {
			err = errors.Join(err, errors.New(name+" comparison is incomplete or unqualified; no throughput conclusion"))
		}
	}
	return err
}
