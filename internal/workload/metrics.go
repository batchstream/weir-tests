package workload

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"

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
}

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
	// HTTP client history grows as each independent process opens new pools.
	// Project only the scalar counters used by the monitor, never that history.
	projection := "nodes.*.fs.io_stats.total.read_kilobytes,nodes.*.fs.io_stats.total.write_kilobytes,nodes.*.transport.rx_size_in_bytes,nodes.*.transport.tx_size_in_bytes,nodes.*.http.current_open"
	path := "/_nodes/stats/fs,transport,http?filter_path=" + projection
	status, raw, err := p.direct.request(ctx, http.MethodGet, path, nil)
	if err != nil {
		return counters, err
	}
	var reply struct {
		Nodes map[string]struct {
			HTTP struct {
				CurrentOpen *uint64 `json:"current_open"`
			} `json:"http"`
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
	if status != http.StatusOK || json.Unmarshal(raw, &reply) != nil || len(reply.Nodes) != 1 {
		return counters, errors.New("resource evidence requires exactly one Elasticsearch node")
	}
	for _, node := range reply.Nodes {
		if node.HTTP.CurrentOpen == nil || node.Transport.RX == nil || node.Transport.TX == nil {
			return counters, errors.New("Elasticsearch projected connection or transport counter unavailable")
		}
		counters.Connections = node.HTTP.CurrentOpen
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
	}
	counters.IOScope = "Elasticsearch host device I/O bytes where available, including other processes; transport network excludes client HTTP traffic; Docker network is the complete container measurement"
	counters.CommandUnavailable = "Elasticsearch node statistics do not count physical client HTTP commands; adapter invocation counts are separate"
	return counters, nil
}
