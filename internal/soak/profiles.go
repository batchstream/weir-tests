package soak

import (
	"errors"
	"fmt"
	"slices"
	"strconv"
	"strings"
)

// The SDK calls the workload makes, as they appear in reports.
const (
	apiPut          = "put"
	apiPutVisible   = "put_visible"
	apiLua          = "lua"
	apiMutate       = "mutate"
	apiMutateStream = "mutate_stream"
	apiReadOne      = "read_one"
	apiRead         = "read"
	apiReadStream   = "read_stream"
	apiNative       = "native"
)

// A turn is one operation of a worker, or one stream it holds, and reports
// false once the run stops.
type turn func(w *worker, tokens <-chan struct{}) bool

// profile is how a worker uses the SDK. A stream lasts for many operations, so
// a worker that holds streams alternates between them; read percent would
// otherwise rarely open a read stream.
type profile struct {
	write, read       turn
	writeAPI, readAPI string
	alternate         bool
}

var profiles = map[string]profile{
	"single":  {write: (*worker).putTurn, read: (*worker).readOneTurn, writeAPI: apiPut, readAPI: apiReadOne},
	"visible": {write: (*worker).visibleTurn, read: (*worker).readOneTurn, writeAPI: apiPutVisible, readAPI: apiReadOne},
	"lua":     {write: (*worker).luaTurn, read: (*worker).readOneTurn, writeAPI: apiLua, readAPI: apiReadOne},
	"batch":   {write: (*worker).mutateTurn, read: (*worker).readBatchTurn, writeAPI: apiMutate, readAPI: apiRead},
	"stream":  {write: (*worker).mutateStreamTurn, read: (*worker).readStreamTurn, writeAPI: apiMutateStream, readAPI: apiReadStream, alternate: true},
	"native":  {write: (*worker).putTurn, read: (*worker).nativeTurn, writeAPI: apiPut, readAPI: apiNative},
}

// DefaultProfiles exercises every SDK call, most workers using the calls most
// applications make.
const DefaultProfiles = "single=40,visible=10,lua=15,batch=15,stream=15,native=5"

// ProfileShare is the share of a backend's workers that use one profile.
type ProfileShare struct {
	Name   string `json:"name"`
	Weight int    `json:"weight"`
}

// ParseProfiles reads "name=weight,..." in the order given.
func ParseProfiles(spec string) ([]ProfileShare, error) {
	var shares []ProfileShare
	for entry := range strings.SplitSeq(spec, ",") {
		name, value, found := strings.Cut(strings.TrimSpace(entry), "=")
		weight, err := strconv.Atoi(value)
		if !found || err != nil || weight < 1 {
			return nil, fmt.Errorf("profile %q needs a positive weight, as in single=40", entry)
		}
		if _, known := profiles[name]; !known {
			return nil, fmt.Errorf("unknown profile %q", name)
		}
		if slices.ContainsFunc(shares, func(share ProfileShare) bool { return share.Name == name }) {
			return nil, fmt.Errorf("profile %q appears twice", name)
		}
		share := ProfileShare{Name: name, Weight: weight}
		shares = append(shares, share)
	}
	if len(shares) == 0 {
		return nil, errors.New("at least one profile is required")
	}
	return shares, nil
}

// profileOf spreads workers across the shares in proportion to their weights.
func profileOf(shares []ProfileShare, index, workers int) string {
	total := 0
	for _, share := range shares {
		total += share.Weight
	}
	position := index * total / workers
	for _, share := range shares {
		if position < share.Weight {
			return share.Name
		}
		position -= share.Weight
	}
	return shares[len(shares)-1].Name
}

// checkShares reports a profile that the given workers are too few to reach.
func checkShares(shares []ProfileShare, workers int) error {
	reached := make(map[string]bool, len(shares))
	for index := range workers {
		reached[profileOf(shares, index, workers)] = true
	}
	for _, share := range shares {
		if !reached[share.Name] {
			return fmt.Errorf("%d workers are too few to give profile %s any", workers, share.Name)
		}
	}
	return nil
}

// untestedCalls lists the SDK calls the configured profiles and read percent
// should exercise that never succeeded, so a run cannot pass without them.
func untestedCalls(shares []ProfileShare, readPercent int, totals Counters) []string {
	var missing []string
	for _, share := range shares {
		chosen := profiles[share.Name]
		if written := totals.APIs[chosen.writeAPI]; readPercent < 100 && written.Applied == 0 && !slices.Contains(missing, chosen.writeAPI) {
			missing = append(missing, chosen.writeAPI)
		}
		if read := totals.APIs[chosen.readAPI]; readPercent > 0 && read.Reads <= read.ReadFailures && !slices.Contains(missing, chosen.readAPI) {
			missing = append(missing, chosen.readAPI)
		}
	}
	return missing
}
