package acceptance

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"
)

type roundTrip func(*http.Request) (*http.Response, error)

func (fn roundTrip) RoundTrip(request *http.Request) (*http.Response, error) { return fn(request) }

func TestSearchSetupAndOracleUseOnlyDedicatedIndexWithoutNetwork(t *testing.T) {
	calls := 0
	client := searchClient()
	client.Transport = roundTrip(func(request *http.Request) (*http.Response, error) {
		checkDeadline(t, request.Context())
		calls++
		if request.URL.Host != "search.invalid:9200" || !strings.HasPrefix(request.URL.Path, "/weiraccept_run12345_nonce_s") {
			t.Fatal("request escaped dedicated target")
		}
		status := http.StatusOK
		body := ""
		switch calls {
		case 1:
			if request.Method != http.MethodHead {
				t.Fatal("missing fresh preflight")
			}
			status = http.StatusNotFound
		case 2:
			if request.Method != http.MethodPut {
				t.Fatal("missing owned create")
			}
			body = `{"acknowledged":true}`
		case 3:
			body = `{"found":true,"_index":"weiraccept_run12345_nonce_s","_id":"record","_source":{"owner":"run12345","value":1}}`
		case 4:
			body = `{"count":1,"_shards":{"total":1,"successful":1,"failed":0}}`
		default:
			t.Fatal("unexpected request/replay")
		}
		response := &http.Response{StatusCode: status, Body: io.NopCloser(strings.NewReader(body)), Header: make(http.Header)}
		return response, nil
	})
	backend := &searchBackend{base: "http://search.invalid:9200", name: "weiraccept_run12345_nonce_s", owner: "run12345", client: client}
	ctx, cancel := context.WithTimeout(context.Background(), callBudget)
	defer cancel()
	if err := backend.Prepare(ctx); err != nil {
		t.Fatal(err)
	}
	if err := backend.Oracle(ctx, 1); err != nil {
		t.Fatal(err)
	}
	if !backend.Owned() || calls != 4 {
		t.Fatal("setup/oracle sequence changed")
	}
}

func TestSearchDoesNotAdoptAnExistingIndex(t *testing.T) {
	client := searchClient()
	calls := 0
	client.Transport = roundTrip(func(request *http.Request) (*http.Response, error) {
		calls++
		response := &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader("")), Header: make(http.Header)}
		return response, nil
	})
	backend := &searchBackend{base: "http://search.invalid:9200", name: "weiraccept_run12345_nonce_s", client: client}
	ctx, cancel := context.WithTimeout(context.Background(), callBudget)
	defer cancel()
	if backend.Prepare(ctx) == nil || backend.Owned() || calls != 1 {
		t.Fatal("adopted existing index")
	}
}

func TestBoundedNativeDriverPolicy(t *testing.T) {
	opts := mongoOptions("mongodb://mongo.invalid:27017/?replicaSet=rs0")
	if opts.RetryReads == nil || *opts.RetryReads || opts.RetryWrites == nil || *opts.RetryWrites || opts.MaxAdaptiveRetries == nil || *opts.MaxAdaptiveRetries != 0 {
		t.Fatal("native driver retry policy changed")
	}
	if opts.MaxPoolSize == nil || *opts.MaxPoolSize != 2 || opts.ConnectTimeout == nil || *opts.ConnectTimeout > callBudget || opts.ServerSelectionTimeout == nil || *opts.ServerSelectionTimeout > callBudget {
		t.Fatal("native driver bounds changed")
	}
	client := searchClient()
	transport := client.Transport.(*http.Transport)
	if transport.Proxy != nil || transport.MaxConnsPerHost != 2 || !transport.DisableCompression || client.Timeout > callBudget {
		t.Fatal("Search client bounds changed")
	}
	if client.CheckRedirect(nil, nil) != http.ErrUseLastResponse {
		t.Fatal("Search follows redirects")
	}
}
