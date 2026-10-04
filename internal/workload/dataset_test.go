package workload

import (
	"bytes"
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
		for _, operation := range operations {
			if owner, exists := owners[operation.Record]; exists && owner != worker {
				t.Fatal("record shared by workers")
			}
			owners[operation.Record] = worker
			if operation.Write {
				actual[operation.Record] = 1 - actual[operation.Record]
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

func TestRepeatedWritesChangePayloadAndReadsFollowLatestWrite(t *testing.T) {
	for _, backend := range []string{"mongo", "search"} {
		t.Run(backend, func(t *testing.T) {
			dataset := testDataset(t, backend)
			opts := PlanOptions{Operations: 1003, WritePercent: 70}
			directPlan, err := dataset.Plan(opts)
			if err != nil {
				t.Fatal(err)
			}
			weirPlan, err := dataset.Plan(opts)
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(directPlan, weirPlan) {
				t.Fatal("direct and Weir offered different operation plans")
			}
			latest := make([][]byte, dataset.Config.Records)
			writes := make([]int, dataset.Config.Records)
			for record := range latest {
				seed := Operation{Record: record}
				latest[record] = dataset.Document(seed)
			}
			repeated, readsAfterWrite := 0, 0
			for _, operations := range directPlan.Workers {
				for _, operation := range operations {
					body := dataset.Document(operation)
					if operation.Write {
						if bytes.Equal(body, latest[operation.Record]) {
							t.Fatal("replacement repeated the persisted payload instead of changing it")
						}
						if writes[operation.Record] > 0 {
							repeated++
						}
						writes[operation.Record]++
						latest[operation.Record] = body
					} else {
						if !bytes.Equal(body, latest[operation.Record]) {
							t.Fatal("read expectation differs from most recent write")
						}
						if writes[operation.Record] > 0 {
							readsAfterWrite++
						}
					}
				}
			}
			if repeated == 0 || readsAfterWrite == 0 {
				t.Fatal("regression did not exercise repeated writes and dependent reads")
			}
			for record, revision := range directPlan.Expected {
				final := Operation{Record: record, Revision: revision}
				if !bytes.Equal(dataset.Document(final), latest[record]) {
					t.Fatal("postflight expectation differs from final persisted payload")
				}
			}
		})
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
