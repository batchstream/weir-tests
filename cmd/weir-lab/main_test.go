package main

import "testing"

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
