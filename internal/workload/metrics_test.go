package workload

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

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
}

func TestSearchCountersProjectScalarsAndExcludeLargeClientHistory(t *testing.T) {
	dataset := testDataset(t, "search")
	agent := strings.Repeat("x", 256)
	entry := fmt.Sprintf(`{"id":1,"agent":%q},`, agent)
	history := "[" + strings.TrimSuffix(strings.Repeat(entry, 4096), ",") + "]"
	full := `{"nodes":{"owned":{"http":{"current_open":7,"clients":` + history + `},"transport":{"rx_size_in_bytes":20,"tx_size_in_bytes":30},"fs":{"io_stats":{"total":{"read_kilobytes":2,"write_kilobytes":3}}}}}}`
	limit := 64*(dataset.Config.PayloadBytes+1024) + 64<<10
	if len(full) <= limit {
		t.Fatal("regression must reproduce a response exceeding the business workspace")
	}
	projected := `{"nodes":{"owned":{"http":{"current_open":7},"transport":{"rx_size_in_bytes":20,"tx_size_in_bytes":30},"fs":{"io_stats":{"total":{"read_kilobytes":2,"write_kilobytes":3}}}}}}`
	allowed := map[string]bool{
		"nodes.*.http.current_open":                 true,
		"nodes.*.transport.rx_size_in_bytes":        true,
		"nodes.*.transport.tx_size_in_bytes":        true,
		"nodes.*.fs.io_stats.total.read_kilobytes":  true,
		"nodes.*.fs.io_stats.total.write_kilobytes": true,
	}
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodGet || request.URL.Path != "/_nodes/stats/fs,transport,http" {
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
	_, _, err := direct.request(context.Background(), http.MethodGet, "/_nodes/stats/fs,transport,http", nil)
	if err == nil || !strings.Contains(err.Error(), "bounded document workspace") {
		t.Fatal("unprojected history must reproduce the original bounded-response failure", err)
	}
	paths := &Paths{direct: direct}
	counters, err := paths.DatabaseCounters(context.Background())
	if err != nil || counters.Connections == nil || *counters.Connections != 7 || counters.NetworkIn != 20 || counters.NetworkOut != 30 || counters.ReadBytes != 2*1024 || counters.WriteBytes != 3*1024 {
		t.Fatal("projected counters lost independent statistics or grew with history", counters, err)
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
