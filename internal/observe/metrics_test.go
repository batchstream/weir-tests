package observe

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestMetricSnapshotSelectsStoreAndRejectsUnavailableEvidence(t *testing.T) {
	raw := "# TYPE operations counter\noperations{store=\"mongo\",kind=\"execution\"} 112\noperations{kind=\"execution\",store=\"search\"} 8\nescaped{label=\"a\\\"b\\\\c\\nd\"} 1\n"
	metrics, err := Parse(raw)
	if err != nil {
		t.Fatal(err)
	}
	before := Snapshot{Metrics: metrics}
	raw = strings.Replace(raw, "112", "144", 1)
	metrics, err = Parse(raw)
	if err != nil {
		t.Fatal(err)
	}
	after := Snapshot{Metrics: metrics}
	labels := map[string]string{"store": "mongo"}
	value, err := Delta(before, after, "operations", labels)
	if err != nil || value != 32 {
		t.Fatalf("timed store delta=%v err=%v", value, err)
	}
	if metrics[2].Labels["label"] != "a\"b\\c\nd" {
		t.Fatal("escaped labels changed")
	}
	_, err = Delta(after, before, "operations", labels)
	if err == nil {
		t.Fatal("counter reset became a valid zero")
	}
	_, err = Delta(before, after, "absent", labels)
	if err == nil {
		t.Fatal("missing metric became a valid zero")
	}
	after.Error = "monitor failed"
	_, err = Delta(before, after, "operations", labels)
	if err == nil {
		t.Fatal("failed snapshot became a valid delta")
	}
}

func TestMetricsRejectMalformedResponse(t *testing.T) {
	for _, raw := range []string{"", "not a metric", "a NaN", "a +Inf", `a{k="a",k="b"} 1`, `a{k="a",} 1`, `a{k="a" x="b"} 1`} {
		if _, err := Parse(raw); err == nil {
			t.Fatalf("accepted invalid metrics %q", raw)
		}
	}
}

func TestFetchOnlyExplicitFixtureMetricsAndReportsHTTPFailures(t *testing.T) {
	paths := make(chan string, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { paths <- r.URL.Path; w.Write([]byte("a 1\n")) }))
	defer server.Close()
	address := strings.TrimPrefix(server.URL, "http://")
	snapshot := Fetch(t.Context(), address)
	path := <-paths
	if snapshot.Error != "" || snapshot.Raw != "a 1\n" || path != "/metrics" {
		t.Fatalf("snapshot=%+v path=%q", snapshot, path)
	}
	for _, address := range []string{"example.com:80", "192.0.2.1:80", "127.0.0.1", "127.0.0.1:0", "127.0.0.1:80/other", ""} {
		if snapshot := Fetch(t.Context(), address); snapshot.Error == "" {
			t.Fatal("accepted undisclosed deployment", address)
		}
	}
	failed := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusServiceUnavailable) }))
	defer failed.Close()
	if snapshot := Fetch(t.Context(), strings.TrimPrefix(failed.URL, "http://")); snapshot.Error == "" {
		t.Fatal("HTTP failure became empty metric set")
	}
	slow := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { <-r.Context().Done() }))
	defer slow.Close()
	ctx, cancel := context.WithTimeout(t.Context(), 20*time.Millisecond)
	defer cancel()
	if snapshot := Fetch(ctx, strings.TrimPrefix(slow.URL, "http://")); snapshot.Error == "" {
		t.Fatal("canceled scrape became valid")
	}
}
