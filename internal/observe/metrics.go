// Package observe reads the public diagnostics of explicitly supplied local
// fixtures. It never imports Weir server code or discovers deployments.
package observe

import (
	"context"
	"errors"
	"fmt"
	"io"
	"math"
	"net"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"
)

type Metric struct {
	Name   string            `json:"name"`
	Labels map[string]string `json:"labels,omitempty"`
	Value  float64           `json:"value"`
}

type Snapshot struct {
	At       string   `json:"at_utc"`
	Endpoint string   `json:"endpoint"`
	Raw      string   `json:"raw,omitempty"`
	Metrics  []Metric `json:"metrics,omitempty"`
	Error    string   `json:"error,omitempty"`
}

var samplePattern = regexp.MustCompile(`^([a-zA-Z_:][a-zA-Z0-9_:]*)(?:\{(.*)\})?\s+([^\s]+)(?:\s+[0-9]+)?$`)
var labelPattern = regexp.MustCompile(`^([a-zA-Z_][a-zA-Z0-9_]*)=("(?:[^"\\]|\\.)*")`)

func Fetch(ctx context.Context, address string) Snapshot {
	snapshot := Snapshot{At: time.Now().UTC().Format(time.RFC3339Nano), Endpoint: address}
	host, port, err := net.SplitHostPort(address)
	numericPort, portErr := strconv.Atoi(port)
	if err != nil || portErr != nil || numericPort < 1 || numericPort > 65535 || net.ParseIP(host) == nil || !net.ParseIP(host).IsLoopback() {
		snapshot.Error = "diagnostics requires an explicit loopback IP:port"
		return snapshot
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, "http://"+address+"/metrics", nil)
	if err != nil {
		snapshot.Error = err.Error()
		return snapshot
	}
	transport := &http.Transport{Proxy: nil, DisableKeepAlives: true, DisableCompression: true}
	client := &http.Client{Transport: transport, Timeout: 2 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	defer client.CloseIdleConnections()
	response, err := client.Do(request)
	if err != nil {
		snapshot.Error = err.Error()
		return snapshot
	}
	defer response.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(response.Body, (4<<20)+1))
	if err != nil || len(raw) > 4<<20 || response.StatusCode != http.StatusOK {
		snapshot.Error = "diagnostic metrics response unavailable or oversized"
		return snapshot
	}
	snapshot.Raw = string(raw)
	snapshot.Metrics, err = Parse(snapshot.Raw)
	if err != nil {
		snapshot.Error = err.Error()
	}
	return snapshot
}

func Parse(raw string) ([]Metric, error) {
	metrics := make([]Metric, 0)
	for lineNumber, line := range strings.Split(raw, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		parts := samplePattern.FindStringSubmatch(line)
		if len(parts) != 4 {
			return nil, fmt.Errorf("invalid diagnostic metric line %d", lineNumber+1)
		}
		value, err := strconv.ParseFloat(parts[3], 64)
		if err != nil || math.IsNaN(value) || math.IsInf(value, 0) {
			return nil, fmt.Errorf("non-finite diagnostic metric line %d", lineNumber+1)
		}
		labels := make(map[string]string)
		rest := parts[2]
		for rest != "" {
			label := labelPattern.FindStringSubmatch(rest)
			if len(label) != 3 {
				return nil, fmt.Errorf("invalid diagnostic labels line %d", lineNumber+1)
			}
			decoded, err := strconv.Unquote(label[2])
			if err != nil {
				return nil, err
			}
			if _, exists := labels[label[1]]; exists {
				return nil, errors.New("duplicate diagnostic metric label")
			}
			labels[label[1]] = decoded
			rest = strings.TrimPrefix(rest, label[0])
			if rest != "" {
				if !strings.HasPrefix(rest, ",") {
					return nil, errors.New("invalid diagnostic label delimiter")
				}
				rest = strings.TrimPrefix(rest, ",")
				if rest == "" {
					return nil, errors.New("trailing diagnostic label delimiter")
				}
			}
		}
		metric := Metric{Name: parts[1], Labels: labels, Value: value}
		metrics = append(metrics, metric)
	}
	if len(metrics) == 0 {
		return nil, errors.New("diagnostic metrics response is empty")
	}
	return metrics, nil
}

func (s Snapshot) Sum(name string, labels map[string]string) (float64, bool) {
	if s.Error != "" {
		return 0, false
	}
	var sum float64
	found := false
	for _, metric := range s.Metrics {
		if metric.Name != name {
			continue
		}
		matches := true
		for key, value := range labels {
			if metric.Labels[key] != value {
				matches = false
				break
			}
		}
		if matches {
			sum += metric.Value
			found = true
		}
	}
	return sum, found
}

func Delta(before, after Snapshot, name string, labels map[string]string) (float64, error) {
	left, foundBefore := before.Sum(name, labels)
	right, foundAfter := after.Sum(name, labels)
	if before.Error != "" || after.Error != "" {
		return 0, errors.New("before/after diagnostic observation failed")
	}
	if !foundBefore || !foundAfter {
		return 0, fmt.Errorf("diagnostic metric %s unavailable", name)
	}
	if right < left {
		return 0, fmt.Errorf("diagnostic metric %s decreased or reset", name)
	}
	return right - left, nil
}
