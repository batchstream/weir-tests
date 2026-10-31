// weir-soak runs a long write and read workload through explicitly supplied
// Weir and database services and checks every reported outcome against the
// databases directly. It never starts, stops or faults services.
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/batchstream/weir-tests/internal/soak"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run() error {
	var config soak.Config
	mongo := soak.Backend{Kind: "mongo"}
	search := soak.Backend{Kind: "search"}
	flag.StringVar(&config.WeirAddress, "weir-address", "", "Weir application host:port; a name resolving to every replica spreads calls across them")
	flag.StringVar(&config.WeirDiagnostics, "weir-diagnostics", "", "Weir diagnostics host:port, resolved at every sample to scrape each replica")
	flag.StringVar(&mongo.URI, "mongo-uri", "", "credential-free MongoDB URI for direct checks; empty skips MongoDB")
	flag.StringVar(&mongo.Store, "mongo-store", "mongo", "Weir Store connected to that MongoDB")
	flag.IntVar(&mongo.Workers, "mongo-workers", 128, "sequential MongoDB workers")
	flag.Float64Var(&mongo.Rate, "mongo-rate", 2000, "MongoDB operations per second")
	flag.StringVar(&search.URI, "search-url", "", "credential-free Search root URL for direct checks; empty skips Search")
	flag.StringVar(&search.Store, "search-store", "search", "Weir Store connected to that Search cluster")
	flag.IntVar(&search.Workers, "search-workers", 256, "sequential Search workers")
	flag.Float64Var(&search.Rate, "search-rate", 8000, "Search operations per second")
	keys := flag.Int("keys-per-worker", 64, "keys each worker alone writes")
	flag.DurationVar(&config.Duration, "duration", 12*time.Hour, "load duration before the final check")
	flag.DurationVar(&config.SampleInterval, "sample-interval", time.Minute, "timeline interval")
	flag.DurationVar(&config.OperationTimeout, "operation-timeout", 30*time.Second, "per-call deadline; an expired write counts as unknown")
	flag.DurationVar(&config.Settle, "settle", 30*time.Second, "wait after the load stops so unknown writes finish before the final check")
	flag.IntVar(&config.ReadPercent, "read-percent", 10, "share of operations that read a key through Weir")
	flag.IntVar(&config.VerifyPercent, "verify-percent", 1, "share of APPLIED writes read back directly from the database")
	flag.IntVar(&config.PaddingBytes, "padding-bytes", 1024, "document padding")
	flag.StringVar(&config.Output, "output", "", "report path prefix; writes .json and .jsonl")
	flag.BoolVar(&config.KeepData, "keep-data", false, "keep the run's database namespace")
	flag.Parse()
	for _, backend := range []soak.Backend{mongo, search} {
		if backend.URI != "" {
			backend.KeysPerWorker = *keys
			config.Backends = append(config.Backends, backend)
		}
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	return soak.Run(ctx, config)
}
