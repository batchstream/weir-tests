package benchmark

import (
	"testing"
	"time"
)

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
