package workload

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
)

func TestNativeSearchBulkKeepsDistinctAcknowledgementsAndDocumentOrder(t *testing.T) {
	dataset := testDataset(t, "search")
	operations := []Operation{{Record: 2, Write: true, Revision: 1}, {Record: 4, Write: true, Revision: 1}, {Record: 6, Write: true, Revision: 1}}
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodPost {
			t.Error("bulk operation must use one POST")
		}
		if request.URL.Path == "/"+dataset.Config.Namespace+"/_bulk" {
			query := request.URL.Query()
			if query.Get("pipeline") != "_none" || query.Get("refresh") != "false" || query.Get("wait_for_active_shards") != "1" || query.Get("timeout") != "1s" {
				t.Error("bulk mutation policy mismatch", query)
			}
			raw, err := io.ReadAll(request.Body)
			lines := strings.Split(strings.TrimSuffix(string(raw), "\n"), "\n")
			if err != nil || len(lines) != len(operations)*2 {
				t.Error("bulk must include one descriptor and document per input", err)
			}
			fmt.Fprint(writer, `{"items":[`)
			for index, operation := range operations {
				if index > 0 {
					fmt.Fprint(writer, ",")
				}
				fmt.Fprintf(writer, `{"index":{"_index":%q,"_id":%q,"status":201,"result":"created","_shards":{"successful":1,"failed":0}}}`, dataset.Config.Namespace, dataset.ID(operation.Record))
			}
			fmt.Fprint(writer, "]}")
			return
		}
		if request.URL.Path != "/"+dataset.Config.Namespace+"/_mget" || request.URL.Query().Get("realtime") != "true" {
			t.Error("matching bulk read policy missing")
		}
		var body struct {
			IDs []string `json:"ids"`
		}
		if json.NewDecoder(request.Body).Decode(&body) != nil || len(body.IDs) != len(operations) {
			t.Error("bulk IDs missing")
		}
		fmt.Fprint(writer, `{"docs":[`)
		for index, operation := range operations {
			if index > 0 {
				fmt.Fprint(writer, ",")
			}
			fmt.Fprintf(writer, `{"_index":%q,"_id":%q,"found":true,"_source":%s}`, dataset.Config.Namespace, dataset.ID(operation.Record), dataset.Document(operation))
		}
		fmt.Fprint(writer, "]}")
	}))
	defer server.Close()
	path := &directPath{dataset: dataset, search: server.Client(), baseURL: server.URL}
	for _, outcome := range path.ExecuteBatch(context.Background(), operations) {
		if outcome.Status != Success || !outcome.Applied || outcome.RequestBytes == 0 {
			t.Fatal("bulk acknowledgement lost", outcome)
		}
	}
	for index := range operations {
		operations[index].Write = false
	}
	for _, outcome := range path.ExecuteBatch(context.Background(), operations) {
		if outcome.Status != Success || outcome.ResponseBytes == 0 {
			t.Fatal("bulk documents failed validation", outcome)
		}
	}
}

func TestNativeSearchBulkFailureNeverBecomesSuccessfulThroughputOrRetry(t *testing.T) {
	dataset := testDataset(t, "search")
	operations := []Operation{{Record: 2, Write: true, Revision: 1}, {Record: 4, Write: true, Revision: 1}}
	var calls atomic.Int64
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		calls.Add(1)
		writer.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer server.Close()
	path := &directPath{dataset: dataset, search: server.Client(), baseURL: server.URL}
	outcomes := path.ExecuteBatch(context.Background(), operations)
	if len(outcomes) != 2 || calls.Load() != 1 {
		t.Fatal("bulk failure was replayed or lost input accounting")
	}
	for _, outcome := range outcomes {
		if outcome.Status != Indeterminate || outcome.Applied || outcome.RequestBytes != 0 {
			t.Fatal("unconfirmed bulk write counted as successful", outcome)
		}
	}
	operations[1].Record = operations[0].Record
	path.ExecuteBatch(context.Background(), operations)
	if calls.Load() != 1 {
		t.Fatal("duplicate batch reached backend")
	}
}
