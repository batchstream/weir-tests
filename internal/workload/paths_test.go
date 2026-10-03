package workload

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
)

func TestSearchDirectUsesMatchingPolicyAndNeverReplaysMutation(t *testing.T) {
	dataset := testDataset(t, "search")
	var requests atomic.Int64
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodPut || request.URL.Path != "/"+dataset.Config.Namespace+"/_doc/"+dataset.ID(2) {
			t.Error("wrong mutation resource", request.Method, request.URL.Path)
		}
		query := request.URL.Query()
		if query.Get("pipeline") != "_none" || query.Get("refresh") != "false" || query.Get("wait_for_active_shards") != "1" || query.Get("timeout") != "1s" {
			t.Error("write acknowledgement/visibility policy mismatch")
		}
		requests.Add(1)
		writer.WriteHeader(http.StatusServiceUnavailable)
		fmt.Fprint(writer, `{"error":"acknowledgement unavailable"}`)
	}))
	defer server.Close()
	path := &directPath{dataset: dataset, search: server.Client(), baseURL: server.URL}
	operation := Operation{Record: 2, Write: true, Revision: 1}
	result := path.Execute(context.Background(), operation)
	if result.Status != Indeterminate || requests.Load() != 1 {
		t.Fatal("mutation was retried or treated as successful", result, requests.Load())
	}
}

func TestSearchRequiresNativeAcknowledgementAndPersistedIdentity(t *testing.T) {
	dataset := testDataset(t, "search")
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.Method == http.MethodPut {
			fmt.Fprint(writer, `{"_index":"wrong-index","_id":"wrong-id","result":"updated","_shards":{"successful":1,"failed":0}}`)
			return
		}
		seed := Operation{Record: 2}
		if request.Method != http.MethodPost || request.URL.Path != "/"+dataset.Config.Namespace+"/_mget" || request.URL.Query().Get("realtime") != "true" {
			t.Error("mget read policy mismatch")
		}
		fmt.Fprintf(writer, `{"docs":[{"_index":%q,"_id":%q,"found":true,"_source":%s}]}`, dataset.Config.Namespace, dataset.ID(2), dataset.Document(seed))
	}))
	defer server.Close()
	path := &directPath{dataset: dataset, search: server.Client(), baseURL: server.URL}
	write := Operation{Record: 2, Write: true, Revision: 1}
	if result := path.Execute(context.Background(), write); result.Status != Indeterminate {
		t.Fatal("accepted unrelated acknowledgement", result)
	}
	read := Operation{Record: 2}
	if result := path.Execute(context.Background(), read); result.Status != Success || result.ResponseBytes == 0 {
		t.Fatal("rejected valid persisted source", result)
	}
	read.Revision = 1
	if result := path.Execute(context.Background(), read); result.Status != Failed {
		t.Fatal("accepted stale read as successful")
	}
}

func TestSearchReadDoesNotReplayOnReusedConnectionFailure(t *testing.T) {
	dataset := testDataset(t, "search")
	var requests atomic.Int64
	var firstRemote atomic.Value
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodPost || request.URL.Path != "/"+dataset.Config.Namespace+"/_mget" || request.Header.Get("Idempotency-Key") != "" {
			t.Error("read must be non-replayable single-ID POST")
		}
		var query struct {
			IDs []string `json:"ids"`
		}
		if err := json.NewDecoder(request.Body).Decode(&query); err != nil || len(query.IDs) != 1 || query.IDs[0] != dataset.ID(2) {
			t.Error("read did not offer the expected single-ID logical workload")
		}
		call := requests.Add(1)
		if call == 1 {
			firstRemote.Store(request.RemoteAddr)
		}
		if call == 2 {
			if request.RemoteAddr != firstRemote.Load().(string) {
				t.Error("regression did not exercise a reused connection")
			}
			connection, _, err := writer.(http.Hijacker).Hijack()
			if err != nil {
				t.Error(err)
				return
			}
			connection.Close()
			return
		}
		seed := Operation{Record: 2}
		fmt.Fprintf(writer, `{"docs":[{"_index":%q,"_id":%q,"found":true,"_source":%s}]}`, dataset.Config.Namespace, dataset.ID(2), dataset.Document(seed))
	}))
	defer server.Close()
	transport := &http.Transport{MaxConnsPerHost: 1, MaxIdleConnsPerHost: 1}
	client := &http.Client{Transport: transport}
	defer client.CloseIdleConnections()
	path := &directPath{dataset: dataset, search: client, baseURL: server.URL}
	read := Operation{Record: 2}
	if result := path.Execute(context.Background(), read); result.Status != Success {
		t.Fatal("initial read failed", result)
	}
	result := path.Execute(context.Background(), read)
	if result.Status != Failed || requests.Load() != 2 {
		t.Fatal("broken reused connection was silently replayed and counted as successful", result, requests.Load())
	}
}

func TestSearchMgetRequiresExactlyOneMatchingDocument(t *testing.T) {
	dataset := testDataset(t, "search")
	for _, body := range []string{`{"docs":[]}`, `{"docs":[{},{}]}`, `{"docs":[{"_index":"wrong","_id":"wrong","found":true,"_source":{}}]}`} {
		t.Run(body, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) { fmt.Fprint(writer, body) }))
			defer server.Close()
			path := &directPath{dataset: dataset, search: server.Client(), baseURL: server.URL}
			read := Operation{Record: 2}
			if result := path.Execute(context.Background(), read); result.Status != Failed {
				t.Fatal("invalid mget response was counted as success", result)
			}
		})
	}
}

func TestOpenPreservesOwnedCleanupFailureAfterSDKInitializationFails(t *testing.T) {
	dataset := testDataset(t, "search")
	var deletes atomic.Int64
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		switch request.Method {
		case http.MethodGet:
			fmt.Fprint(writer, `{"version":{"number":"8.19.22"}}`)
		case http.MethodPut:
			fmt.Fprint(writer, `{"acknowledged":true}`)
		case http.MethodDelete:
			deletes.Add(1)
			writer.WriteHeader(http.StatusServiceUnavailable)
		}
	}))
	defer server.Close()
	dataset.Config.SearchURL = server.URL
	dataset.Config.WeirSeed = "invalid-initialization-address"
	paths, err := Open(context.Background(), dataset)
	if paths != nil || err == nil || !strings.Contains(err.Error(), "initialize Weir") || !strings.Contains(err.Error(), "owned Search index cleanup failed") || deletes.Load() != 1 {
		t.Fatal("initialization hid cleanup failure", paths, err, deletes.Load())
	}
}
