package workload

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
)

func TestNativeSearchTransformUsesOneAtomicServerScript(t *testing.T) {
	dataset := testDataset(t, "search")
	dataset.Config.LuaMutations = true
	operation := Operation{Record: 2, Write: true, Revision: 1}
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		calls.Add(1)
		if request.Method != http.MethodPost || request.URL.Path != "/"+dataset.Config.Namespace+"/_update/"+dataset.ID(operation.Record) || request.URL.Query().Get("retry_on_conflict") != "0" || request.URL.Query().Get("refresh") != "false" || request.URL.Query().Get("wait_for_active_shards") != "1" || request.URL.Query().Get("timeout") != "1s" {
			t.Error("native transform lost its single-update or acknowledgement policy")
		}
		var body struct {
			Script struct {
				Lang   string `json:"lang"`
				Source string `json:"source"`
			} `json:"script"`
		}
		if json.NewDecoder(request.Body).Decode(&body) != nil || body.Script.Lang != "painless" || body.Script.Source != "ctx._source.revision = 1 - ctx._source.revision" {
			t.Error("native transform did not compute the revision from the server document")
		}
		fmt.Fprintf(writer, `{"_index":%q,"_id":%q,"result":"updated","_shards":{"successful":1,"failed":0}}`, dataset.Config.Namespace, dataset.ID(operation.Record))
	}))
	defer server.Close()
	path := &directPath{dataset: dataset, search: server.Client(), baseURL: server.URL}
	result := path.Execute(t.Context(), operation)
	if result.Status != Success || !result.Applied || calls.Load() != 1 || result.RequestBytes != 0 || result.ResponseBytes != 0 {
		t.Fatal("native RMW was not one acknowledged atomic update without document transfer", result, calls.Load())
	}
}

func TestNativeSearchTransformLostAcknowledgementIsNotReplayed(t *testing.T) {
	dataset := testDataset(t, "search")
	dataset.Config.LuaMutations = true
	operation := Operation{Record: 2, Write: true, Revision: 1}
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		calls.Add(1)
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
	if result.Status != Indeterminate || result.Applied || calls.Load() != 1 {
		t.Fatal("lost acknowledgement invented success or replayed the toggle", result, calls.Load())
	}
}

func TestNativeSearchTransformRequiresCompleteAcknowledgement(t *testing.T) {
	cases := []struct {
		name   string
		status int
		body   string
		want   Status
	}{
		{name: "conflict", status: http.StatusConflict, body: `{}`, want: Failed},
		{name: "missing", status: http.StatusNotFound, body: `{}`, want: Failed},
		{name: "server failure", status: http.StatusServiceUnavailable, body: `{}`, want: Indeterminate},
		{name: "wrong record", status: http.StatusOK, body: `{"_index":"%s","_id":"other","result":"updated","_shards":{"successful":1,"failed":0}}`, want: Indeterminate},
		{name: "noop", status: http.StatusOK, body: `{"_index":"%s","_id":"record-00000002","result":"noop","_shards":{"successful":1,"failed":0}}`, want: Indeterminate},
		{name: "shard failure", status: http.StatusOK, body: `{"_index":"%s","_id":"record-00000002","result":"updated","_shards":{"successful":0,"failed":1}}`, want: Indeterminate},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			dataset := testDataset(t, "search")
			dataset.Config.LuaMutations = true
			var calls atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
				calls.Add(1)
				writer.WriteHeader(test.status)
				fmt.Fprint(writer, strings.ReplaceAll(test.body, "%s", dataset.Config.Namespace))
			}))
			defer server.Close()
			path := &directPath{dataset: dataset, search: server.Client(), baseURL: server.URL}
			operation := Operation{Record: 2, Write: true, Revision: 1}
			result := path.Execute(t.Context(), operation)
			if result.Status != test.want || result.Applied || calls.Load() != 1 {
				t.Fatal("failed update acknowledgement was accepted or replayed", result, calls.Load())
			}
		})
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

func TestLuaComparisonUsesOrdinarySetupAndBatchedIndependentPostflightReads(t *testing.T) {
	dataset := testDataset(t, "search")
	dataset.Config.LuaMutations = true
	var reads, seeds atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path == "/"+dataset.Config.Namespace+"/_bulk" {
			seeds.Add(1)
			raw, err := io.ReadAll(request.Body)
			lines := strings.Split(strings.TrimSpace(string(raw)), "\n")
			if err != nil || len(lines) != 2*dataset.Config.Records {
				t.Error("fixture reset was not an ordinary replacement batch", err)
			}
			items := make([]string, dataset.Config.Records)
			for index := range items {
				items[index] = fmt.Sprintf(`{"index":{"_index":%q,"_id":%q,"status":201,"result":"created","_shards":{"successful":1,"failed":0}}}`, dataset.Config.Namespace, dataset.ID(index))
			}
			fmt.Fprintf(writer, `{"items":[%s]}`, strings.Join(items, ","))
			return
		}
		if request.URL.Path == "/"+dataset.Config.Namespace+"/_refresh" {
			fmt.Fprint(writer, `{}`)
			return
		}
		if request.URL.Path == "/"+dataset.Config.Namespace+"/_count" {
			fmt.Fprintf(writer, `{"count":%d}`, dataset.Config.Records)
			return
		}
		reads.Add(1)
		var body struct {
			IDs []string `json:"ids"`
		}
		if request.Method != http.MethodPost || json.NewDecoder(request.Body).Decode(&body) != nil || len(body.IDs) != dataset.Config.Records {
			t.Error("postflight did not read the complete fixture in one batch")
		}
		docs := make([]string, dataset.Config.Records)
		for index := range docs {
			operation := Operation{Record: index}
			docs[index] = fmt.Sprintf(`{"_index":%q,"_id":%q,"found":true,"_source":%s}`, dataset.Config.Namespace, dataset.ID(index), dataset.Document(operation))
		}
		fmt.Fprintf(writer, `{"docs":[%s]}`, strings.Join(docs, ","))
	}))
	defer server.Close()
	direct := &directPath{dataset: dataset, search: server.Client(), baseURL: server.URL}
	paths := &Paths{Direct: direct, direct: direct, owned: true}
	if err := paths.Prepare(t.Context(), 64); err != nil {
		t.Fatal("Lua guard blocked ordinary fixture setup or untimed postflight reads", err)
	}
	if reads.Load() != 1 || seeds.Load() != 1 {
		t.Fatal("setup entered timed RMW or postflight lost read batching", reads.Load(), seeds.Load())
	}
}
