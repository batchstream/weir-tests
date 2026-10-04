//go:build integration

package integration_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"strconv"
	"strings"
	"testing"
	"time"

	weir "github.com/batchstream/weir-go"
	"github.com/batchstream/weir-tests/internal/fixture"
	"github.com/batchstream/weir-tests/internal/observe"
	"go.mongodb.org/mongo-driver/v2/bson"
)

// Sample only processes started by this suite, including both possible owners.
func ownerResidentBytes(ctx context.Context, nodes []fixture.Node) (uint64, error) {
	var largest uint64
	for _, node := range nodes {
		if !node.Owner {
			continue
		}
		if node.ProcessID <= 0 {
			return 0, errors.New("owned Weir PID unavailable")
		}
		command := exec.CommandContext(ctx, "ps", "-o", "rss=", "-p", strconv.Itoa(node.ProcessID))
		output, err := command.Output()
		if err != nil {
			return 0, err
		}
		kib, err := strconv.ParseUint(strings.TrimSpace(string(output)), 10, 64)
		if err != nil {
			return 0, err
		}
		largest = max(largest, kib*1024)
	}
	return largest, nil
}

func (s *system) testStreamMemory(t *testing.T, backend *backendData) {
	t.Helper()
	const payloadBytes = 64 << 10
	const distinctRecords = 1024
	padding := strings.Repeat("x", payloadBytes)
	requests := make([]*weir.MutateRequest, distinctRecords)
	reads := make([]*weir.ReadRequest, distinctRecords)
	var jsonData []byte
	if backend.name != "mongo" {
		fields := map[string]any{"n": 42, "padding": padding}
		data, err := json.Marshal(fields)
		if err != nil {
			t.Fatal(err)
		}
		jsonData = data
	}
	for index := range distinctRecords {
		id := fmt.Sprintf("stream_memory_%04d", index)
		write := backend.write(t, id, 42)
		if backend.name == "mongo" {
			fields := bson.D{{Key: "_id", Value: id}, {Key: "n", Value: int64(42)}, {Key: "padding", Value: padding}}
			write.Document = bsonDocument(t, fields)
		} else {
			write.Document.Data = jsonData
		}
		request := &weir.MutateRequest{Resource: write.Resource, Action: weir.MutationPut, Document: write.Document}
		requests[index] = request
		read := &weir.ReadRequest{Resource: write.Resource}
		reads[index] = read
	}
	rpc, cancel := context.WithTimeout(s.ctx, 90*time.Second)
	writeOptions := weir.MutateOptions{StoreName: backend.name, Requests: requests}
	results, err := s.client.Mutate(rpc, writeOptions)
	cancel()
	if err != nil || len(results) != distinctRecords {
		t.Fatal("prepare distinct stream documents", len(results), err)
	}
	for _, result := range results {
		assertApplied(t, result, nil)
	}
	backend.assertPersisted(t, s.ctx, "stream_memory_0000", 42)
	backend.assertPersisted(t, s.ctx, "stream_memory_1023", 42)
	var shortPeak uint64
	for _, count := range []int{1024, 4096} {
		before := s.nodeMetrics(t)
		ctx, stop := context.WithTimeout(s.ctx, 90*time.Second)
		produced, consumed := 0, uint64(0)
		var bytes, peak uint64
		sample := func() error {
			resident, err := ownerResidentBytes(ctx, s.cluster.Nodes)
			if err != nil {
				return err
			}
			peak = max(peak, resident)
			for _, node := range s.cluster.Nodes {
				if !node.Owner {
					continue
				}
				snapshot := observe.Fetch(ctx, node.Diagnostics)
				labels := map[string]string{"store": backend.name}
				retained, found := snapshot.Sum("weir_store_result_reserved_bytes", labels)
				limit, limitFound := snapshot.Sum("weir_store_result_reserved_bytes_limit", labels)
				if !found || !limitFound || retained < 0 || retained > limit {
					return fmt.Errorf("Store result credits invalid: retained=%v limit=%v error=%s", retained, limit, snapshot.Error)
				}
			}
			return nil
		}
		if err := sample(); err != nil {
			stop()
			t.Fatal(err)
		}
		options := weir.ReadStreamOptions{
			StoreName: backend.name,
			Next: func(context.Context) (*weir.ReadRequest, error) {
				if produced == count {
					return nil, io.EOF
				}
				request := reads[produced%distinctRecords]
				produced++
				return request, nil
			},
			Consume: func(_ context.Context, index uint64, result *weir.ReadResult) error {
				if index != consumed+1 || result == nil || result.Document == nil || result.Failure != nil || result.Missing {
					return fmt.Errorf("unexpected stream result at %d", index)
				}
				if n, err := backend.number(result.Document); err != nil || n != 42 || len(result.Document.Data) < payloadBytes {
					return fmt.Errorf("large streamed document changed: n=%v error=%v", n, err)
				}
				consumed++
				bytes += uint64(len(result.Document.Data))
				if consumed%128 == 0 {
					return sample()
				}
				return nil
			},
		}
		err := s.client.ReadStream(ctx, options)
		if err == nil {
			err = sample()
		}
		stop()
		if err != nil || consumed != uint64(count) || bytes < uint64(count)*payloadBytes {
			t.Fatalf("large call failed: count=%d consumed=%d bytes=%d error=%v", count, consumed, bytes, err)
		}
		evidence := streamBatchEvidence{Before: before, After: s.nodeMetrics(t), Store: backend.name, Method: "read", Records: count}
		assertStreamBatchMetrics(t, evidence)
		if count == 1024 {
			shortPeak = peak
		} else if peak > shortPeak+(96<<20) {
			t.Fatalf("4x logical output grew Weir RSS excessively: short=%d long=%d", shortPeak, peak)
		}
		t.Logf("%s: one Execute stream delivered %d records (%d bytes), sampled max owner RSS=%d bytes", backend.name, count, bytes, peak)
	}
}
