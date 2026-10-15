package benchmark

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"reflect"
	"time"
)

type crawlerSettleOptions struct {
	URL, Namespace       string
	Timeout, Quiet, Poll time.Duration
}
type crawlerSettleSample struct {
	At        string          `json:"at_utc"`
	ElapsedNS int64           `json:"elapsed_ns"`
	Stats     json.RawMessage `json:"index_stats,omitempty"`
	Error     string          `json:"error,omitempty"`
}
type crawlerSettling struct {
	Started                            string `json:"started_at_utc"`
	TimeoutNS, RequiredQuietNS, PollNS int64
	WaitNS                             int64                 `json:"wait_ns"`
	Samples                            []crawlerSettleSample `json:"samples"`
	Settled                            bool                  `json:"settled"`
}

// waitCrawlerSettled excludes observable reset indexing/merge/flush activity
// without changing segments or replacing the database between paths.
func waitCrawlerSettled(ctx context.Context, client *http.Client, opts crawlerSettleOptions) (result crawlerSettling, resultErr error) {
	if opts.Timeout == 0 {
		opts.Timeout = 30 * time.Second
	}
	if opts.Quiet == 0 {
		opts.Quiet = 2 * time.Second
	}
	if opts.Poll == 0 {
		opts.Poll = time.Second
	}
	started := time.Now()
	result = crawlerSettling{Started: started.UTC().Format(time.RFC3339Nano), TimeoutNS: int64(opts.Timeout), RequiredQuietNS: int64(opts.Quiet), PollNS: int64(opts.Poll)}
	defer func() { result.WaitNS = time.Since(started).Nanoseconds() }()
	if opts.Timeout <= 0 || opts.Quiet <= 0 || opts.Poll <= 0 || opts.Quiet > opts.Timeout {
		return result, errors.New("invalid bounded crawler settling policy")
	}
	bounded, cancel := context.WithTimeout(ctx, opts.Timeout)
	defer cancel()
	var quietSince time.Time
	var previous map[string]uint64
	for {
		raw, err := crawlerStats(bounded, client, opts.URL, opts.Namespace)
		observed := time.Now()
		sample := crawlerSettleSample{At: observed.UTC().Format(time.RFC3339Nano), ElapsedNS: observed.Sub(started).Nanoseconds(), Stats: raw}
		if err != nil {
			sample.Error = err.Error()
			result.Samples = append(result.Samples, sample)
			return result, fmt.Errorf("crawler reset did not settle: %w", err)
		}
		counters, idle, err := crawlerIdleCounters(raw, opts.Namespace)
		if err != nil {
			sample.Error = err.Error()
			result.Samples = append(result.Samples, sample)
			return result, err
		}
		result.Samples = append(result.Samples, sample)
		if idle {
			if quietSince.IsZero() || !reflect.DeepEqual(previous, counters) {
				quietSince = observed
			}
			previous = counters
			if observed.Sub(quietSince) >= opts.Quiet {
				result.Settled = true
				return result, nil
			}
		} else {
			quietSince = time.Time{}
			previous = nil
		}
		timer := time.NewTimer(opts.Poll)
		select {
		case <-timer.C:
		case <-bounded.Done():
			timer.Stop()
			return result, fmt.Errorf("crawler reset did not settle within %s: %w", opts.Timeout, bounded.Err())
		}
	}
}

func crawlerIdleCounters(raw []byte, namespace string) (map[string]uint64, bool, error) {
	var response struct {
		Shards struct {
			Failed int `json:"failed"`
		} `json:"_shards"`
		Indices map[string]struct {
			Primaries struct {
				Indexing struct {
					Current *uint64 `json:"index_current"`
				} `json:"indexing"`
				Merges struct {
					Current   *uint64 `json:"current"`
					Total     *uint64 `json:"total"`
					Time      *uint64 `json:"total_time_in_millis"`
					Documents *uint64 `json:"total_docs"`
					Bytes     *uint64 `json:"total_size_in_bytes"`
				} `json:"merges"`
				Flush struct {
					Total *uint64 `json:"total"`
					Time  *uint64 `json:"total_time_in_millis"`
				} `json:"flush"`
			} `json:"primaries"`
		} `json:"indices"`
	}
	if err := json.Unmarshal(raw, &response); err != nil {
		return nil, false, err
	}
	if response.Shards.Failed != 0 || len(response.Indices) != 2 {
		return nil, false, errors.New("crawler settling requires complete statistics for both owned indices")
	}
	counters := map[string]uint64{}
	idle := true
	for _, product := range []bool{true, false} {
		index := crawlerIndex(namespace, product)
		value, ok := response.Indices[index]
		if !ok {
			return nil, false, errors.New("crawler settling index identity mismatch")
		}
		stats := value.Primaries
		observed := map[string]*uint64{"index_current": stats.Indexing.Current, "merge_current": stats.Merges.Current, "merge_total": stats.Merges.Total, "merge_ms": stats.Merges.Time, "merge_documents": stats.Merges.Documents, "merge_bytes": stats.Merges.Bytes, "flush_total": stats.Flush.Total, "flush_ms": stats.Flush.Time}
		for name, counter := range observed {
			if counter == nil {
				return nil, false, fmt.Errorf("crawler settling missing %s/%s", index, name)
			}
			if name == "index_current" || name == "merge_current" {
				idle = idle && *counter == 0
				continue
			}
			counters[index+"/"+name] = *counter
		}
	}
	return counters, idle, nil
}
