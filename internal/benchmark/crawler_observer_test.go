package benchmark

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestCrawlerObserverCountsCompleteAndAbortedBodiesAndRethrows(t *testing.T) {
	handler := http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		_, _ = io.Copy(io.Discard, request.Body)
		if request.URL.Query().Get("abort") == "true" {
			hijacker, ok := writer.(http.Hijacker)
			if !ok {
				t.Error("upstream cannot expose a truncated response")
				return
			}
			connection, buffer, err := hijacker.Hijack()
			if err != nil {
				t.Error(err)
				return
			}
			_, _ = fmt.Fprint(buffer, "HTTP/1.1 200 OK\r\nContent-Length: 100\r\nContent-Type: application/json\r\n\r\npartial")
			_ = buffer.Flush()
			_ = connection.Close()
			return
		}
		_, _ = io.WriteString(writer, "complete")
	})
	upstream := httptest.NewServer(handler)
	defer upstream.Close()
	observer, err := startCrawlerObserver(upstream.URL)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := observer.close(t.Context()); err != nil {
			t.Error(err)
		}
	}()
	payload := "request-body"
	request := httptest.NewRequest(http.MethodPost, observer.URL+"/_bulk", strings.NewReader(payload))
	recorder := httptest.NewRecorder()
	observer.server.Handler.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK || recorder.Body.String() != "complete" {
		t.Fatal("complete response changed", recorder.Code, recorder.Body.String())
	}
	request = httptest.NewRequest(http.MethodPost, observer.URL+"/_bulk?abort=true", strings.NewReader(payload))
	request = request.WithContext(context.WithValue(request.Context(), http.ServerContextKey, observer.server))
	recorder = httptest.NewRecorder()
	var recovered any
	func() { defer func() { recovered = recover() }(); observer.server.Handler.ServeHTTP(recorder, request) }()
	if recovered != http.ErrAbortHandler {
		t.Fatal("observer swallowed or changed the upstream body-copy panic", recovered)
	}
	counters := observer.snapshot()
	counter := counters["POST _bulk"]
	if counter.Requests != 2 || counter.RequestBytes != uint64(2*len(payload)) || counter.ResponseBytes != uint64(len("completepartial")) || counter.Failed != 1 || counter.Aborted != 1 {
		t.Fatal("aborted request or partial bytes disappeared from the ledger", counter)
	}
	before := map[string]crawlerHTTPCount{}
	delta := crawlerHTTPDelta(before, counters)
	if delta["POST _bulk"].Aborted != 1 {
		t.Fatal("aborted counter missing from timed delta", delta)
	}
}

func TestCrawlerDisabledObserverRequiresExplicitUnavailableLedger(t *testing.T) {
	stage := crawlerStage{}
	if err := auditCrawlerHTTPObservation(stage, "disabled"); err != nil {
		t.Fatal("explicit no-observer control required a ledger", err)
	}
	for _, mode := range []string{"", "enabled", "unexpected"} {
		if err := auditCrawlerHTTPObservation(stage, mode); err == nil {
			t.Fatal("missing or invalid mode silently bypassed the observer audit", mode)
		}
	}
	stage.HTTPBefore = map[string]crawlerHTTPCount{}
	stage.HTTPAfter = map[string]crawlerHTTPCount{}
	stage.HTTPDelta = map[string]crawlerHTTPCount{}
	if err := auditCrawlerHTTPObservation(stage, "enabled"); err != nil {
		t.Fatal(err)
	}
	if err := auditCrawlerHTTPObservation(stage, "disabled"); err == nil {
		t.Fatal("disabled observer published fabricated available counters")
	}
}
