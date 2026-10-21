package benchmark

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestCPUOverlapUsesLatestActualClientStartAndPreservesMonotonicTime(t *testing.T) {
	launch := time.Now()
	planned := launch.Add(59*time.Second - 50*time.Millisecond)
	end := planned.Add(20 * time.Second)
	clients := []ClientProcessResult{{StartLagNS: int64(20 * time.Millisecond)}, {StartLagNS: int64(100 * time.Millisecond)}, {StartLagNS: int64(40 * time.Millisecond)}}
	capture := CPUCapture{}
	capture.recordTiming(planned, end, clients)
	if capture.timedStart != planned.Add(100*time.Millisecond) || capture.timedEnd != end {
		t.Fatal("capture did not preserve monotonic times and maximum actual client start lag")
	}
	if got := conservativeOverlap(launch, capture.timedStart, capture.timedEnd, 60*time.Second); got >= time.Second {
		t.Fatalf("planned-only overlap falsely qualified: %s", got)
	}
	negative := []ClientProcessResult{{StartLagNS: -1}}
	capture.recordTiming(planned, end, negative)
	if capture.timedStart != planned {
		t.Fatal("negative lag moved workload before the planned barrier")
	}
}

func TestCPUFailureOrCancellationNeverContinues(t *testing.T) {
	ctx := context.Background()
	if !captureMayContinue(ctx, nil) {
		t.Fatal("successful capture cannot continue")
	}
	for _, message := range []string{"stage failure", "stop/join failure", "overlap failure", "profile read failure", "receipt write failure"} {
		if captureMayContinue(ctx, errors.New(message)) {
			t.Fatal("capture continued after", message)
		}
	}
	canceled, cancel := context.WithCancel(ctx)
	cancel()
	if captureMayContinue(canceled, nil) {
		t.Fatal("capture continued after caller cancellation")
	}
}

func TestConservativeCPUOverlap(t *testing.T) {
	launch := time.Unix(100, 0)
	tests := []struct {
		name  string
		start time.Time
		end   time.Time
		want  time.Duration
	}{
		{name: "timed workload", start: launch.Add(12 * time.Second), end: launch.Add(32 * time.Second), want: 20 * time.Second},
		{name: "capture expires mid-stage", start: launch.Add(50 * time.Second), end: launch.Add(70 * time.Second), want: 10 * time.Second},
		{name: "setup consumed capture", start: launch.Add(61 * time.Second), end: launch.Add(81 * time.Second)},
		{name: "warmup failed"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got := conservativeOverlap(launch, test.start, test.end, 60*time.Second)
			if got != test.want {
				t.Fatalf("overlap %s, want %s", got, test.want)
			}
		})
	}
}
