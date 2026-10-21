package main

import (
	"encoding/json"
	"runtime"
	"testing"
	"time"
)

func TestCPUProfileReceiptRequiresExplicitFalseSourceDirty(t *testing.T) {
	target := runtime.GOOS + "/" + runtime.GOARCH
	for _, raw := range []string{`{}`, `{"source_dirty":null}`, `{"source_dirty":true}`, `{"source_dirty":false}`} {
		t.Run(raw, func(t *testing.T) {
			var receipt struct {
				SourceDirty *bool `json:"source_dirty"`
			}
			if err := json.Unmarshal([]byte(raw), &receipt); err != nil {
				t.Fatal(err)
			}
			got := cleanCPUProfileReceipt(receipt.SourceDirty, target, target)
			if got != (raw == `{"source_dirty":false}`) {
				t.Fatalf("receipt cleanliness accepted=%t for %s", got, raw)
			}
		})
	}
	clean := false
	if cleanCPUProfileReceipt(&clean, target, "wrong/target") || cleanCPUProfileReceipt(&clean, "wrong/target", "wrong/target") {
		t.Fatal("non-native or mismatched receipt target accepted")
	}
}

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
