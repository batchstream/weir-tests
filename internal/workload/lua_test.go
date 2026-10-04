package workload

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
)

func TestNativeSearchTransformConditionsWriteOnObservedVersion(t *testing.T) {
	dataset := testDataset(t, "search")
	dataset.Config.LuaMutations = true
	operation := Operation{Record: 2, Write: true, Revision: 1}
	var reads, writes atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.Method == http.MethodPost {
			reads.Add(1)
			var body struct {
				IDs []string `json:"ids"`
			}
			if json.NewDecoder(request.Body).Decode(&body) != nil || len(body.IDs) != 1 || body.IDs[0] != dataset.ID(operation.Record) || request.URL.Path != "/"+dataset.Config.Namespace+"/_mget" || request.URL.Query().Get("realtime") != "true" {
				t.Error("native transform did not issue exactly one real-time point read")
			}
			previous := operation
			previous.Revision = 0
			fmt.Fprintf(writer, `{"docs":[{"_index":%q,"_id":%q,"found":true,"_seq_no":7,"_primary_term":2,"_source":%s}]}`, dataset.Config.Namespace, dataset.ID(operation.Record), dataset.Document(previous))
			return
		}
		writes.Add(1)
		if request.Method != http.MethodPut || request.URL.Query().Get("if_seq_no") != "7" || request.URL.Query().Get("if_primary_term") != "2" || request.URL.Query().Get("pipeline") != "_none" || request.URL.Query().Get("refresh") != "false" || request.URL.Query().Get("wait_for_active_shards") != "1" || request.URL.Query().Get("timeout") != "1s" {
			t.Error("native transform lost its version or acknowledgement policy")
		}
		raw, err := io.ReadAll(request.Body)
		if err != nil || dataset.Validate(raw, operation) != nil {
			t.Error("native transform changed business document shape or produced the wrong revision", err)
		}
		fmt.Fprintf(writer, `{"_index":%q,"_id":%q,"result":"updated","_shards":{"successful":1,"failed":0}}`, dataset.Config.Namespace, dataset.ID(operation.Record))
	}))
	defer server.Close()
	path := &directPath{dataset: dataset, search: server.Client(), baseURL: server.URL}
	result := path.Execute(t.Context(), operation)
	if result.Status != Success || !result.Applied || reads.Load() != 1 || writes.Load() != 1 {
		t.Fatal("native RMW was not one read and one acknowledged conditional write", result, reads.Load(), writes.Load())
	}
}

func TestNativeSearchTransformLostWriteAcknowledgementIsNotReplayed(t *testing.T) {
	dataset := testDataset(t, "search")
	dataset.Config.LuaMutations = true
	operation := Operation{Record: 2, Write: true, Revision: 1}
	var reads, writes atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.Method == http.MethodPost {
			reads.Add(1)
			previous := operation
			previous.Revision = 0
			fmt.Fprintf(writer, `{"docs":[{"_index":%q,"_id":%q,"found":true,"_seq_no":7,"_primary_term":2,"_source":%s}]}`, dataset.Config.Namespace, dataset.ID(operation.Record), dataset.Document(previous))
			return
		}
		writes.Add(1)
		_, _ = io.Copy(io.Discard, request.Body)
		connection, _, err := writer.(http.Hijacker).Hijack()
		if err != nil {
			t.Error(err)
			return
		}
		_ = connection.Close()
	}))
	defer server.Close()
	path := &directPath{dataset: dataset, search: server.Client(), baseURL: server.URL}
	result := path.Execute(t.Context(), operation)
	if result.Status != Indeterminate || result.Applied || reads.Load() != 1 || writes.Load() != 1 {
		t.Fatal("lost write acknowledgement invented success or replayed RMW", result, reads.Load(), writes.Load())
	}
}

func TestLuaComparisonRejectsClientBatching(t *testing.T) {
	dataset := testDataset(t, "search")
	dataset.Config.LuaMutations = true
	first := Operation{Record: 1, Write: true, Revision: 1}
	second := Operation{Record: 2, Write: true, Revision: 1}
	operations := []Operation{first, second}
	direct := &directPath{dataset: dataset}
	sdk := &weirPath{dataset: dataset}
	for _, executor := range []Executor{direct, sdk} {
		for _, result := range executor.ExecuteBatch(t.Context(), operations) {
			if result.Status != Failed || result.Applied {
				t.Fatal("Lua comparison allowed client batching", result)
			}
		}
	}
}
