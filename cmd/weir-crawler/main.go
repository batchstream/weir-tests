// Command weir-crawler compares realistic crawler mutations on exclusively owned Search fixtures.
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/batchstream/weir-tests/internal/benchmark"
)

func main() {
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	if len(os.Args) == 3 && os.Args[1] == "audit" {
		raw, err := os.ReadFile(os.Args[2])
		var report benchmark.CrawlerReport
		if err == nil {
			err = json.Unmarshal(raw, &report)
		}
		if err == nil {
			err = benchmark.AuditCrawlerReport(&report)
		}
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		fmt.Printf("verified %d crawler stages offline\n", len(report.Stages))
		return
	}
	if len(os.Args) == 2 && os.Args[1] == "client-worker" {
		if err := benchmark.RunCrawlerWorker(ctx, os.Stdin, os.Stdout); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		return
	}
	var opts benchmark.CrawlerOptions
	var rates, levels, profiles, modes string
	flag.StringVar(&opts.InputShape, "input-shape", "single", "single (one record per native update/SDK call, page-size 1) or explicit page batch control")
	flag.StringVar(&opts.HTTPObserver, "http-observer", "disabled", "enabled or disabled; disabled bypasses the observer for all paths and leaves HTTP counters unavailable")
	flag.StringVar(&opts.Before, "before", ".tools/weir-current", "immutable current Weir binary")
	flag.StringVar(&opts.After, "after", ".tools/weir-optimized", "immutable optimized Weir binary")
	flag.StringVar(&opts.BeforeReceipt, "before-receipt", ".tools/weir-current.receipt.json", "current server source and binary receipt")
	flag.StringVar(&opts.AfterReceipt, "after-receipt", ".tools/weir-optimized.receipt.json", "optimized server source and binary receipt")
	flag.StringVar(&opts.Output, "output", "results/local/crawler.json", "raw evidence report")
	flag.StringVar(&profiles, "profiles", "two-step", "two-step or combined crawler rules, or exclusive partial-update batching mechanism control")
	flag.StringVar(&modes, "modes", "fixed-rate", "fixed-rate, saturation, or both comma-separated")
	flag.StringVar(&rates, "rates", "4,16,48", "absolute offered product rates; none disables fixed-rate cases")
	flag.StringVar(&levels, "concurrency-levels", "128", "total goroutine workers across OS clients: <=2048 total, <=512 per process; fixed-rate uses only the largest level")
	flag.StringVar(&opts.Visibility, "visibility", "query_visible", "query_visible or application_acknowledged (partial-update only); identical native and SDK completion")
	flag.IntVar(&opts.SeedPaddingBytes, "seed-padding-bytes", 0, "additional untouched product payload bytes, partial-update only")
	flag.IntVar(&opts.AfterBatchMaxOperations, "after-batch-max-operations", 32, "optimized server batching.max_operations")
	flag.IntVar(&opts.AfterBatchMaxBytes, "after-batch-max-bytes", 8<<20, "optimized server batching.max_bytes in bytes")
	flag.IntVar(&opts.AfterMaxInFlight, "after-max-in-flight", 0, "optimized server batch cap; zero is unlimited and omitted for the old server")
	flag.IntVar(&opts.Processes, "client-processes", 4, "independent OS clients, one pool per process")
	flag.IntVar(&opts.PageSize, "page-size", 1, "products per page, each with two sellers")
	flag.IntVar(&opts.QueuePages, "queue-pages", 1, "bounded queued pages per worker, counted explicitly")
	flag.IntVar(&opts.Rounds, "rounds", 3, "three-way rotating rounds")
	flag.Float64Var(&opts.CPUs, "database-cpus", 1, "enforced owned Search container CPU quota")
	flag.DurationVar(&opts.AfterBatchMaxWait, "after-batch-max-wait", 0, "optimized server batching.max_wait; zero preserves immediate execution")
	flag.DurationVar(&opts.Warmup, "warmup", 10*time.Second, "untimed warmup followed by data reset with pools retained")
	flag.DurationVar(&opts.Duration, "duration", 20*time.Second, "offered arrival interval; subsequent completion drain remains included")
	flag.DurationVar(&opts.Timeout, "timeout", 10*time.Second, "same planned-arrival-to-declared-completion deadline for all paths")
	flag.Parse()
	var err error
	opts.Profiles = strings.Split(profiles, ",")
	opts.Modes = strings.Split(modes, ",")
	if rates == "none" {
		rates = ""
	}
	opts.Rates, err = positiveList(rates)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	opts.Concurrency, err = positiveList(levels)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	opts.Executable, err = os.Executable()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	opts.Before, _ = filepath.Abs(opts.Before)
	opts.After, _ = filepath.Abs(opts.After)
	if _, err := benchmark.RunCrawler(ctx, opts); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
func positiveList(raw string) ([]int, error) {
	if raw == "" {
		return nil, nil
	}
	var values []int
	previous := 0
	for _, part := range strings.Split(raw, ",") {
		value, err := strconv.Atoi(part)
		if err != nil || value <= previous {
			return nil, fmt.Errorf("levels/rates must be ascending positive integers: %q", raw)
		}
		values = append(values, value)
		previous = value
	}
	return values, nil
}
