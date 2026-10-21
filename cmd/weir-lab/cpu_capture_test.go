package main

import (
	"testing"
	"time"
)

func TestValidateCPUCapture(t *testing.T) {
	normal := labOptions{}
	if err := validateCPUCapture(normal, nil); err != nil {
		t.Fatal(err)
	}
	valid := labOptions{mode: "saturation", backend: "mongo", serverReceipt: "candidate.receipt.json", cpuProfileConcurrency: 32, cpuProfileDuration: 60 * time.Second, warmupDuration: 10 * time.Second, duration: 20 * time.Second}
	levels := []int{4, 8, 32, 128}
	if err := validateCPUCapture(valid, levels); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"bulk", "fixed", "all", "no receipt", "missing level", "duration only", "missing duration", "too long", "negative", "no headroom"} {
		t.Run(name, func(t *testing.T) {
			opts := valid
			switch name {
			case "bulk":
				opts.mode = "bulk-saturation"
			case "fixed":
				opts.mode = "fixed"
			case "all":
				opts.backend = "all"
			case "no receipt":
				opts.serverReceipt = ""
			case "missing level":
				opts.cpuProfileConcurrency = 16
			case "duration only":
				opts.cpuProfileConcurrency = 0
			case "missing duration":
				opts.cpuProfileDuration = 0
			case "too long":
				opts.cpuProfileDuration = 61 * time.Second
			case "negative":
				opts.cpuProfileDuration = -time.Second
			case "no headroom":
				opts.duration = 50 * time.Second
			}
			if err := validateCPUCapture(opts, levels); err == nil {
				t.Fatal("invalid capture accepted")
			}
		})
	}
}
