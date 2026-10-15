package fixture

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestSearchReadinessWaitsForHealthBeforeRequiringTemplateAcknowledgement(t *testing.T) {
	cases := []struct {
		name         string
		firstStatus  int
		firstBody    string
		healthStatus string
		acknowledged bool
	}{
		{name: "transient 503 then yellow", firstStatus: http.StatusServiceUnavailable, firstBody: "starting", healthStatus: "yellow", acknowledged: true},
		{name: "red then green", firstStatus: http.StatusOK, firstBody: `{"timed_out":false,"status":"red","number_of_nodes":1}`, healthStatus: "green", acknowledged: true},
		{name: "template acknowledgement is still required", firstStatus: http.StatusServiceUnavailable, firstBody: "starting", healthStatus: "yellow"},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			var versions, health, templates atomic.Int32
			handler := http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
				switch request.URL.Path {
				case "/":
					versions.Add(1)
					_, _ = fmt.Fprintf(writer, `{"version":{"number":%q}}`, SearchVersion)
				case "/_cluster/health":
					if request.Method != http.MethodGet || request.URL.Query().Get("wait_for_status") != "yellow" || request.URL.Query().Get("timeout") != "2s" {
						t.Errorf("health request changed its readiness budget: %s %s", request.Method, request.URL)
					}
					if health.Add(1) == 1 {
						writer.WriteHeader(test.firstStatus)
						_, _ = fmt.Fprint(writer, test.firstBody)
						return
					}
					_, _ = fmt.Fprintf(writer, `{"timed_out":false,"status":%q,"number_of_nodes":1}`, test.healthStatus)
				case "/_index_template/weir-tests":
					templates.Add(1)
					if request.Method != http.MethodPut || health.Load() != 2 {
						t.Errorf("template published before singleton health: %s, health calls %d", request.Method, health.Load())
					}
					_, _ = fmt.Fprintf(writer, `{"acknowledged":%t}`, test.acknowledged)
				default:
					writer.WriteHeader(http.StatusNotFound)
				}
			})
			server := httptest.NewServer(handler)
			defer server.Close()
			cluster := &Cluster{SearchURL: server.URL}
			ctx, cancel := context.WithTimeout(t.Context(), 2*time.Second)
			defer cancel()
			err := cluster.readySearch(ctx)
			if test.acknowledged && err != nil {
				t.Fatal("transient health failure aborted readiness", err)
			}
			if !test.acknowledged && (err == nil || !strings.Contains(err.Error(), "not acknowledged")) {
				t.Fatal("readiness ignored an unacknowledged template", err)
			}
			if versions.Load() != 1 || health.Load() != 2 || templates.Load() != 1 {
				t.Fatal("unexpected readiness protocol", versions.Load(), health.Load(), templates.Load())
			}
		})
	}
}

func TestSearchHealthRetriesStopAtParentCancellation(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 2*time.Second)
	defer cancel()
	var health, templates atomic.Int32
	handler := http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		switch request.URL.Path {
		case "/":
			_, _ = fmt.Fprintf(writer, `{"version":{"number":%q}}`, SearchVersion)
		case "/_cluster/health":
			writer.WriteHeader(http.StatusServiceUnavailable)
			_, _ = fmt.Fprint(writer, "still starting")
			if health.Add(1) == 2 {
				cancel()
			}
		case "/_index_template/weir-tests":
			templates.Add(1)
			_, _ = fmt.Fprint(writer, `{"acknowledged":true}`)
		default:
			writer.WriteHeader(http.StatusNotFound)
		}
	})
	server := httptest.NewServer(handler)
	defer server.Close()
	cluster := &Cluster{SearchURL: server.URL}
	err := cluster.readySearch(ctx)
	if !errors.Is(err, context.Canceled) || ctx.Err() != context.Canceled {
		t.Fatal("permanent health failure did not stop at the parent context", err, ctx.Err())
	}
	if health.Load() != 2 || templates.Load() != 0 {
		t.Fatal("readiness failed to retry health or published a template before healthy", health.Load(), templates.Load())
	}
}
