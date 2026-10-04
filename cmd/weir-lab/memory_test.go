package main

import "testing"

func TestFixtureMemoryCoversActualBackendConcurrency(t *testing.T) {
	opts := labOptions{concurrency: 64}
	backends := []string{"mongo", "search"}
	workspace, processMiB, err := fixtureMemoryBudget(opts, backends, 32)
	if err != nil {
		t.Fatal(err)
	}
	if workspace["mongo"]<<20 < 32*((32<<20)+(128<<10)+(8<<20)) || workspace["search"] < 32*96 {
		t.Fatal("workspace silently throttles the configured backend concurrency", workspace)
	}
	reservedMiB := 64 + 64/4 + 64*96 + workspace["mongo"] + workspace["search"] + 2*(64+32*2)
	if processMiB < reservedMiB || processMiB != 12288 {
		t.Fatal("ingress and both backend envelopes do not fit the process", processMiB, reservedMiB)
	}
	opts.workingMemoryMiB = 384
	workspace, processMiB, err = fixtureMemoryBudget(opts, backends, 32)
	if err != nil || workspace["mongo"] != 384 || workspace["search"] != 384 || processMiB != 8192 {
		t.Fatal("explicit workspace budget was not honored", workspace, processMiB, err)
	}
	opts.workingMemoryMiB = 0
	opts.concurrency = 512
	workspace, processMiB, err = fixtureMemoryBudget(opts, backends, 32)
	if err != nil || workspace["mongo"] != 1312 || workspace["search"] != 3072 || processMiB != 55296 {
		t.Fatal("512 independent requests do not fit the explicitly declared ingress envelope", workspace, processMiB, err)
	}
	if _, _, err := fixtureMemoryBudget(opts, backends, int(^uint(0)>>1)); err == nil {
		t.Fatal("overflowing execution envelope accepted")
	}
	opts.luaMutations = true
	workspace, _, err = fixtureMemoryBudget(opts, backends, 32)
	if err != nil || workspace["mongo"] != 32*64 || workspace["search"] != 32*96 {
		t.Fatal("Lua transaction scratch silently reduced requested backend capacity", workspace, err)
	}
}
