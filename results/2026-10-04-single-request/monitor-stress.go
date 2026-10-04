//go:build ignore

package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"sync"
	"time"

	"github.com/batchstream/weir-tests/internal/fixture"
	"github.com/batchstream/weir-tests/internal/workload"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run() (resultErr error) {
	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Minute)
	defer cancel()
	binary := os.Getenv("WEIR_TEST_BINARY")
	if binary == "" {
		binary = ".tools/weir"
	}
	opts := fixture.Options{WeirBinary: binary, Backends: []string{"search"}, OwnerCount: 1, StoreConcurrency: 2, BatchSize: 32, IngressSessions: 64, DatabaseCPUs: 1}
	cluster, err := fixture.Start(ctx, opts)
	if err != nil {
		return err
	}
	fmt.Println("owned fixture", cluster.Directory)
	defer func() {
		cleanup, cancel := context.WithTimeout(context.Background(), 60*time.Second)
		defer cancel()
		resultErr = errors.Join(resultErr, cluster.Close(cleanup))
	}()
	namespace, err := workload.NewNamespace()
	if err != nil {
		return err
	}
	config := workload.Config{Backend: "search", SearchURL: cluster.SearchURL, WeirSeed: cluster.Seed(), StoreName: "search", Namespace: namespace, Records: 32, PayloadBytes: 1024, Concurrency: 4}
	dataset, err := workload.New(config)
	if err != nil {
		return err
	}
	paths, err := workload.Open(ctx, dataset)
	if err != nil {
		return err
	}
	defer func() {
		cleanup, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		resultErr = errors.Join(resultErr, paths.Cleanup(cleanup), paths.Close())
	}()
	before, err := paths.DatabaseCounters(ctx)
	if err != nil {
		return fmt.Errorf("initial projected real counters: %w", err)
	}
	raw, _ := json.Marshal(before)
	fmt.Println("before", string(raw))
	transport := &http.Transport{Proxy: nil, DisableKeepAlives: true, MaxConnsPerHost: 8}
	client := &http.Client{Transport: transport, Timeout: 10 * time.Second}
	defer client.CloseIdleConnections()
	var joined sync.WaitGroup
	workerErrors := make([]error, 8)
	for worker := range 8 {
		joined.Add(1)
		go func() {
			defer joined.Done()
			for range 256 {
				request, err := http.NewRequestWithContext(ctx, http.MethodGet, cluster.SearchURL+"/", nil)
				if err != nil {
					workerErrors[worker] = err
					return
				}
				response, err := client.Do(request)
				if err != nil {
					workerErrors[worker] = err
					return
				}
				_, readErr := io.Copy(io.Discard, response.Body)
				closeErr := response.Body.Close()
				if readErr != nil || closeErr != nil || response.StatusCode != http.StatusOK {
					workerErrors[worker] = errors.Join(readErr, closeErr, fmt.Errorf("status %d", response.StatusCode))
					return
				}
			}
		}()
	}
	joined.Wait()
	if err := errors.Join(workerErrors...); err != nil {
		return err
	}
	response, err := client.Get(cluster.SearchURL + "/_nodes/stats/fs,transport,http")
	if err != nil {
		return err
	}
	bytes, err := io.ReadAll(io.LimitReader(response.Body, 4<<20))
	closeErr := response.Body.Close()
	if err != nil || closeErr != nil {
		return errors.Join(err, closeErr)
	}
	limit := 64*(1024+1024) + 64<<10
	if len(bytes) <= limit {
		return fmt.Errorf("history response %d did not exceed old %d byte bound", len(bytes), limit)
	}
	fmt.Println("unfiltered_history_bytes", len(bytes), "old_business_bound", limit)
	for range 10 {
		counters, err := paths.DatabaseCounters(ctx)
		if err != nil {
			return fmt.Errorf("projected counters after 2048 connections: %w", err)
		}
		if counters.Connections == nil {
			return errors.New("actual current_open missing")
		}
		raw, _ := json.Marshal(counters)
		fmt.Println("after", string(raw))
	}
	fmt.Println("PASS real projection survives 2048 closed client connections")
	return nil
}
