package benchmark

import (
	"context"
	"io"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strings"
	"sync"
	"time"
)

type crawlerHTTPCount struct{ Requests, RequestBytes, ResponseBytes, Failed uint64 }
type crawlerObserver struct {
	transport *http.Transport
	mu        sync.Mutex
	totals    map[string]crawlerHTTPCount
	server    *http.Server
	URL       string
}
type crawlerCountingBody struct {
	io.ReadCloser
	count uint64
}

func (b *crawlerCountingBody) Read(raw []byte) (int, error) {
	n, err := b.ReadCloser.Read(raw)
	b.count += uint64(n)
	return n, err
}

type crawlerCountingWriter struct {
	http.ResponseWriter
	bytes  uint64
	status int
}

func (w *crawlerCountingWriter) WriteHeader(status int) {
	w.status = status
	w.ResponseWriter.WriteHeader(status)
}
func (w *crawlerCountingWriter) Write(raw []byte) (int, error) {
	n, err := w.ResponseWriter.Write(raw)
	w.bytes += uint64(n)
	return n, err
}
func (o *crawlerObserver) snapshot() map[string]crawlerHTTPCount {
	o.mu.Lock()
	defer o.mu.Unlock()
	result := make(map[string]crawlerHTTPCount, len(o.totals))
	for key, value := range o.totals {
		result[key] = value
	}
	return result
}
func startCrawlerObserver(target string) (*crawlerObserver, error) {
	parsed, err := url.Parse(target)
	if err != nil {
		return nil, err
	}
	listener, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		return nil, err
	}
	observer := &crawlerObserver{totals: map[string]crawlerHTTPCount{}, URL: "http://" + listener.Addr().String()}
	proxy := httputil.NewSingleHostReverseProxy(parsed)
	observer.transport = &http.Transport{Proxy: nil, DisableCompression: true, MaxConnsPerHost: 1024, MaxIdleConnsPerHost: 1024, MaxIdleConns: 1024, IdleConnTimeout: time.Minute}
	proxy.Transport = observer.transport
	handler := http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		body := &crawlerCountingBody{ReadCloser: request.Body}
		request.Body = body
		counting := &crawlerCountingWriter{ResponseWriter: writer, status: http.StatusOK}
		proxy.ServeHTTP(counting, request)
		route := request.URL.Path
		parts := strings.Split(strings.Trim(route, "/"), "/")
		label := "other"
		for _, part := range parts {
			if strings.HasPrefix(part, "_") {
				label = part
				break
			}
		}
		if label == "other" && request.Method == http.MethodGet && len(parts) == 1 {
			label = "index-metadata"
		}
		if strings.Contains(route, "_stats") || strings.Contains(route, "_nodes") {
			label = "monitor"
		}
		key := request.Method + " " + label
		observer.mu.Lock()
		counter := observer.totals[key]
		counter.Requests++
		counter.RequestBytes += body.count
		counter.ResponseBytes += counting.bytes
		if counting.status >= 400 {
			counter.Failed++
		}
		observer.totals[key] = counter
		observer.mu.Unlock()
	})
	observer.server = &http.Server{Handler: handler, ReadHeaderTimeout: 5e9}
	go func() { _ = observer.server.Serve(listener) }()
	return observer, nil
}
func (o *crawlerObserver) close(ctx context.Context) error {
	err := o.server.Shutdown(ctx)
	o.transport.CloseIdleConnections()
	return err
}
func crawlerHTTPDelta(before, after map[string]crawlerHTTPCount) map[string]crawlerHTTPCount {
	result := map[string]crawlerHTTPCount{}
	for key, value := range after {
		old := before[key]
		delta := crawlerHTTPCount{Requests: value.Requests - old.Requests, RequestBytes: value.RequestBytes - old.RequestBytes, ResponseBytes: value.ResponseBytes - old.ResponseBytes, Failed: value.Failed - old.Failed}
		result[key] = delta
	}
	return result
}
