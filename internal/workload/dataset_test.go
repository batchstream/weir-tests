package workload

import (
	"encoding/json"
	"reflect"
	"testing"
)

func testDataset(t *testing.T, backend string) *Dataset {
	t.Helper()
	config := Config{Backend: backend, StoreName: backend, Namespace: "weirtest_0123456789abcdef01234567", Records: 37, PayloadBytes: 63, Concurrency: 5}
	dataset, err := New(config)
	if err != nil {
		t.Fatal(err)
	}
	return dataset
}

func TestDeterministicPlanOwnsWorkerIDsAndReadRevisions(t *testing.T) {
	dataset := testDataset(t, "mongo")
	opts := PlanOptions{Operations: 1003, WritePercent: 35}
	first, err := dataset.Plan(opts)
	if err != nil {
		t.Fatal(err)
	}
	second, err := dataset.Plan(opts)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(first, second) {
		t.Fatal("plans differ")
	}
	owners := make(map[int]int)
	actual := make([]int, dataset.Config.Records)
	reads, writes, count := 0, 0, 0
	for worker, operations := range first.Workers {
		for sequence, operation := range operations {
			if owner, exists := owners[operation.Record]; exists && owner != worker {
				t.Fatal("record shared by workers")
			}
			owners[operation.Record] = worker
			if operation.Worker != worker || operation.Sequence != sequence {
				t.Fatal("sequence metadata mismatch")
			}
			if operation.Write {
				actual[operation.Record] = 1
				writes++
			} else {
				reads++
			}
			if operation.Revision != actual[operation.Record] {
				t.Fatal("read expected revision cannot be reproduced sequentially")
			}
			if err := dataset.Validate(dataset.Document(operation), operation); err != nil {
				t.Fatal(err)
			}
			count++
		}
	}
	if count != opts.Operations || count != first.Count || reads != first.Reads || writes != first.Writes || !reflect.DeepEqual(actual, first.Expected) {
		t.Fatal("plan arithmetic mismatch")
	}
}

func TestPayloadValidationRejectsMissingFieldsAndWrongRevision(t *testing.T) {
	for _, backend := range []string{"mongo", "search"} {
		t.Run(backend, func(t *testing.T) {
			dataset := testDataset(t, backend)
			seed := Operation{Record: 2}
			updated := Operation{Record: 2, Revision: 1}
			if err := dataset.Validate(dataset.Document(seed), updated); err == nil {
				t.Fatal("accepted stale seed document")
			}
			if err := dataset.Validate([]byte(`{}`), seed); err == nil {
				t.Fatal("accepted malformed/missing document")
			}
			if backend == "search" {
				fields := map[string]any{"fixture_id": dataset.ID(2), "padding": dataset.padding}
				raw, err := json.Marshal(fields)
				if err != nil {
					t.Fatal(err)
				}
				if err := dataset.Validate(raw, seed); err == nil {
					t.Fatal("accepted missing revision zero")
				}
			}
		})
	}
}

func TestWorkloadBoundsAndOwnedNamespace(t *testing.T) {
	config := Config{Backend: "search", StoreName: "search", Namespace: "weirtest_0123456789abcdef01234567", Records: 100000, PayloadBytes: 1 << 20, Concurrency: 1}
	if _, err := New(config); err == nil {
		t.Fatal("accepted excessive precomputed workspace")
	}
	config.Records, config.PayloadBytes, config.Namespace = 1, 0, "production"
	if _, err := New(config); err == nil {
		t.Fatal("accepted unowned namespace")
	}
	first, err := NewNamespace()
	if err != nil {
		t.Fatal(err)
	}
	second, err := NewNamespace()
	if err != nil {
		t.Fatal(err)
	}
	if first == second || !namespacePattern.MatchString(first) {
		t.Fatal("namespace not fresh")
	}
	dataset := testDataset(t, "search")
	opts := PlanOptions{Operations: 1, WritePercent: 10}
	if _, err := dataset.Plan(opts); err == nil {
		t.Fatal("accepted fewer operations than workers")
	}
}
