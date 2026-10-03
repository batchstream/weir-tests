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
	"strings"
	"syscall"
	"time"

	"github.com/batchstream/weir-tests/internal/benchmark"
	"github.com/batchstream/weir-tests/internal/fixture"
	"github.com/batchstream/weir-tests/internal/workload"
)

type labOptions struct {
	binary       string
	mongoBinary  string
	output       string
	backend      string
	operations   int
	warmup       int
	records      int
	payload      int
	concurrency  int
	rounds       int
	writePercent int
	batchCollect time.Duration
}

func main() {
	var opts labOptions
	flag.StringVar(&opts.binary, "weir", ".tools/weir", "locked Weir executable from scripts/prepare_weir.py")
	flag.StringVar(&opts.mongoBinary, "mongod", os.Getenv("WEIR_TEST_MONGODB_BINARY"), "optional locked native MongoDB executable; empty uses Docker")
	flag.StringVar(&opts.output, "output", "results/local/benchmark", "new report directory")
	flag.StringVar(&opts.backend, "backend", "all", "mongo, search or all")
	flag.IntVar(&opts.operations, "operations", 10000, "logical operations per path per round")
	flag.IntVar(&opts.warmup, "warmup", 1000, "untimed operations per path")
	flag.IntVar(&opts.records, "records", 1024, "fixed working-set document count")
	flag.IntVar(&opts.payload, "payload-bytes", 1024, "deterministic document payload size")
	flag.IntVar(&opts.concurrency, "concurrency", 8, "matched client workers and Weir Store concurrency")
	flag.IntVar(&opts.rounds, "rounds", 3, "alternating paired rounds")
	flag.IntVar(&opts.writePercent, "write-percent", 10, "write fraction from 0 through 100")
	flag.DurationVar(&opts.batchCollect, "batch-collect", 5*time.Millisecond, "explicit Weir microbatch collection interval")
	flag.Parse()
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	if err := runLab(ctx, opts); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func runLab(ctx context.Context, opts labOptions) (resultErr error) {
	if opts.operations < 1 || opts.operations > 10000000 || opts.warmup < 0 || opts.warmup > 10000000 || opts.rounds < 1 || opts.rounds > 100 || opts.writePercent < 0 || opts.writePercent > 100 || opts.concurrency < 1 || opts.concurrency > 32 || opts.records < opts.concurrency || opts.payload < 1 || opts.batchCollect < 0 || opts.batchCollect > 10*time.Millisecond {
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
		"weir_store_concurrency":       fmt.Sprint(opts.concurrency),
		"weir_max_batch_operations":    "32",
		"weir_batch_collect":           opts.batchCollect.String(),
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
	start := fixture.Options{WeirBinary: binary, MongoBinary: opts.mongoBinary, Backends: backends, OwnerCount: 1, StoreConcurrency: opts.concurrency, BatchSize: 32, BatchCollect: opts.batchCollect}
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
		dataset, err := workload.New(config)
		if err != nil {
			return err
		}
		paths, err := workload.Open(ctx, dataset)
		if err != nil {
			return err
		}
		benchOptions := benchmark.Options{Dataset: dataset, Paths: paths, Operations: opts.operations, WarmupOperations: opts.warmup, WritePercent: opts.writePercent, Rounds: opts.rounds, OperationTimeout: 10 * time.Second, Provenance: provenance}
		caseErr := runCase(ctx, paths, benchOptions, opts.output)
		if caseErr != nil {
			return caseErr
		}
	}
	return nil
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
