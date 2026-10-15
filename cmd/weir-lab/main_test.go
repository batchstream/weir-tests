package main

import (
	"context"
	"strings"
	"testing"
	"time"
)

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
