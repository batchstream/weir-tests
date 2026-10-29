package workload

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
)

func TestMongoCommandCountersIgnoreHeterogeneousUnknownAndPreserveMissing(t *testing.T) {
	zero := bson.D{{Key: "total", Value: int64(0)}}
	positive := bson.D{{Key: "total", Value: int64(71)}}
	commands := bson.D{
		{Key: "<UNKNOWN>", Value: int64(3)},
		{Key: "find", Value: zero},
		{Key: "update", Value: bson.D{}},
		{Key: "bulkWrite", Value: positive},
		{Key: "killCursors", Value: zero},
		{Key: "commitTransaction", Value: positive},
		{Key: "abortTransaction", Value: zero},
	}
	metrics := bson.D{{Key: "commands", Value: commands}}
	connections := bson.D{{Key: "current", Value: int32(9)}}
	network := bson.D{{Key: "bytesIn", Value: int64(12)}, {Key: "bytesOut", Value: int64(34)}}
	cache := bson.D{{Key: "bytes read into cache", Value: int64(56)}, {Key: "bytes written from cache", Value: int64(78)}}
	wiredTiger := bson.D{{Key: "cache", Value: cache}}
	status := bson.D{{Key: "metrics", Value: metrics}, {Key: "connections", Value: connections}, {Key: "network", Value: network}, {Key: "wiredTiger", Value: wiredTiger}}
	raw, err := bson.Marshal(status)
	if err != nil {
		t.Fatal(err)
	}
	var reply mongoServerStatus
	if err := bson.Unmarshal(raw, &reply); err != nil {
		t.Fatal("the real scalar <UNKNOWN> command must not invalidate known counters", err)
	}
	counters := reply.counters()
	for _, name := range []string{"find", "killCursors", "abortTransaction"} {
		value, found := counters.Commands[name]
		if !found || value != 0 {
			t.Fatal("observed zero was mistaken for missing", name, counters)
		}
	}
	for _, name := range []string{"update", "getMore"} {
		if _, found := counters.Commands[name]; found || !strings.Contains(counters.CommandUnavailable, "missing "+name) {
			t.Fatal("absent total or absent command silently became zero", name, counters)
		}
	}
	if len(counters.Commands) != 5 || counters.Commands["bulkWrite"] != 71 || counters.Commands["commitTransaction"] != 71 || counters.Connections == nil || *counters.Connections != 9 || counters.NetworkIn != 12 || counters.NetworkOut != 34 || counters.ReadBytes != 56 || counters.WriteBytes != 78 {
		t.Fatal("known physical command, connection or storage evidence lost", counters)
	}
	if counters.ProcessCPUNS != nil {
		t.Fatal("missing extra_info became a process CPU sample", *counters.ProcessCPUNS)
	}
	localTime := time.Date(2026, 10, 30, 8, 0, 0, 0, time.UTC)
	for _, scenario := range []struct {
		name    string
		process string
		user    int64
		want    bool
	}{
		{name: "mongod", process: "mongod", user: 1500, want: true},
		{name: "router", process: "mongos", user: 1500},
		{name: "negative", process: "mongod", user: -1},
	} {
		extra := bson.D{{Key: "user_time_us", Value: scenario.user}, {Key: "system_time_us", Value: int64(500)}}
		cpuFields := bson.D{{Key: "process", Value: scenario.process}, {Key: "localTime", Value: localTime}, {Key: "extra_info", Value: extra}}
		withCPU := append(status, cpuFields...)
		raw, err = bson.Marshal(withCPU)
		if err != nil {
			t.Fatal(err)
		}
		var cpuReply mongoServerStatus
		if err := bson.Unmarshal(raw, &cpuReply); err != nil {
			t.Fatal(err)
		}
		cpuCounters := cpuReply.counters()
		cpu, at := cpuCounters.ProcessCPUNS, cpuCounters.ProcessCPUAtNS
		if !scenario.want {
			if cpu != nil || at != nil {
				t.Fatal("router or invalid CPU became a database CPU sample", scenario.name)
			}
			continue
		}
		if cpu == nil || *cpu != int64(2*time.Millisecond) || at == nil || *at != localTime.UnixNano() {
			t.Fatal("mongod user+system CPU was not converted to nanoseconds at its server time", cpu, at)
		}
	}
}

func TestSearchCountersProjectScalarsAndExcludeLargeClientHistory(t *testing.T) {
	dataset := testDataset(t, "search")
	agent := strings.Repeat("x", 256)
	entry := fmt.Sprintf(`{"id":1,"agent":%q},`, agent)
	history := "[" + strings.TrimSuffix(strings.Repeat(entry, 4096), ",") + "]"
	full := `{"nodes":{"owned":{"http":{"current_open":7,"clients":` + history + `},"transport":{"rx_size_in_bytes":20,"tx_size_in_bytes":30},"fs":{"io_stats":{"total":{"read_kilobytes":2,"write_kilobytes":3}}}}}}`
	limit := 64*(dataset.Config.PayloadBytes+1024) + 64<<10
	if len(full) <= limit {
		t.Fatal("test must produce a response exceeding the business workspace")
	}
	projected := `{"nodes":{"owned":{"http":{"current_open":7},"transport":{"rx_size_in_bytes":20,"tx_size_in_bytes":30},"fs":{"io_stats":{"total":{"read_kilobytes":2,"write_kilobytes":3}}},"process":{"timestamp":1793347200000,"cpu":{"total_in_millis":1500}}}}}`
	allowed := map[string]bool{
		"nodes.*.http.current_open":                   true,
		"nodes.*.transport.rx_size_in_bytes":          true,
		"nodes.*.transport.tx_size_in_bytes":          true,
		"nodes.*.fs.io_stats.total.read_kilobytes":    true,
		"nodes.*.fs.io_stats.total.write_kilobytes":   true,
		"nodes.*.fs.io_stats.total.operations":        true,
		"nodes.*.fs.io_stats.total.io_time_in_millis": true,
		"nodes.*.thread_pool.write":                   true,
		"nodes.*.thread_pool.write_coordination":      true,
		"nodes.*.indexing_pressure.memory":            true,
		"nodes.*.jvm.gc.collectors":                   true,
		"nodes.*.jvm.mem.heap_used_in_bytes":          true,
		"nodes.*.jvm.mem.heap_max_in_bytes":           true,
		"nodes.*.process.timestamp":                   true,
		"nodes.*.process.cpu.total_in_millis":         true,
	}
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodGet || request.URL.Path != "/_nodes/stats/fs,transport,http,thread_pool,indexing_pressure,jvm,process" {
			t.Error("unexpected monitor request", request.Method, request.URL.Path)
		}
		filters := strings.Split(request.URL.Query().Get("filter_path"), ",")
		valid := len(filters) == len(allowed)
		seen := make(map[string]bool)
		for _, filter := range filters {
			valid = valid && allowed[filter] && !seen[filter]
			seen[filter] = true
		}
		if !valid {
			fmt.Fprint(writer, full)
			return
		}
		fmt.Fprint(writer, projected)
	}))
	defer server.Close()
	direct := &directPath{dataset: dataset, search: server.Client(), baseURL: server.URL}
	paths := &Paths{direct: direct}
	counters, err := paths.DatabaseCounters(context.Background())
	if err != nil || counters.Connections == nil || *counters.Connections != 7 || counters.NetworkIn != 20 || counters.NetworkOut != 30 || counters.ReadBytes != 2*1024 || counters.WriteBytes != 3*1024 || counters.ProcessCPUNS == nil || *counters.ProcessCPUNS != int64(1500*time.Millisecond) || counters.ProcessCPUAtNS == nil || *counters.ProcessCPUAtNS != 1793347200000*int64(time.Millisecond) {
		t.Fatal("projected counters lost independent statistics or grew with history", counters, err)
	}
	projected = strings.Replace(projected, `"total_in_millis":1500`, `"total_in_millis":-1`, 1)
	counters, err = paths.DatabaseCounters(context.Background())
	if err != nil || counters.ProcessCPUNS != nil {
		t.Fatal("unreadable Elasticsearch CPU (-1) became a sample", counters, err)
	}
}

func TestSearchSnapshotPreservesEffectivePolicyAndOwnedIndexEvidence(t *testing.T) {
	dataset := testDataset(t, "search")
	for _, scenario := range []string{"valid", "async", "foreign-index", "failed-shard", "missing-policy"} {
		t.Run(scenario, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
				switch {
				case strings.HasPrefix(request.URL.Path, "/_nodes/"):
					fmt.Fprint(writer, `{"nodes":{"owned":{"http":{"current_open":7},"transport":{"rx_size_in_bytes":20,"tx_size_in_bytes":30},"thread_pool":{"write":{"threads":1,"active":1,"queue":2,"rejected":0,"completed":3}},"indexing_pressure":{"memory":{"current":{"all_in_bytes":0},"total":{"coordinating_rejections":0,"primary_rejections":0,"replica_rejections":0},"limit_in_bytes":1024}}}}}`)
				case strings.HasSuffix(request.URL.Path, "/_settings"):
					if request.URL.Query().Get("include_defaults") != "true" || request.URL.Query().Get("flat_settings") != "true" {
						t.Error("effective settings omitted explicit/default evidence")
					}
					durability := "request"
					if scenario == "async" {
						durability = "async"
					}
					index := dataset.Config.Namespace
					if scenario == "foreign-index" {
						index = "foreign"
					}
					if scenario == "missing-policy" {
						fmt.Fprintf(writer, `{%q:{"settings":{}}}`, index)
						return
					}
					fmt.Fprintf(writer, `{%q:{"settings":{"index.refresh_interval":"1s","index.translog.durability":%q,"index.number_of_shards":"1","index.number_of_replicas":"0"},"defaults":{"index.search.idle.after":"30s"}}}`, index, durability)
				case strings.Contains(request.URL.Path, "/_stats/"):
					failed := 0
					if scenario == "failed-shard" {
						failed = 1
					}
					fmt.Fprintf(writer, `{"_shards":{"failed":%d},"indices":{%q:{"primaries":{"refresh":{"total":4},"translog":{"operations":5}}}}}`, failed, dataset.Config.Namespace)
				default:
					t.Error("unexpected snapshot request", request.URL)
				}
			}))
			defer server.Close()
			direct := &directPath{dataset: dataset, search: server.Client(), baseURL: server.URL}
			paths := &Paths{direct: direct}
			counters, err := paths.DatabaseSnapshot(context.Background())
			if scenario != "valid" {
				if err == nil {
					t.Fatal("invalid effective policy or owned-index evidence accepted", scenario, counters)
				}
				return
			}
			if err != nil || counters.SearchUnavailable != "" || !strings.Contains(string(counters.SearchNodeStats), `"queue":2`) || !strings.Contains(string(counters.SearchSettings), `"index.search.idle.after":"30s"`) || !strings.Contains(string(counters.SearchIndexStats), `"operations":5`) {
				t.Fatal("independent settings, pressure or index statistics lost", counters, err)
			}
		})
	}
}

func TestSearchProjectedCountersDistinguishMissingFromObservedZero(t *testing.T) {
	cases := []struct {
		name               string
		body               string
		valid              bool
		storageUnavailable bool
	}{
		{name: "observed-zero", body: `{"nodes":{"owned":{"http":{"current_open":0},"transport":{"rx_size_in_bytes":0,"tx_size_in_bytes":0},"fs":{"io_stats":{"total":{"read_kilobytes":0,"write_kilobytes":0}}}}}}`, valid: true},
		{name: "missing-current-open", body: `{"nodes":{"owned":{"http":{},"transport":{"rx_size_in_bytes":0,"tx_size_in_bytes":0},"fs":{"io_stats":{"total":{"read_kilobytes":0,"write_kilobytes":0}}}}}}`},
		{name: "missing-transport", body: `{"nodes":{"owned":{"http":{"current_open":0},"fs":{"io_stats":{"total":{"read_kilobytes":0,"write_kilobytes":0}}}}}}`},
		{name: "missing-filesystem-total", body: `{"nodes":{"owned":{"http":{"current_open":0},"transport":{"rx_size_in_bytes":0,"tx_size_in_bytes":0},"fs":{}}}}`, valid: true, storageUnavailable: true},
		{name: "null-storage-counter", body: `{"nodes":{"owned":{"http":{"current_open":0},"transport":{"rx_size_in_bytes":0,"tx_size_in_bytes":0},"fs":{"io_stats":{"total":{"read_kilobytes":null,"write_kilobytes":0}}}}}}`, valid: true, storageUnavailable: true},
		{name: "no-node", body: `{"nodes":{}}`},
		{name: "multiple-nodes", body: `{"nodes":{"first":{},"second":{}}}`},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
				if request.URL.Query().Get("filter_path") == "" {
					t.Error("monitor request omitted scalar projection")
				}
				fmt.Fprint(writer, test.body)
			}))
			defer server.Close()
			direct := &directPath{dataset: testDataset(t, "search"), search: server.Client(), baseURL: server.URL}
			paths := &Paths{direct: direct}
			counters, err := paths.DatabaseCounters(context.Background())
			if test.valid {
				if err != nil || counters.Connections == nil || *counters.Connections != 0 || counters.NetworkIn != 0 || counters.NetworkOut != 0 || counters.ReadBytes != 0 || counters.WriteBytes != 0 {
					t.Fatal("observed zero counters were rejected", counters, err)
				}
				if (counters.StorageUnavailable != "") != test.storageUnavailable {
					t.Fatal("optional filesystem availability was hidden or confused with observed zero", counters)
				}
			} else if err == nil {
				t.Fatal("missing projected evidence silently became zero", counters)
			}
		})
	}
}

func TestMongoCommandCounterRejectsMalformedKnownTotal(t *testing.T) {
	find := bson.D{{Key: "total", Value: "invalid"}}
	commands := bson.D{{Key: "<UNKNOWN>", Value: int64(0)}, {Key: "find", Value: find}}
	metrics := bson.D{{Key: "commands", Value: commands}}
	status := bson.D{{Key: "metrics", Value: metrics}}
	raw, err := bson.Marshal(status)
	if err != nil {
		t.Fatal(err)
	}
	var reply mongoServerStatus
	if err := bson.Unmarshal(raw, &reply); err == nil {
		t.Fatal("malformed known command total became valid zero evidence")
	}
}
