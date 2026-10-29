package main

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestLabVersionPinsCheckedInLock(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join("..", "..", "versions.json"))
	if err != nil {
		t.Fatal(err)
	}
	pins, err := parseLabVersionPins(raw)
	if err != nil {
		t.Fatal(err)
	}
	var entries map[string]json.RawMessage
	if err := json.Unmarshal(raw, &entries); err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"weir_source", "mongodb_image", "elasticsearch_image"} {
		var expected string
		if err := json.Unmarshal(entries[key], &expected); err != nil || expected == "" || pins[key] != expected {
			t.Fatalf("required pin %s was lost or changed: %v", key, err)
		}
		original := entries[key]
		invalids := []string{`null`, `""`, `{}`, `[]`, `7`, `true`}
		if key == "weir_source" {
			invalids = append(invalids, `"main"`, `"b0687cc450fb"`, `"B0687CC450FB0133A0B2E0592A5058EC797BA47F"`)
		}
		for _, invalid := range invalids {
			entries[key] = json.RawMessage(invalid)
			changed, err := json.Marshal(entries)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := parseLabVersionPins(changed); err == nil {
				t.Fatalf("accepted invalid required pin %s=%s", key, invalid)
			}
		}
		delete(entries, key)
		changed, err := json.Marshal(entries)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := parseLabVersionPins(changed); err == nil {
			t.Fatalf("accepted missing required pin %s", key)
		}
		entries[key] = original
	}
}

func TestClientModulesReportTheLinkedSDKAndProtocol(t *testing.T) {
	modules := clientModules()
	for _, key := range []string{"sdk", "protocol"} {
		if !strings.HasPrefix(modules[key], "v") {
			t.Fatal(key, modules)
		}
	}
}

func TestConcurrencyAndBatchListsRejectEmptyDuplicateOrInvalidEntries(t *testing.T) {
	for _, raw := range []string{"", "1,", "0,8", "8,8", "8,-1", "many"} {
		if _, err := parsePositiveList(raw); err == nil {
			t.Fatal("invalid matrix accepted", raw)
		}
	}
	values, err := parsePositiveList("8, 32,128")
	if err != nil || len(values) != 3 || values[1] != 32 {
		t.Fatal("valid concurrency ladder rejected", values, err)
	}
}

func TestSingleAdmissionBoundsBeforeBinaryLookup(t *testing.T) {
	opts := labOptions{mode: "saturation", backend: "search", databaseCPUs: 1,
		levels: "1024,2048", batches: "1", clientProcesses: 4, duration: 180 * time.Second,
		warmupDuration: 5 * time.Minute, operations: 1, records: 8192, payload: 1,
		rounds: 1, writePercent: 100, binary: filepath.Join(t.TempDir(), "missing-weir")}
	err := runLab(t.Context(), opts)
	if err == nil || !strings.Contains(err.Error(), "prepare the locked Weir executable") {
		t.Fatal("valid bounded single-request ladder was rejected or started external work", err)
	}
	opts.levels = "2052"
	err = runLab(t.Context(), opts)
	if err == nil || !strings.Contains(err.Error(), "strictly increase through 2048") {
		t.Fatal("unbounded client concurrency passed validation", err)
	}
	opts.levels, opts.clientProcesses = "1024,2048", 1
	err = runLab(t.Context(), opts)
	if err == nil || !strings.Contains(err.Error(), "at most 1024 workers per client process") {
		t.Fatal("unbounded per-process concurrency reached binary lookup", err)
	}
	opts.clientProcesses, opts.warmupDuration = 4, 5*time.Minute+time.Second
	err = runLab(t.Context(), opts)
	if err == nil || !strings.Contains(err.Error(), "invalid saturation matrix or durations") {
		t.Fatal("excessive warmup passed validation", err)
	}
}

func TestBatchOverridesRejectInvalidControlsBeforeStartingFixtures(t *testing.T) {
	cases := []labOptions{
		{backendBatchBytes: -1},
		{backendBatchBytes: 33 << 20},
		{backendBatchWait: -time.Second},
		{backendBatchWait: 2 * time.Second},
		{backendInFlight: -1},
		{backendInFlight: 4097},
		{backendBatchBytes: 1024},
		{backendBatchWait: time.Millisecond},
		{backendInFlight: 4},
	}
	for _, opts := range cases {
		opts.mode, opts.databaseCPUs = "fixed", 1
		err := runLab(context.Background(), opts)
		if err == nil || !strings.Contains(err.Error(), "batch byte, wait and concurrency") {
			t.Fatal("invalid override reached binary lookup or fixture startup", opts, err)
		}
	}
}
