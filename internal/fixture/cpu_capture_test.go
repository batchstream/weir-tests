package fixture

import (
	"context"
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestCPUProfileServeArgumentsAreExplicitAndTemporary(t *testing.T) {
	opts := Options{WeirBinary: "/unused/weir"}
	node := Node{}
	c := &Cluster{Directory: t.TempDir(), options: opts, Nodes: []Node{node}, processes: []*process{nil}}
	normal := c.nodeCommand(0).Args
	if strings.Contains(strings.Join(normal, " "), "cpu-profile") {
		t.Fatal("default node has profiling flags")
	}
	c.cpuProfilePath, c.cpuProfileDuration = "/unused/capture.pprof", 60*time.Second
	profiled := c.nodeCommand(0).Args
	want := append(append([]string{}, normal...), "--cpu-profile", "/unused/capture.pprof", "--cpu-profile-duration", "1m0s")
	if !reflect.DeepEqual(profiled, want) {
		t.Fatalf("serve args %q, want %q", profiled, want)
	}
	c.cpuProfilePath, c.cpuProfileDuration = "", 0
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := c.RestartNodeWithCPUProfile(ctx, "/unused/capture.pprof", time.Second); err == nil {
		t.Fatal("canceled restart accepted")
	}
	if !reflect.DeepEqual(c.nodeCommand(0).Args, normal) {
		t.Fatal("profile arguments leaked to normal restart")
	}
	for _, duration := range []time.Duration{-time.Second, 0, 61 * time.Second} {
		if err := c.RestartNodeWithCPUProfile(context.Background(), "/unused/capture.pprof", duration); err == nil {
			t.Fatal("invalid duration accepted")
		}
	}
}

func TestCPUProfileRestartRequiresJoinedProcess(t *testing.T) {
	opts := Options{WeirBinary: "/unused/weir"}
	node := Node{}
	running := &process{done: make(chan struct{})}
	c := &Cluster{options: opts, Nodes: []Node{node}, processes: []*process{running}}
	err := c.RestartNodeWithCPUProfile(context.Background(), "/unused/capture.pprof", time.Second)
	if err == nil || !strings.Contains(err.Error(), "still running") {
		t.Fatalf("unjoined process restart: %v", err)
	}
	if c.cpuProfilePath != "" {
		t.Fatal("failed restart retained profile arguments")
	}
}
