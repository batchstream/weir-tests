package benchmark

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"time"

	"github.com/batchstream/weir-tests/internal/fixture"
)

// CPUCapture diagnoses the first Weir stage at one explicitly selected concurrency.
// It is never a baseline: restarting and sampling add overhead and include startup.
type CPUCapture struct {
	Cluster      *fixture.Cluster
	Path         string
	Duration     time.Duration
	Concurrency  int
	used         bool
	launchBefore time.Time
	ready        time.Time
	timedStart   time.Time
	timedEnd     time.Time
	pid          int
}

type cpuCaptureReceipt struct {
	Classification   string            `json:"classification"`
	Profile          string            `json:"profile"`
	SHA256           string            `json:"profile_sha256"`
	PID              int               `json:"weir_pid"`
	Backend          string            `json:"backend"`
	Concurrency      int               `json:"concurrency"`
	Round            int               `json:"round"`
	Duration         string            `json:"requested_capture_duration"`
	LaunchBefore     time.Time         `json:"launch_before_utc"`
	Ready            time.Time         `json:"ready_after_utc"`
	TimedStart       time.Time         `json:"timed_start_utc"`
	TimedEnd         time.Time         `json:"timed_end_utc"`
	Joined           time.Time         `json:"process_stop_returned_utc"`
	StopJoined       bool              `json:"successful_stop_and_join"`
	MinimumOverlapNS int64             `json:"conservative_timed_overlap_ns"`
	Provenance       map[string]string `json:"provenance"`
	Error            string            `json:"error,omitempty"`
}

func (c *CPUCapture) begin(ctx context.Context) error {
	c.used = true
	if err := c.Cluster.StopNode(ctx, 0); err != nil {
		return err
	}
	c.launchBefore = time.Now().UTC()
	if err := c.Cluster.RestartNodeWithCPUProfile(ctx, c.Path, c.Duration); err != nil {
		return err
	}
	c.ready = time.Now().UTC()
	c.pid = c.Cluster.Nodes[0].ProcessID
	return nil
}

func conservativeOverlap(launch, start, end time.Time, duration time.Duration) time.Duration {
	if start.IsZero() || end.IsZero() {
		return 0
	}
	bound := launch.Add(duration)
	if end.After(bound) {
		end = bound
	}
	if !end.After(start) {
		return 0
	}
	return end.Sub(start)
}

func (c *CPUCapture) finish(options SaturationOptions, stageErr error) error {
	// Always join even after caller cancellation; Close remains the final cleanup owner.
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	stopErr := c.Cluster.StopNode(ctx, 0)
	joined := time.Now().UTC()
	overlap := conservativeOverlap(c.launchBefore, c.timedStart, c.timedEnd, c.Duration)
	if stopErr != nil {
		overlap = 0
	}
	err := errors.Join(stageErr, stopErr)
	if overlap < time.Second {
		err = errors.Join(err, errors.New("CPU capture did not establish at least 1s of timed Weir workload overlap"))
	}
	data, readErr := os.ReadFile(c.Path)
	err = errors.Join(err, readErr)
	if len(data) == 0 {
		err = errors.Join(err, errors.New("CPU capture file is empty"))
	}
	digest := sha256.Sum256(data)
	receipt := cpuCaptureReceipt{Classification: "diagnostic non-baseline; startup-inclusive CPU profiling and restart overhead; no optimization or causal claim", Profile: c.Path, SHA256: hex.EncodeToString(digest[:]), PID: c.pid, Backend: options.Dataset.Config.Backend, Concurrency: c.Concurrency, Round: 1, Duration: c.Duration.String(), LaunchBefore: c.launchBefore, Ready: c.ready, TimedStart: c.timedStart, TimedEnd: c.timedEnd, Joined: joined, StopJoined: stopErr == nil, MinimumOverlapNS: overlap.Nanoseconds(), Provenance: options.Provenance}
	if err != nil {
		receipt.Error = err.Error()
	}
	encoded, marshalErr := json.MarshalIndent(receipt, "", "  ")
	err = errors.Join(err, marshalErr)
	file, createErr := os.OpenFile(c.Path+".receipt.json", os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	err = errors.Join(err, createErr)
	if createErr == nil {
		_, writeErr := file.Write(append(encoded, '\n'))
		err = errors.Join(err, writeErr, file.Close())
	}
	if stopErr == nil {
		err = errors.Join(err, c.Cluster.RestartNode(ctx, 0))
	}
	return err
}
