package workload

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"

	"go.mongodb.org/mongo-driver/v2/bson"
)

type DatabaseCounters struct {
	NetworkIn  uint64 `json:"network_in_bytes"`
	NetworkOut uint64 `json:"network_out_bytes"`
	ReadBytes  uint64 `json:"storage_read_bytes"`
	WriteBytes uint64 `json:"storage_write_bytes"`
	IOScope    string `json:"io_scope"`
}

// DatabaseCounters reads cumulative evidence, including the monitor's own
// traffic. Storage counters measure bytes and never imply disk saturation.
func (p *Paths) DatabaseCounters(ctx context.Context) (DatabaseCounters, error) {
	var counters DatabaseCounters
	if p.direct.mongo != nil {
		var reply struct {
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
		command := bson.D{{Key: "serverStatus", Value: 1}}
		if err := p.direct.mongo.Database("admin").RunCommand(ctx, command).Decode(&reply); err != nil {
			return counters, err
		}
		counters.NetworkIn, counters.NetworkOut = reply.Network.In, reply.Network.Out
		counters.ReadBytes, counters.WriteBytes = reply.WiredTiger.Cache.Read, reply.WiredTiger.Cache.Written
		counters.IOScope = "MongoDB WiredTiger cache storage bytes; excludes other files and does not measure device busy time"
		return counters, nil
	}
	status, raw, err := p.direct.request(ctx, http.MethodGet, "/_nodes/stats/fs,transport", nil)
	if err != nil {
		return counters, err
	}
	var reply struct {
		Nodes map[string]struct {
			Transport struct {
				RX uint64 `json:"rx_size_in_bytes"`
				TX uint64 `json:"tx_size_in_bytes"`
			} `json:"transport"`
			FS struct {
				IO struct {
					Total struct {
						Read  uint64 `json:"read_kilobytes"`
						Write uint64 `json:"write_kilobytes"`
					} `json:"total"`
				} `json:"io_stats"`
			} `json:"fs"`
		} `json:"nodes"`
	}
	if status != http.StatusOK || json.Unmarshal(raw, &reply) != nil || len(reply.Nodes) != 1 {
		return counters, errors.New("resource evidence requires exactly one Elasticsearch node")
	}
	for _, node := range reply.Nodes {
		counters.NetworkIn, counters.NetworkOut = node.Transport.RX, node.Transport.TX
		counters.ReadBytes, counters.WriteBytes = node.FS.IO.Total.Read*1024, node.FS.IO.Total.Write*1024
	}
	counters.IOScope = "Elasticsearch node filesystem bytes where available; transport network excludes client HTTP traffic; Docker network is the complete container measurement"
	return counters, nil
}
