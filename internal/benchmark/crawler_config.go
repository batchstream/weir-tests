package benchmark

import (
	"encoding/json"
	"errors"
	"strconv"
	"strings"
	"time"

	"github.com/batchstream/weir-tests/internal/observe"
)

const crawlerReportSchema = 1

func normalizeCrawlerControls(opts *CrawlerOptions) error {
	if len(opts.Paths) == 0 {
		opts.Paths = []string{"direct", "before", "after"}
	}
	seenPaths := map[string]bool{}
	for _, path := range opts.Paths {
		if path != "direct" && path != "before" && path != "after" || seenPaths[path] {
			return errors.New("crawler paths must contain unique direct/before/after values")
		}
		seenPaths[path] = true
	}
	if opts.Visibility == "" {
		opts.Visibility = "query_visible"
	}
	if opts.Visibility != "query_visible" && opts.Visibility != "application_acknowledged" {
		return errors.New("visibility must be query_visible or application_acknowledged")
	}
	if opts.Visibility != "query_visible" && (len(opts.Profiles) != 1 || opts.Profiles[0] != "partial-update") {
		return errors.New("application_acknowledged requires the partial-update profile")
	}
	if opts.SeedPaddingBytes < 0 || opts.SeedPaddingBytes > 1<<20 || opts.SeedPaddingBytes > 0 && (len(opts.Profiles) != 1 || opts.Profiles[0] != "partial-update") {
		return errors.New("seed-padding-bytes requires partial-update and must be between zero and 1 MiB")
	}
	if len(opts.Concurrency) > 0 && int64(opts.Concurrency[len(opts.Concurrency)-1])*int64(opts.PageSize)*2*int64(opts.SeedPaddingBytes) > 256<<20 {
		return errors.New("seed padding workspace exceeds 256 MiB")
	}
	if opts.AfterBatchMaxOperations == 0 {
		opts.AfterBatchMaxOperations = 32
	}
	if opts.AfterBatchMaxBytes == 0 {
		opts.AfterBatchMaxBytes = 8 << 20
	}
	if opts.AfterBatchMaxOperations < 1 || opts.AfterBatchMaxOperations > 1024 || opts.AfterBatchMaxBytes < 1 || opts.AfterBatchMaxBytes > 32<<20 || opts.AfterMaxInFlight < 0 || opts.AfterMaxInFlight > 4096 || opts.AfterBatchMaxWait < 0 || opts.AfterBatchMaxWait > time.Second {
		return errors.New("invalid bounded crawler batch controls")
	}
	return nil
}

func auditCrawlerServerConfiguration(stage crawlerStage, opts CrawlerOptions) error {
	var configuration struct {
		Stores []struct {
			Name     string          `json:"name"`
			Batching json.RawMessage `json:"batching"`
		} `json:"stores"`
	}
	if err := json.Unmarshal(stage.ServerConfiguration, &configuration); err != nil {
		return err
	}
	if len(configuration.Stores) != 1 || configuration.Stores[0].Name != "search" {
		return errors.New("crawler stage lacks its owned Search Store configuration")
	}
	operations, bytes, inFlight, wait := 32, 8<<20, 0, time.Duration(0)
	if stage.Path == "after" {
		operations, bytes, inFlight, wait = opts.AfterBatchMaxOperations, opts.AfterBatchMaxBytes, opts.AfterMaxInFlight, opts.AfterBatchMaxWait
	}
	if stage.BatchMaxWaitNS != int64(wait) {
		return errors.New("crawler stage collection wait differs from its declared path")
	}
	var batch struct {
		Operations *int    `json:"max_operations"`
		Bytes      *string `json:"max_bytes"`
		InFlight   *int    `json:"max_in_flight"`
		Wait       *string `json:"max_wait"`
	}
	if err := json.Unmarshal(configuration.Stores[0].Batching, &batch); err != nil {
		return err
	}
	if batch.Operations == nil || batch.Bytes == nil || batch.InFlight == nil || batch.Wait == nil {
		return errors.New("explicit crawler batching must include all four controls, including zeros")
	}
	configuredBytes, err := strconv.Atoi(strings.TrimSuffix(*batch.Bytes, "B"))
	if err != nil || !strings.HasSuffix(*batch.Bytes, "B") {
		return errors.New("crawler stage batch byte limit is missing or invalid")
	}
	var configuredWait time.Duration
	if *batch.Wait != "" {
		configuredWait, err = time.ParseDuration(*batch.Wait)
		if err != nil {
			return err
		}
	}
	if *batch.Operations != operations || configuredBytes != bytes || *batch.InFlight != inFlight || configuredWait != wait {
		return errors.New("crawler stage batching configuration differs from its declared path")
	}
	return nil
}

// weirDefaultMaxInFlight is the execution limit Weir selects for max_in_flight: 0.
const weirDefaultMaxInFlight = 16

// liveExecutionLimit is the execution limit gauge a declared max_in_flight produces.
func liveExecutionLimit(inFlight int) float64 {
	if inFlight == 0 {
		return weirDefaultMaxInFlight
	}
	return float64(inFlight)
}

func auditCrawlerLiveControls(stage crawlerStage, opts CrawlerOptions) error {
	inFlight := 0
	if stage.Path == "after" {
		inFlight = opts.AfterMaxInFlight
	}
	expected := map[string]float64{"weir_store_active_executions_limit": liveExecutionLimit(inFlight)}
	for _, snapshot := range []observe.Snapshot{stage.ServerBefore, stage.ServerAfter} {
		if snapshot.Error != "" {
			return errors.New("crawler live batching controls are unavailable")
		}
		for name, value := range expected {
			found := 0
			for _, metric := range snapshot.Metrics {
				if metric.Name == name && metric.Labels["store"] == "search" {
					found++
					if metric.Value != value {
						return errors.New("crawler live batching control differs from its declared path")
					}
				}
			}
			if found != 1 {
				return errors.New("crawler live batching control is missing or duplicated")
			}
		}
	}
	return nil
}
