package workload

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
)

type DatabaseCounters struct {
	NetworkIn          uint64            `json:"network_in_bytes"`
	NetworkOut         uint64            `json:"network_out_bytes"`
	ReadBytes          uint64            `json:"storage_read_bytes"`
	WriteBytes         uint64            `json:"storage_write_bytes"`
	IOScope            string            `json:"io_scope"`
	StorageUnavailable string            `json:"storage_observation_unavailable,omitempty"`
	Connections        *uint64           `json:"current_connections,omitempty"`
	Commands           map[string]uint64 `json:"physical_command_totals,omitempty"`
	CommandUnavailable string            `json:"physical_command_observation_unavailable,omitempty"`
	SearchNodeStats    json.RawMessage   `json:"search_node_stats,omitempty"`
	SearchIndexStats   json.RawMessage   `json:"search_index_stats,omitempty"`
	SearchSettings     json.RawMessage   `json:"search_index_settings,omitempty"`
	SearchUnavailable  string            `json:"search_pressure_observation_unavailable,omitempty"`
	// ProcessCPUNS is the database process's cumulative CPU time from its own API.
	ProcessCPUNS *int64 `json:"process_cpu_ns,omitempty"`
}

const searchNodeProjection = "nodes.*.fs.io_stats.total.read_kilobytes,nodes.*.fs.io_stats.total.write_kilobytes,nodes.*.fs.io_stats.total.operations,nodes.*.fs.io_stats.total.io_time_in_millis,nodes.*.transport.rx_size_in_bytes,nodes.*.transport.tx_size_in_bytes,nodes.*.http.current_open,nodes.*.thread_pool.write,nodes.*.thread_pool.write_coordination,nodes.*.indexing_pressure.memory,nodes.*.jvm.gc.collectors,nodes.*.jvm.mem.heap_used_in_bytes,nodes.*.jvm.mem.heap_max_in_bytes,nodes.*.process.cpu.total_in_millis"

type mongoCommandCounter struct {
	Total *uint64 `bson:"total"`
}

type mongoServerStatus struct {
	Connections struct {
		Current *uint64 `bson:"current"`
	} `bson:"connections"`
	Metrics struct {
		Commands struct {
			Find        mongoCommandCounter `bson:"find"`
			Update      mongoCommandCounter `bson:"update"`
			BulkWrite   mongoCommandCounter `bson:"bulkWrite"`
			GetMore     mongoCommandCounter `bson:"getMore"`
			KillCursors mongoCommandCounter `bson:"killCursors"`
			Commit      mongoCommandCounter `bson:"commitTransaction"`
			Abort       mongoCommandCounter `bson:"abortTransaction"`
		} `bson:"commands"`
	} `bson:"metrics"`
	Network struct {
		In  uint64 `bson:"bytesIn"`
		Out uint64 `bson:"bytesOut"`
	} `bson:"network"`
	WiredTiger struct {
		Cache struct {
			Read    uint64 `bson:"bytes read into cache"`
			Written uint64 `bson:"bytes written from cache"`
		} `bson:"cache"`
	} `bson:"wiredTiger"`
	// Linux mongod reports its cumulative process CPU time here.
	ExtraInfo struct {
		User   *int64 `bson:"user_time_us"`
		System *int64 `bson:"system_time_us"`
	} `bson:"extra_info"`
}

func (s mongoServerStatus) counters() DatabaseCounters {
	counters := DatabaseCounters{NetworkIn: s.Network.In, NetworkOut: s.Network.Out, ReadBytes: s.WiredTiger.Cache.Read, WriteBytes: s.WiredTiger.Cache.Written, Connections: s.Connections.Current, Commands: make(map[string]uint64)}
	commands := s.Metrics.Commands
	observed := map[string]*uint64{"find": commands.Find.Total, "update": commands.Update.Total, "bulkWrite": commands.BulkWrite.Total, "getMore": commands.GetMore.Total, "killCursors": commands.KillCursors.Total, "commitTransaction": commands.Commit.Total, "abortTransaction": commands.Abort.Total}
	for _, name := range []string{"find", "update", "bulkWrite", "getMore", "killCursors", "commitTransaction", "abortTransaction"} {
		total := observed[name]
		if total == nil {
			counters.CommandUnavailable += "missing " + name + "; "
			continue
		}
		counters.Commands[name] = *total
	}
	counters.IOScope = "MongoDB WiredTiger cache storage bytes; excludes other files and does not measure device busy time"
	if s.ExtraInfo.User != nil && s.ExtraInfo.System != nil {
		cpu := (*s.ExtraInfo.User + *s.ExtraInfo.System) * int64(time.Microsecond)
		counters.ProcessCPUNS = &cpu
	}
	return counters
}

// DatabaseCounters reads cumulative evidence, including the monitor's own
// traffic. Storage counters measure bytes and never imply disk saturation.
func (p *Paths) DatabaseCounters(ctx context.Context) (DatabaseCounters, error) {
	var counters DatabaseCounters
	if p.direct.mongo != nil {
		var reply mongoServerStatus
		command := bson.D{{Key: "serverStatus", Value: 1}}
		if err := p.direct.mongo.Database("admin").RunCommand(ctx, command).Decode(&reply); err != nil {
			return counters, err
		}
		return reply.counters(), nil
	}
	return SearchDatabaseCounters(ctx, p.direct.search, p.direct.baseURL)
}

// SearchDatabaseCounters projects fixed scalar or fixed-schema pressure fields;
// it never reads the unbounded HTTP client history or arbitrary index settings.
func SearchDatabaseCounters(ctx context.Context, client *http.Client, baseURL string) (DatabaseCounters, error) {
	var counters DatabaseCounters
	if client == nil || baseURL == "" {
		return counters, errors.New("owned Search monitoring client and endpoint required")
	}
	// HTTP client history grows as each independent process opens new pools.
	// Project only the scalar counters used by the monitor, never that history.
	path := "/_nodes/stats/fs,transport,http,thread_pool,indexing_pressure,jvm,process?filter_path=" + searchNodeProjection
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, baseURL+path, nil)
	if err != nil {
		return counters, err
	}
	response, err := client.Do(request)
	if err != nil {
		return counters, err
	}
	defer response.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(response.Body, (256<<10)+1))
	if err != nil || len(raw) > 256<<10 {
		return counters, errors.New("projected Search node statistics unavailable or exceed 256 KiB")
	}
	var reply struct {
		Nodes map[string]struct {
			ThreadPool       map[string]json.RawMessage `json:"thread_pool"`
			IndexingPressure struct {
				Memory json.RawMessage `json:"memory"`
			} `json:"indexing_pressure"`
			HTTP struct {
				CurrentOpen *uint64 `json:"current_open"`
			} `json:"http"`
			Process struct {
				CPU struct {
					Total *int64 `json:"total_in_millis"`
				} `json:"cpu"`
			} `json:"process"`
			Transport struct {
				RX *uint64 `json:"rx_size_in_bytes"`
				TX *uint64 `json:"tx_size_in_bytes"`
			} `json:"transport"`
			FS struct {
				IO struct {
					Total struct {
						Read  *uint64 `json:"read_kilobytes"`
						Write *uint64 `json:"write_kilobytes"`
					} `json:"total"`
				} `json:"io_stats"`
			} `json:"fs"`
		} `json:"nodes"`
	}
	if response.StatusCode != http.StatusOK || json.Unmarshal(raw, &reply) != nil || len(reply.Nodes) != 1 {
		return counters, errors.New("resource evidence requires exactly one Elasticsearch node")
	}
	for _, node := range reply.Nodes {
		if node.HTTP.CurrentOpen == nil || node.Transport.RX == nil || node.Transport.TX == nil {
			return counters, errors.New("Elasticsearch projected connection or transport counter unavailable")
		}
		counters.Connections = node.HTTP.CurrentOpen
		if node.Process.CPU.Total != nil {
			cpu := *node.Process.CPU.Total * int64(time.Millisecond)
			counters.ProcessCPUNS = &cpu
		}
		counters.NetworkIn, counters.NetworkOut = *node.Transport.RX, *node.Transport.TX
		var unavailable []string
		if node.FS.IO.Total.Read != nil {
			counters.ReadBytes = *node.FS.IO.Total.Read * 1024
		} else {
			unavailable = append(unavailable, "Elasticsearch filesystem read bytes unavailable")
		}
		if node.FS.IO.Total.Write != nil {
			counters.WriteBytes = *node.FS.IO.Total.Write * 1024
		} else {
			unavailable = append(unavailable, "Elasticsearch filesystem write bytes unavailable")
		}
		counters.StorageUnavailable = strings.Join(unavailable, "; ")
		if len(node.ThreadPool["write"]) == 0 || string(node.ThreadPool["write"]) == "null" {
			counters.SearchUnavailable = "Elasticsearch write thread pool unavailable; "
		} else {
			var write map[string]*uint64
			if json.Unmarshal(node.ThreadPool["write"], &write) != nil || write["threads"] == nil || write["active"] == nil || write["queue"] == nil || write["rejected"] == nil || write["completed"] == nil {
				counters.SearchUnavailable = "Elasticsearch write thread pool counters incomplete; "
			}
		}
		if len(node.IndexingPressure.Memory) == 0 || string(node.IndexingPressure.Memory) == "null" {
			counters.SearchUnavailable += "Elasticsearch indexing pressure memory unavailable"
		} else {
			var pressure struct {
				Current map[string]*uint64 `json:"current"`
				Total   map[string]*uint64 `json:"total"`
				Limit   *uint64            `json:"limit_in_bytes"`
			}
			if json.Unmarshal(node.IndexingPressure.Memory, &pressure) != nil || pressure.Current["all_in_bytes"] == nil || pressure.Total["coordinating_rejections"] == nil || pressure.Total["primary_rejections"] == nil || pressure.Total["replica_rejections"] == nil || pressure.Limit == nil {
				counters.SearchUnavailable += "Elasticsearch indexing pressure counters incomplete"
			}
		}
	}
	counters.SearchNodeStats = raw
	counters.IOScope = "Elasticsearch host device I/O bytes where available, including other processes; transport network excludes client HTTP traffic; Docker network is the complete container measurement"
	counters.CommandUnavailable = "Elasticsearch node statistics do not count physical client HTTP commands; adapter invocation counts are separate"
	return counters, nil
}

// DatabaseSnapshot adds owned-index evidence at stage boundaries only. The
// per-second monitor uses DatabaseCounters and does not re-read settings.
func (p *Paths) DatabaseSnapshot(ctx context.Context) (DatabaseCounters, error) {
	counters, err := p.DatabaseCounters(ctx)
	if err != nil || p.direct.mongo != nil {
		return counters, err
	}
	index := p.direct.dataset.Config.Namespace
	status, raw, err := p.direct.request(ctx, http.MethodGet, "/"+index+"/_settings?include_defaults=true&flat_settings=true", nil)
	if err != nil || status != http.StatusOK {
		return counters, errors.New("owned Search index settings unavailable")
	}
	var settings map[string]struct {
		Settings map[string]string `json:"settings"`
	}
	if json.Unmarshal(raw, &settings) != nil || len(settings) != 1 {
		return counters, errors.New("owned Search index settings identity unavailable")
	}
	observed, found := settings[index]
	if !found || observed.Settings["index.refresh_interval"] != "1s" || observed.Settings["index.translog.durability"] != "request" || observed.Settings["index.number_of_shards"] != "1" || observed.Settings["index.number_of_replicas"] != "0" {
		return counters, errors.New("owned Search effective refresh, durability or shard policy differs")
	}
	counters.SearchSettings = raw
	status, raw, err = p.direct.request(ctx, http.MethodGet, "/"+index+"/_stats/indexing,get,search,refresh,flush,merge,translog?level=indices", nil)
	if err != nil || status != http.StatusOK {
		return counters, errors.New("owned Search index statistics unavailable")
	}
	var stats struct {
		Shards struct {
			Failed *int `json:"failed"`
		} `json:"_shards"`
		Indices map[string]json.RawMessage `json:"indices"`
	}
	if json.Unmarshal(raw, &stats) != nil || stats.Shards.Failed == nil || *stats.Shards.Failed != 0 || len(stats.Indices) != 1 || len(stats.Indices[index]) == 0 || string(stats.Indices[index]) == "null" {
		return counters, errors.New("owned Search index statistics identity or shard failure differs")
	}
	counters.SearchIndexStats = raw
	return counters, nil
}
