package benchmark

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
)

func TestCrawlerResetChunksCompleteFixtureWithinByteBudget(t *testing.T) {
	var mutex sync.Mutex
	var requests, totalBytes int
	seen := map[string]bool{}
	handler := http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		raw, err := io.ReadAll(io.LimitReader(request.Body, crawlerResetBulkBytes+1))
		if err != nil {
			t.Error(err)
			return
		}
		if len(raw) > crawlerResetBulkBytes || request.URL.Query().Get("refresh") != "true" {
			t.Error("reset bulk exceeded metadata-inclusive budget or lost refresh barrier", len(raw))
			http.Error(writer, "invalid request", http.StatusBadRequest)
			return
		}
		mutex.Lock()
		defer mutex.Unlock()
		requests++
		totalBytes += len(raw)
		decoder := json.NewDecoder(bytes.NewReader(raw))
		var items []any
		for decoder.More() {
			var metadata struct {
				Index struct {
					ID    string `json:"_id"`
					Index string `json:"_index"`
				} `json:"index"`
			}
			if err := decoder.Decode(&metadata); err != nil {
				t.Error(err)
				return
			}
			var doc struct {
				UID     string `json:"uid"`
				Padding string `json:"benchmark_padding"`
			}
			if err := decoder.Decode(&doc); err != nil {
				t.Error(err)
				return
			}
			key := metadata.Index.Index + "/" + metadata.Index.ID
			if seen[key] || doc.UID != metadata.Index.ID {
				t.Error("reset duplicated or changed a document", key, doc.UID)
			}
			padding := 0
			if metadata.Index.Index == "fixture-products" {
				padding = 8192
			}
			if len(doc.Padding) != padding {
				t.Error("reset did not preserve the declared product payload", key, len(doc.Padding))
			}
			seen[key] = true
			shards := map[string]any{"successful": 1, "failed": 0}
			item := map[string]any{"_index": metadata.Index.Index, "_id": metadata.Index.ID, "status": 201, "result": "created", "_shards": shards}
			action := map[string]any{"index": item}
			items = append(items, action)
		}
		reply := map[string]any{"errors": false, "items": items}
		_ = json.NewEncoder(writer).Encode(reply)
	})
	server := httptest.NewServer(handler)
	defer server.Close()
	opts := crawlerResetOptions{URL: server.URL, Namespace: "fixture", Workers: 32, PageSize: 20, SeedPaddingBytes: 8192}
	if err := resetCrawler(t.Context(), server.Client(), opts); err != nil {
		t.Fatal(err)
	}
	mutex.Lock()
	defer mutex.Unlock()
	if requests < 2 || totalBytes <= crawlerResetBulkBytes || len(seen) != opts.Workers*opts.PageSize*6+1 {
		t.Fatal("reset did not preserve the full fixture across bounded chunks", requests, totalBytes, len(seen))
	}
}

func TestCrawlerResetRejectsIncompleteOrFailedItemAcknowledgements(t *testing.T) {
	for _, failure := range []string{"count", "identity", "status", "shard", "errors"} {
		t.Run(failure, func(t *testing.T) {
			shards := map[string]any{"successful": 1, "failed": 0}
			item := map[string]any{"_index": "fixture-products", "_id": "record", "status": 201, "result": "created", "_shards": shards}
			action := map[string]any{"index": item}
			items := []any{action}
			reply := map[string]any{"errors": false, "items": items}
			switch failure {
			case "count":
				reply["items"] = []any{}
			case "identity":
				item["_id"] = "wrong"
			case "status":
				item["status"] = 429
			case "shard":
				shards["failed"] = 1
			case "errors":
				reply["errors"] = true
			}
			handler := http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) { _ = json.NewEncoder(writer).Encode(reply) })
			server := httptest.NewServer(handler)
			defer server.Close()
			record := crawlerResetIdentity{ID: "record", Index: "fixture-products"}
			opts := crawlerResetBulkOptions{URL: server.URL, Body: []byte("{}\n{}\n"), Records: []crawlerResetIdentity{record}}
			if err := resetCrawlerBulk(t.Context(), server.Client(), opts); err == nil {
				t.Fatal("invalid reset acknowledgement was accepted", failure)
			}
		})
	}
}

func TestCrawlerModesBuildOnlySelectedCases(t *testing.T) {
	for _, mode := range []string{"fixed-rate", "saturation", "both", "none"} {
		t.Run(mode, func(t *testing.T) {
			opts := CrawlerOptions{Profiles: []string{"two-step"}, Rates: []int{20, 80}, Concurrency: []int{4, 8}}
			switch mode {
			case "fixed-rate", "saturation":
				opts.Modes = []string{mode}
			case "none":
				opts.Rates = nil
			}
			cases, err := crawlerCases(opts)
			if err != nil {
				t.Fatal(err)
			}
			want := 2
			if mode == "both" {
				want = 4
			}
			if len(cases) != want {
				t.Fatal("wrong selected matrix size", cases)
			}
			for _, test := range cases {
				if mode == "fixed-rate" && (test.Mode != "fixed-rate" || test.Workers != 8 || test.Rate == 0) || mode == "saturation" && (test.Mode != "saturation" || test.Rate != 0) || mode == "none" && test.Mode != "saturation" {
					t.Fatal("unselected mode leaked into the matrix", test)
				}
			}
		})
	}
	opts := CrawlerOptions{Profiles: []string{"two-step"}, Concurrency: []int{4}, Modes: []string{"fixed-rate"}}
	if _, err := crawlerCases(opts); err == nil {
		t.Fatal("an empty fixed-rate experiment was accepted")
	}
	opts.Modes = []string{"saturation", "saturation"}
	if _, err := crawlerCases(opts); err == nil {
		t.Fatal("duplicate modes were accepted")
	}
}

func TestCrawlerAuditUsesExactSelectedModeMatrix(t *testing.T) {
	source := strings.Repeat("1", 40)
	digest := strings.Repeat("2", 64)
	identity := map[string]any{"revision": source}
	receipt := map[string]any{"source": source, "sha256": digest, "source_dirty": false, "identity": identity}
	raw, _ := json.Marshal(receipt)
	server := crawlerServerReceipt{SHA256: digest, Receipt: raw}
	for _, mode := range []string{"fixed-rate", "saturation"} {
		t.Run(mode, func(t *testing.T) {
			opts := CrawlerOptions{Modes: []string{mode}, Profiles: []string{"two-step"}, Rates: []int{20}, Concurrency: []int{4, 8}, PageSize: 1, Processes: 1, Rounds: 1}
			count := 3
			if mode == "saturation" {
				count = 6
			}
			stages := make([]crawlerStage, count)
			workers := 8
			rate := 20
			if mode == "saturation" {
				workers = 4
				rate = 0
			}
			stages[0] = crawlerStage{Path: "direct", Mode: mode, Profile: "two-step", Round: 1, Concurrency: workers, ProductsPerSecond: rate}
			servers := map[string]crawlerServerReceipt{"current": server, "optimized": server}
			report := &CrawlerReport{Schema: crawlerReportSchema, Sources: crawlerSources(), Servers: servers, Parameters: opts, Stages: stages}
			err := AuditCrawlerReport(report)
			if err == nil || !strings.Contains(err.Error(), "unqualified") {
				t.Fatal("audit did not use the exact selected matrix", err)
			}
			extra := crawlerStage{}
			report.Stages = append(report.Stages, extra)
			err = AuditCrawlerReport(report)
			if err == nil || !strings.Contains(err.Error(), "matrix is incomplete") {
				t.Fatal("audit accepted an extra mode/stage", err)
			}
		})
	}
}
