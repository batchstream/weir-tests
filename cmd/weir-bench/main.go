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
	"strings"
	"syscall"
	"time"

	"github.com/batchstream/weir-tests/internal/benchmark"
	"github.com/batchstream/weir-tests/internal/workload"
)

type settings struct {
	backend          string
	mongoURI         string
	searchURL        string
	weirSeed         string
	store            string
	operations       int
	warmup           int
	records          int
	padding          int
	workers          int
	writes           int
	rounds           int
	timeout          time.Duration
	operationTimeout time.Duration
	output           string
	allowRemote      bool
	keepData         bool
}

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run() (resultErr error) {
	var input settings
	flag.StringVar(&input.backend, "backend", "mongo", "mongo or search")
	flag.StringVar(&input.mongoURI, "mongo-uri", "", "explicit credential-free Mongo URI")
	flag.StringVar(&input.searchURL, "search-url", "", "explicit credential-free Search root URL")
	flag.StringVar(&input.weirSeed, "weir-seed", "", "explicit Weir initialization IP:port")
	flag.StringVar(&input.store, "store", "", "Weir Store connected to the same database")
	flag.IntVar(&input.operations, "operations", 10000, "total fixed logical operations per path per round")
	flag.IntVar(&input.warmup, "warmup", 1000, "unmeasured warmup operations per path")
	flag.IntVar(&input.records, "records", 1000, "pre-created documents")
	flag.IntVar(&input.padding, "padding-bytes", 512, "document padding bytes; metadata adds BSON/JSON overhead")
	flag.IntVar(&input.workers, "concurrency", 8, "sequential workers per path")
	flag.IntVar(&input.writes, "write-percent", 10, "deterministic write probability; actual mix reported")
	flag.IntVar(&input.rounds, "rounds", 3, "paired alternating AB/BA rounds")
	flag.DurationVar(&input.timeout, "timeout", 3*time.Minute, "whole run timeout, including setup and postflight")
	flag.DurationVar(&input.operationTimeout, "operation-timeout", 10*time.Second, "per-operation timeout without replay")
	flag.StringVar(&input.output, "output", "", "report path prefix; creates .json and .md")
	flag.BoolVar(&input.allowRemote, "allow-remote", false, "explicitly permit non-loopback IP addresses")
	flag.BoolVar(&input.keepData, "keep-data", false, "retain only this run's newly created namespace")
	flag.Parse()
	if err := validate(input); err != nil {
		return err
	}
	namespace, err := workload.NewNamespace()
	if err != nil {
		return err
	}
	config := workload.Config{Backend: input.backend, MongoURI: input.mongoURI, SearchURL: input.searchURL, WeirSeed: input.weirSeed, StoreName: input.store, Namespace: namespace, Records: input.records, PayloadBytes: input.padding, Concurrency: input.workers}
	dataset, err := workload.New(config)
	if err != nil {
		return err
	}
	base, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	ctx, cancel := context.WithTimeout(base, input.timeout)
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
	opts := benchmark.Options{Dataset: dataset, Paths: paths, Operations: input.operations, WarmupOperations: input.warmup, WritePercent: input.writes, Rounds: input.rounds, OperationTimeout: input.operationTimeout, Provenance: provenance}
	report, runErr := benchmark.Run(ctx, opts)
	if report == nil {
		return runErr
	}
	output := input.output
	if output == "" {
		output = filepath.Join(".testdata", "bench-"+input.backend+"-"+namespace)
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

func validate(input settings) error {
	if input.backend != "mongo" && input.backend != "search" || input.weirSeed == "" || input.store == "" || input.timeout <= 0 {
		return errors.New("requires --backend mongo|search, --weir-seed, --store and a positive timeout")
	}
	if err := validateAddress(input.weirSeed, input.allowRemote); err != nil {
		return err
	}
	raw := input.searchURL
	if input.backend == "mongo" {
		raw = input.mongoURI
	}
	address, err := url.Parse(raw)
	if err != nil || address.User != nil || address.Host == "" || address.Fragment != "" {
		return errors.New("requires an explicit credential-free backend endpoint")
	}
	if input.backend == "mongo" {
		if address.Scheme != "mongodb" || address.Path != "" && address.Path != "/" || strings.Contains(address.Host, ",") {
			return errors.New("Mongo endpoint requires mongodb://IP:port/ for one server")
		}
		query, err := url.ParseQuery(address.RawQuery)
		if err != nil {
			return errors.New("invalid Mongo URI query")
		}
		for key, values := range query {
			if key != "directConnection" || len(values) != 1 || values[0] != "true" {
				return errors.New("only directConnection=true is accepted in the fixture Mongo URI")
			}
		}
	} else if (address.Scheme != "http" && address.Scheme != "https") || address.Path != "" && address.Path != "/" || address.RawQuery != "" {
		return errors.New("Search endpoint requires an http(s) root URL")
	}
	return validateAddress(address.Host, input.allowRemote)
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
