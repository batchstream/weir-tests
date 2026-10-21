package benchmark

import (
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/batchstream/weir-tests/internal/observe"
)

func crawlerTestConfiguration(t *testing.T, opts CrawlerOptions, path string) json.RawMessage {
	t.Helper()
	if err := normalizeCrawlerControls(&opts); err != nil {
		t.Fatal(err)
	}
	operations, bytes, inFlight, wait := 32, 8<<20, 0, time.Duration(0)
	if path == "after" {
		operations, bytes, inFlight, wait = opts.AfterBatchMaxOperations, opts.AfterBatchMaxBytes, opts.AfterMaxInFlight, opts.AfterBatchMaxWait
	}
	batching := map[string]any{"max_operations": operations, "max_bytes": fmt.Sprintf("%dB", bytes), "max_in_flight": inFlight, "max_wait": wait.String()}
	store := map[string]any{"name": "search", "batching": batching}
	configuration := map[string]any{"stores": []any{store}}
	raw, err := json.Marshal(configuration)
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

func TestCrawlerControlsPreserveDefaultsAndAllowIndependentAfterControls(t *testing.T) {
	opts := CrawlerOptions{Profiles: []string{"partial-update"}}
	if err := normalizeCrawlerControls(&opts); err != nil {
		t.Fatal(err)
	}
	if opts.Visibility != "query_visible" || opts.AfterBatchMaxOperations != 32 || opts.AfterBatchMaxBytes != 8<<20 || opts.AfterMaxInFlight != 0 || opts.AfterBatchMaxWait != 0 || opts.SeedPaddingBytes != 0 {
		t.Fatal("existing benchmark defaults changed", opts)
	}
	opts.Visibility, opts.SeedPaddingBytes = "application_acknowledged", 8192
	opts.AfterBatchMaxOperations, opts.AfterBatchMaxBytes, opts.AfterMaxInFlight = 64, 65536, 4
	opts.AfterBatchMaxWait = 20 * time.Millisecond
	if err := normalizeCrawlerControls(&opts); err != nil {
		t.Fatal("independent after-path controls were rejected", err)
	}
	for _, path := range []string{"direct", "before", "after"} {
		stage := crawlerStage{Path: path, ServerConfiguration: crawlerTestConfiguration(t, opts, path)}
		if path == "after" {
			stage.BatchMaxWaitNS = int64(opts.AfterBatchMaxWait)
		}
		if err := auditCrawlerServerConfiguration(stage, opts); err != nil {
			t.Fatal(path, err)
		}
		stage.ServerConfiguration = nil
		if err := auditCrawlerServerConfiguration(stage, opts); err == nil {
			t.Fatal("missing owned configuration was accepted", path)
		}
	}
}

func TestCrawlerConfigurationAuditRejectsEveryChangedAfterLimit(t *testing.T) {
	opts := CrawlerOptions{Profiles: []string{"partial-update"}, AfterBatchMaxOperations: 64, AfterBatchMaxBytes: 65536, AfterMaxInFlight: 4, AfterBatchMaxWait: 20 * time.Millisecond}
	if err := normalizeCrawlerControls(&opts); err != nil {
		t.Fatal(err)
	}
	stage := crawlerStage{Path: "after", BatchMaxWaitNS: int64(opts.AfterBatchMaxWait), ServerConfiguration: crawlerTestConfiguration(t, opts, "after")}
	for _, field := range []string{"operations", "bytes", "in-flight", "wait"} {
		changed := opts
		switch field {
		case "operations":
			changed.AfterBatchMaxOperations++
		case "bytes":
			changed.AfterBatchMaxBytes++
		case "in-flight":
			changed.AfterMaxInFlight++
		case "wait":
			changed.AfterBatchMaxWait++
		}
		if err := auditCrawlerServerConfiguration(stage, changed); err == nil {
			t.Fatal("owned route and declared limit mismatch was accepted", field)
		}
	}
}

func TestCrawlerControlsRejectInvalidVisibilityPayloadAndBatchBounds(t *testing.T) {
	for _, invalid := range []string{"visibility", "business-visibility", "business-padding", "negative-padding", "large-padding", "workspace", "negative-operations", "large-operations", "negative-bytes", "large-bytes"} {
		opts := CrawlerOptions{Profiles: []string{"partial-update"}, Concurrency: []int{4}, PageSize: 1}
		switch invalid {
		case "visibility":
			opts.Visibility = "accepted"
		case "business-visibility":
			opts.Profiles, opts.Visibility = []string{"two-step"}, "application_acknowledged"
		case "business-padding":
			opts.Profiles, opts.SeedPaddingBytes = []string{"two-step"}, 1
		case "negative-padding":
			opts.SeedPaddingBytes = -1
		case "large-padding":
			opts.SeedPaddingBytes = (1 << 20) + 1
		case "workspace":
			opts.Concurrency, opts.SeedPaddingBytes = []int{2048}, 1<<20
		case "negative-operations":
			opts.AfterBatchMaxOperations = -1
		case "large-operations":
			opts.AfterBatchMaxOperations = 1025
		case "negative-bytes":
			opts.AfterBatchMaxBytes = -1
		case "large-bytes":
			opts.AfterBatchMaxBytes = (32 << 20) + 1
		}
		if err := normalizeCrawlerControls(&opts); err == nil {
			t.Fatal("invalid experiment control was accepted", invalid)
		}
	}
}

func TestCrawlerLiveControlsRequireBothSnapshotsAndDeclaredValues(t *testing.T) {
	opts := CrawlerOptions{AfterMaxInFlight: 4, AfterBatchMaxWait: 20 * time.Millisecond}
	labels := map[string]string{"store": "search"}
	wait := observe.Metric{Name: "weir_store_batch_max_wait_seconds", Labels: labels, Value: .02}
	cap := observe.Metric{Name: "weir_store_active_executions_limit", Labels: labels, Value: 4}
	snapshot := observe.Snapshot{Metrics: []observe.Metric{wait, cap}}
	stage := crawlerStage{Path: "after", ServerBefore: snapshot, ServerAfter: snapshot}
	if err := auditCrawlerLiveControls(stage, opts); err != nil {
		t.Fatal(err)
	}
	stage.ServerAfter.Metrics = []observe.Metric{wait}
	if err := auditCrawlerLiveControls(stage, opts); err == nil {
		t.Fatal("missing cap gauge became a synthetic zero")
	}
	cap.Value = 0
	stage.ServerAfter.Metrics = []observe.Metric{wait, cap}
	if err := auditCrawlerLiveControls(stage, opts); err == nil {
		t.Fatal("changed live cap was accepted despite the route declaration")
	}
	cap.Value = 4
	stage.ServerAfter.Metrics = []observe.Metric{wait, cap, cap}
	if err := auditCrawlerLiveControls(stage, opts); err == nil {
		t.Fatal("ambiguous live gauge series were accepted")
	}
}

func TestCrawlerDefaultPathOmitsControlsAndAuditsAllResolvedGauges(t *testing.T) {
	opts := CrawlerOptions{AfterUseDefaults: true, CaptureEffectiveConfig: true,
		AfterBatchMaxOperations: 128, AfterBatchMaxBytes: 8 << 20, AfterMaxInFlight: 16,
		AfterBatchMaxWait: 5 * time.Millisecond, Paths: []string{"direct", "after"}}
	if err := normalizeCrawlerControls(&opts); err != nil {
		t.Fatal(err)
	}
	configuration := json.RawMessage(`{"stores":[{"name":"search"}]}`)
	stage := crawlerStage{Path: "after", BatchMaxWaitNS: int64(opts.AfterBatchMaxWait),
		ServerConfiguration: configuration, ServerEffectiveConfiguration: "resolved owned settings"}
	if err := auditCrawlerServerConfiguration(stage, opts); err != nil {
		t.Fatal("omitted defaults were not accepted", err)
	}
	labels := map[string]string{"store": "search"}
	wait := observe.Metric{Name: "weir_store_batch_max_wait_seconds", Labels: labels, Value: .005}
	cap := observe.Metric{Name: "weir_store_active_executions_limit", Labels: labels, Value: 16}
	count := observe.Metric{Name: "weir_store_batch_max_operations", Labels: labels, Value: 128}
	bytes := observe.Metric{Name: "weir_store_batch_max_bytes", Labels: labels, Value: 8 << 20}
	snapshot := observe.Snapshot{Metrics: []observe.Metric{wait, cap, count, bytes}}
	stage.ServerBefore, stage.ServerAfter = snapshot, snapshot
	if err := auditCrawlerLiveControls(stage, opts); err != nil {
		t.Fatal("declared defaults did not match resolved gauges", err)
	}
	stage.ServerAfter.Metrics = []observe.Metric{wait, cap, count}
	if err := auditCrawlerLiveControls(stage, opts); err == nil {
		t.Fatal("missing byte gauge hid unknown default", err)
	}
	stage.ServerConfiguration = crawlerTestConfiguration(t, opts, "after")
	if err := auditCrawlerServerConfiguration(stage, opts); err == nil {
		t.Fatal("an explicit override was presented as an omitted default")
	}
	stage.Path, stage.BatchMaxWaitNS = "before", 0
	stage.ServerConfiguration = json.RawMessage(`{"stores":[{"name":"search","batching":{"max_operations":32,"max_bytes":"8388608B"}}]}`)
	if err := auditCrawlerServerConfiguration(stage, opts); err == nil {
		t.Fatal("explicit zero wait and zero slots were silently omitted")
	}
}

func TestCrawlerPathsRejectUnknownOrDuplicateAndRetainDeclaredSubset(t *testing.T) {
	for _, paths := range [][]string{{"direct", "after"}, {"after"}} {
		opts := CrawlerOptions{Paths: paths}
		if err := normalizeCrawlerControls(&opts); err != nil {
			t.Fatal("valid selected paths rejected", paths, err)
		}
		if len(opts.Paths) != len(paths) {
			t.Fatal("unselected path was added", opts.Paths)
		}
	}
	for _, paths := range [][]string{{"direct", "direct"}, {"native"}} {
		opts := CrawlerOptions{Paths: paths}
		if err := normalizeCrawlerControls(&opts); err == nil {
			t.Fatal("ambiguous path selection was accepted", paths)
		}
	}
}
