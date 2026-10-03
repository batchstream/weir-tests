package workload

import (
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
	for _, name := range []string{"find", "killCursors"} {
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
	if len(counters.Commands) != 3 || counters.Commands["bulkWrite"] != 71 || counters.Connections == nil || *counters.Connections != 9 || counters.NetworkIn != 12 || counters.NetworkOut != 34 || counters.ReadBytes != 56 || counters.WriteBytes != 78 {
		t.Fatal("known physical command, connection or storage evidence lost", counters)
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
