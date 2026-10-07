//go:build integration

package integration_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"testing"

	weir "github.com/batchstream/weir-go"
	"go.mongodb.org/mongo-driver/v2/bson"
)

func (s *system) testLuaSemantics(t *testing.T, backend *backendData) {
	t.Helper()
	cases := []struct {
		name, source string
		present      bool
		n            int64
		failure      weir.FailureCode
	}{
		{name: "merge", source: "return function(current, incoming) current = current or weir.object(); for key, item in pairs(incoming) do current[key] = item end; return current end", present: true, n: 42},
		{name: "replace", source: "return function(_, incoming) return incoming end", present: true, n: 42},
		{name: "keep", source: "return function() return weir.keep() end"},
		{name: "delete", source: "return function() return weir.delete() end"},
		{name: "reject", source: "return function() return weir.reject(\"rejected\") end", failure: weir.FailurePreconditionFailed},
		{name: "no_return", source: "return function(current) local v = current end", failure: weir.FailureInvalidArgument},
		{name: "nil_return", source: "return function() return nil end", failure: weir.FailureInvalidArgument},
		{name: "multiple_return", source: "return function() return {}, weir.keep() end", failure: weir.FailureInvalidArgument},
		{name: "current", source: "return function(current) return current end"},
		{name: "invalid", source: "return function() return ) end", failure: weir.FailureInvalidArgument},
	}
	for _, existing := range []bool{false, true} {
		for _, tc := range cases {
			t.Run(fmt.Sprintf("lua/%s/existing=%t", tc.name, existing), func(t *testing.T) {
				id := fmt.Sprintf("lua_%s_%t", tc.name, existing)
				if existing {
					write := backend.write(t, id, 1)
					opts := weir.WriteOptions{StoreName: backend.name, Request: write}
					result, err := s.client.Create(s.ctx, opts)
					assertApplied(t, result, err)
				}
				input := backend.write(t, id, 42).Document
				program := &weir.LuaTransform{Source: []byte(tc.source), Input: input}
				request := &weir.AtomicTransformRequest{Resource: backend.resource(id), Lua: program}
				opts := weir.AtomicTransformOptions{StoreName: backend.name, Request: request}
				rpc, cancel := context.WithTimeout(s.ctx, rpcTimeout)
				result, err := s.client.AtomicTransform(rpc, opts)
				cancel()
				failure := tc.failure
				if tc.name == "current" && !existing {
					failure = weir.FailureInvalidArgument
				}
				if failure != 0 {
					if err != nil || result.GetOutcome() != weir.MutationNotApplied || result.GetFailure().GetCode() != failure {
						t.Fatalf("rejected Lua: %v %v", result, err)
					}
				} else {
					assertApplied(t, result, err)
				}
				present, n := tc.present, tc.n
				if tc.name != "delete" && !tc.present && existing {
					present, n = true, 1
				}
				if present {
					backend.assertPersisted(t, s.ctx, id, n)
				} else {
					backend.assertMissing(t, s.ctx, id)
				}
			})
		}
	}
	s.testConcurrentLuaCreate(t, backend)
}

func (s *system) testConcurrentLuaCreate(t *testing.T, backend *backendData) {
	t.Helper()
	const id = "lua_cross_owner_counter"
	const iterations = 4
	source := []byte(`return function(current, incoming)
  if current == nil then return incoming end
  current.n = current.n + 1
  return current
end`)
	start := make(chan struct{})
	finished := make(chan error, 2)
	for _, node := range s.cluster.Nodes[:2] {
		wire := transport(t, node.Application)
		input := backend.write(t, id, 1).Document
		go func() {
			<-start
			for range iterations {
				program := &weir.LuaTransform{Source: source, Input: input}
				request := &weir.AtomicTransformRequest{Resource: backend.resource(id), Lua: program}
				opts := weir.AtomicTransformOptions{StoreName: backend.name, Request: request}
				rpc, cancel := context.WithTimeout(s.ctx, rpcTimeout)
				result, err := weir.AtomicTransform(rpc, wire, opts)
				cancel()
				if err != nil || result.GetOutcome() != weir.MutationApplied || result.GetFailure() != nil {
					finished <- fmt.Errorf("cross-owner Lua: result=%v error=%v", result, err)
					return
				}
			}
			finished <- nil
		}()
	}
	close(start)
	var firstError error
	for range 2 {
		if err := <-finished; err != nil && firstError == nil {
			firstError = err
		}
	}
	if firstError != nil {
		t.Fatal(firstError)
	}
	backend.assertPersisted(t, s.ctx, id, 2*iterations)
}

func (s *system) testProjectionAndFailures(t *testing.T, backend *backendData) {
	t.Helper()
	target := backend.provisionTarget(t, s.ctx, "projection.entries")
	for n := int64(1); n <= 3; n++ {
		write := target.write(t, fmt.Sprint(n), n)
		opts := weir.WriteOptions{StoreName: target.name, Request: write}
		result, err := s.client.Create(s.ctx, opts)
		assertApplied(t, result, err)
	}
	if target.name == "search" {
		refresh := httpRequest{method: http.MethodPost, path: "/" + target.collection + "/_refresh"}
		code, body := target.httpDo(t, s.ctx, refresh)
		if code != http.StatusOK {
			t.Fatalf("refresh projection fixture: %d %s", code, body)
		}
	}
	filter := &weir.Document{ContentType: "application/json", Data: []byte(`{"range":{"n":{"gte":2}}}`)}
	if target.name == "mongo" {
		bound := bson.D{{Key: "$gte", Value: int64(2)}}
		value := bson.D{{Key: "n", Value: bound}}
		filter = bsonDocument(t, value)
	}
	projection := &weir.Projection{Mode: weir.ProjectionInclude, Fields: []string{"n"}}
	request := &weir.ScanRequest{Resource: target.collection, Filter: filter, Projection: projection, PageSize: 1}
	seen := make(map[int64]bool)
	exhausted := false
	for page := range 4 {
		wire := transport(t, s.cluster.Nodes[page%2].Application)
		opts := weir.ScanOptions{StoreName: target.name, Request: request, Consume: func(_ context.Context, document *weir.Document) error {
			n, err := target.number(document)
			if err != nil {
				return err
			}
			if n < 2 || seen[n] {
				return fmt.Errorf("wrong or duplicate projected record %d", n)
			}
			seen[n] = true
			if target.name == "mongo" {
				elements, err := bson.Raw(document.Data).Elements()
				if err != nil || len(elements) != 1 || elements[0].Key() != "n" {
					return fmt.Errorf("unexpected Mongo projection: %v %v", elements, err)
				}
			} else {
				var fields map[string]json.RawMessage
				if err := json.Unmarshal(document.Data, &fields); err != nil {
					return err
				}
				if len(fields) != 1 || fields["n"] == nil {
					return fmt.Errorf("unexpected Search projection: %s", document.Data)
				}
			}
			return nil
		}}
		end, err := weir.Scan(s.ctx, wire, opts)
		if err != nil || end.GetFailure() != nil || end.GetDocumentCount() > 1 {
			t.Fatalf("projected page: %v %v", end, err)
		}
		if page == 0 {
			if end.GetExhausted() || len(end.GetNextContinuationToken()) == 0 {
				t.Fatal("first filtered page has no continuation", end)
			}
			changed := &weir.Projection{Mode: weir.ProjectionExclude, Fields: []string{"n"}}
			invalid := &weir.ScanRequest{Resource: target.collection, Filter: filter, Projection: changed, PageSize: 1, ContinuationToken: end.NextContinuationToken}
			invalidOpts := weir.ScanOptions{StoreName: target.name, Request: invalid, Consume: func(context.Context, *weir.Document) error { return nil }}
			checkpoint, err := weir.Scan(s.ctx, wire, invalidOpts)
			if err != nil || checkpoint.GetFailure().GetCode() != weir.FailureInvalidArgument || checkpoint.GetDocumentCount() != 0 || len(checkpoint.GetNextContinuationToken()) != 0 {
				t.Fatalf("changed projection accepted token: %v %v", checkpoint, err)
			}
			request.ContinuationToken = end.NextContinuationToken
		}
		if end.GetExhausted() {
			exhausted = true
			break
		}
		request.ContinuationToken = end.NextContinuationToken
	}
	if !exhausted || len(seen) != 2 {
		t.Fatal("filtered projection omitted documents", seen)
	}
	exclude := &weir.Projection{Mode: weir.ProjectionExclude, Fields: []string{"n"}}
	excluded := &weir.ScanRequest{Resource: target.collection, Projection: exclude, PageSize: 4}
	excludedOptions := weir.ScanOptions{StoreName: target.name, Request: excluded, Consume: func(_ context.Context, document *weir.Document) error {
		if target.name == "mongo" {
			if bson.Raw(document.Data).Lookup("n").Type != 0 {
				return fmt.Errorf("excluded Mongo field remains")
			}
		} else {
			var fields map[string]json.RawMessage
			if err := json.Unmarshal(document.Data, &fields); err != nil {
				return err
			}
			if fields["n"] != nil {
				return fmt.Errorf("excluded Search field remains")
			}
		}
		return nil
	}}
	end, err := s.client.Scan(s.ctx, excludedOptions)
	if err != nil || end.GetFailure() != nil || end.GetDocumentCount() != 3 || !end.GetExhausted() {
		t.Fatalf("exclude projection: %v %v", end, err)
	}
	s.testMissingTargets(t, target)
	s.testAdapterOwnedEnvelopes(t, target)
}

func (s *system) testAdapterOwnedEnvelopes(t *testing.T, target *backendData) {
	t.Helper()
	payload := &weir.Document{ContentType: "application/vnd.example.operation", Data: []byte("opaque request")}
	native := &weir.NativeRequest{Resource: target.collection, Request: payload}
	options := weir.NativeOptions{StoreName: target.name, Request: native, Consume: func(context.Context, *weir.NativeResponse, []byte) error { return nil }}
	result, err := s.client.Native(s.ctx, options)
	if err != nil || result == nil || result.Response != nil || result.Completion != weir.NativeNotStarted || result.Failure.GetCode() != weir.FailureUnsupported {
		t.Fatalf("unknown format must reach adapter classification: %v %v", result, err)
	}

	projection := &weir.Projection{Mode: weir.ProjectionInclude, Fields: []string{"$literal"}}
	request := &weir.ScanRequest{Resource: target.collection, Projection: projection, PageSize: 1}
	scan := weir.ScanOptions{StoreName: target.name, Request: request, Consume: func(context.Context, *weir.Document) error { return nil }}
	if target.name == "mongo" {
		end, err := s.client.Scan(s.ctx, scan)
		if err != nil || end.GetDocumentCount() != 0 || end.GetFailure().GetCode() != weir.FailureUnsupported {
			t.Fatalf("Mongo field rule must be enforced by adapter: %v %v", end, err)
		}
		return
	}

	literal := target.provisionTarget(t, s.ctx, "literal.fields")
	document := &weir.Document{ContentType: "application/json", Data: []byte(`{"$literal":42,"n":1}`)}
	write := &weir.WriteRequest{Resource: literal.resource("literal"), Document: document}
	writeOptions := weir.WriteOptions{StoreName: literal.name, Request: write}
	mutation, err := s.client.Create(s.ctx, writeOptions)
	assertApplied(t, mutation, err)
	refresh := httpRequest{method: http.MethodPost, path: "/" + literal.collection + "/_refresh"}
	code, body := literal.httpDo(t, s.ctx, refresh)
	if code != http.StatusOK {
		t.Fatalf("refresh literal field: %d %s", code, body)
	}
	request.Resource = literal.collection
	scan.Consume = func(_ context.Context, value *weir.Document) error {
		var fields map[string]int
		if err := json.Unmarshal(value.Data, &fields); err != nil {
			return err
		}
		if len(fields) != 1 || fields["$literal"] != 42 {
			return fmt.Errorf("literal field projection mismatch: %s", value.Data)
		}
		return nil
	}
	end, err := s.client.Scan(s.ctx, scan)
	if err != nil || end.GetFailure() != nil || end.GetDocumentCount() != 1 {
		t.Fatalf("store-neutral literal field: %v %v", end, err)
	}
}

func (s *system) testMissingTargets(t *testing.T, target *backendData) {
	t.Helper()
	missing := *target
	missing.collection += ".never_created"
	requests := []*weir.ReadRequest{
		{Resource: target.resource("1")}, {Resource: target.resource("absent")}, {Resource: missing.resource("1")},
	}
	opts := weir.ReadOptions{StoreName: target.name, Requests: requests}
	results, err := s.client.Read(s.ctx, opts)
	if err != nil || len(results) != 3 {
		t.Fatalf("mixed read results: %v %v", results, err)
	}
	if results[0].Document == nil || !results[1].Missing || results[1].Failure != nil || results[2].Missing || results[2].Failure.GetCode() != weir.FailureTargetNotFound {
		t.Fatalf("record/target absence confused: %v", results)
	}
	emptyFilter := &weir.Document{ContentType: "application/json", Data: []byte(`{"term":{"n":999}}`)}
	if target.name == "mongo" {
		value := bson.D{{Key: "n", Value: int64(999)}}
		emptyFilter = bsonDocument(t, value)
	}
	request := &weir.ScanRequest{Resource: target.collection, Filter: emptyFilter, PageSize: 1}
	scan := weir.ScanOptions{StoreName: target.name, Request: request, Consume: func(context.Context, *weir.Document) error { return nil }}
	end, err := s.client.Scan(s.ctx, scan)
	if err != nil || end.GetFailure() != nil || !end.GetExhausted() || end.GetDocumentCount() != 0 {
		t.Fatalf("empty scan: %v %v", end, err)
	}
	request = &weir.ScanRequest{Resource: missing.collection, PageSize: 1}
	scan.Request = request
	end, err = s.client.Scan(s.ctx, scan)
	if err != nil || end.GetFailure().GetCode() != weir.FailureTargetNotFound || end.GetExhausted() || len(end.GetNextContinuationToken()) != 0 {
		t.Fatalf("missing target scan: %v %v", end, err)
	}
	nativeMissing := &weir.NativeRequest{Resource: missing.collection}
	if target.name == "mongo" {
		command := bson.D{{Key: "count", Value: target.mongo.Name() + ".never_created"}}
		nativeMissing.Request = bsonDocument(t, command)
	} else {
		nativeMissing = nativeHTTPGet(t, missing.collection, "/_doc/absent")
	}
	nativeMissingOptions := weir.NativeOptions{StoreName: target.name, Request: nativeMissing, Consume: func(context.Context, *weir.NativeResponse, []byte) error { return nil }}
	missingResult, err := s.client.Native(s.ctx, nativeMissingOptions)
	if err != nil || missingResult == nil || missingResult.Response != nil || missingResult.Completion != weir.NativeNotStarted || missingResult.Failure.GetCode() != weir.FailureTargetNotFound {
		t.Fatalf("missing Native target: %v %v", missingResult, err)
	}
	if target.name == "search" {
		native := nativeHTTPGet(t, target.collection, "/_doc/absent")
		nativeOpts := weir.NativeOptions{StoreName: target.name, Request: native, Consume: func(context.Context, *weir.NativeResponse, []byte) error { return nil }}
		result, err := s.client.Native(s.ctx, nativeOpts)
		if err != nil || result == nil || result.Completion != weir.NativeResponseComplete || result.Failure != nil {
			t.Fatalf("complete HTTP 404 native response: %v %v", result, err)
		}
		response, err := weir.ParseHTTPNativeResponse(result.Response)
		if err != nil || response.StatusCode != http.StatusNotFound {
			t.Fatalf("HTTP 404 metadata: %v %v", response, err)
		}
	}
}
