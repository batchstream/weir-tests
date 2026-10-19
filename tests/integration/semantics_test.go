//go:build integration

package integration_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"slices"
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
	wires := []nativeInvoke{
		ownerNative(transport(t, s.cluster.Nodes[0].Application)),
		ownerNative(transport(t, s.cluster.Nodes[1].Application)),
	}
	calls := 0
	invoke := func(ctx context.Context, opts weir.NativeOptions) (*weir.NativeResult, error) {
		call := wires[calls%len(wires)]
		calls++
		return call(ctx, opts)
	}
	bound := bson.D{{Key: "$gte", Value: int64(2)}}
	query := nativeQuery{
		mongoFilter:     bson.D{{Key: "n", Value: bound}},
		mongoProjection: bson.D{{Key: "n", Value: 1}, {Key: "_id", Value: 0}},
		searchQuery:     map[string]any{"range": map[string]any{"n": map[string]any{"gte": 2}}},
		searchSource:    []string{"n"},
	}
	pager, err := target.nativePager(invoke, query)
	if err != nil {
		t.Fatal(err)
	}
	documents := collectNativePages(t, s.ctx, pager)
	var numbers []int64
	for _, document := range documents {
		n, err := target.number(document)
		if err != nil {
			t.Fatal(err)
		}
		numbers = append(numbers, n)
		if target.name == "mongo" {
			elements, err := bson.Raw(document.Data).Elements()
			if err != nil || len(elements) != 1 || elements[0].Key() != "n" {
				t.Fatalf("Native Mongo projection: %v %v", elements, err)
			}
		} else {
			var fields map[string]json.RawMessage
			if json.Unmarshal(document.Data, &fields) != nil || len(fields) != 1 || fields["n"] == nil {
				t.Fatalf("Native Search _source inclusion: %s", document.Data)
			}
		}
	}
	slices.Sort(numbers)
	if !slices.Equal(numbers, []int64{2, 3}) || pager.pages < 2 {
		t.Fatalf("filtered Native projection pagination: pages=%d numbers=%v", pager.pages, numbers)
	}
	exclude := nativeQuery{
		mongoProjection: bson.D{{Key: "n", Value: 0}},
		searchSource:    map[string]any{"excludes": []string{"n"}},
	}
	pager, err = target.nativePager(s.client.Native, exclude)
	if err != nil {
		t.Fatal(err)
	}
	documents = collectNativePages(t, s.ctx, pager)
	if len(documents) != 3 {
		t.Fatalf("Native exclusion omitted documents: %d", len(documents))
	}
	for _, document := range documents {
		if target.name == "mongo" {
			if bson.Raw(document.Data).Lookup("n").Type != 0 {
				t.Fatal("Native Mongo excluded field remains")
			}
		} else {
			var fields map[string]json.RawMessage
			if json.Unmarshal(document.Data, &fields) != nil || fields["n"] != nil {
				t.Fatalf("Native Search excluded source field remains: %s", document.Data)
			}
		}
	}
	s.testMissingTargets(t, target)
	s.testAdapterOwnedEnvelopes(t, target)
}

func (s *system) testAdapterOwnedEnvelopes(t *testing.T, target *backendData) {
	t.Helper()
	payload := &weir.Document{ContentType: "application/vnd.example.operation", Data: []byte("opaque request")}
	native := &weir.NativeRequest{Resource: target.collection, Request: payload}
	if target.name == "mongo" {
		native.Resource = weir.EncodeSegment(target.mongo.Database().Name())
	}
	options := weir.NativeOptions{StoreName: target.name, Request: native, Consume: func(context.Context, *weir.NativeResponse, []byte) error { return nil }}
	rpc, cancel := context.WithTimeout(s.ctx, rpcTimeout)
	result, err := s.client.Native(rpc, options)
	cancel()
	if err != nil || result == nil || result.Response != nil || result.Completion != weir.NativeNotStarted || result.Failure.GetCode() != weir.FailureUnsupported {
		t.Fatalf("unknown format must reach adapter classification: %v %v", result, err)
	}
	if target.name == "mongo" {
		// This is a Mongo find projection error, not a portable projection rule.
		projection := bson.D{{Key: "n", Value: 1}, {Key: "other", Value: 0}}
		command := bson.D{{Key: "find", Value: target.mongo.Name()}, {Key: "projection", Value: projection}, {Key: "batchSize", Value: 1}}
		request, err := nativeMongoRequest(target.mongo.Database().Name(), command)
		if err != nil {
			t.Fatal(err)
		}
		caller := &nativePager{store: target.name, invoke: s.client.Native}
		_, body, err := caller.exchange(s.ctx, request)
		raw := bson.Raw(body)
		if err != nil || raw.Validate() != nil {
			t.Fatalf("Mongo projection error lost Native completion: %s %v", raw, err)
		}
		ok, valid := raw.Lookup("ok").AsFloat64OK()
		code, hasCode := raw.Lookup("code").AsInt64OK()
		if !valid || ok != 0 || !hasCode || code != 31254 {
			t.Fatalf("Mongo inclusion/exclusion business error: %s", raw)
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
	query := nativeQuery{searchSource: []string{"$literal"}}
	pager, err := literal.nativePager(s.client.Native, query)
	if err != nil {
		t.Fatal(err)
	}
	documents := collectNativePages(t, s.ctx, pager)
	if len(documents) != 1 {
		t.Fatalf("literal Search projection count=%d", len(documents))
	}
	var fields map[string]int
	if json.Unmarshal(documents[0].Data, &fields) != nil || len(fields) != 1 || fields["$literal"] != 42 {
		t.Fatalf("Native Search literal source field: %s", documents[0].Data)
	}
}

func (s *system) testMissingTargets(t *testing.T, target *backendData) {
	t.Helper()
	missing := *target
	missing.collection += ".never_created"
	if target.name == "mongo" {
		missing.mongo = target.mongo.Database().Collection(target.mongo.Name() + ".never_created")
	}
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
	empty := nativeQuery{
		mongoFilter: bson.D{{Key: "n", Value: int64(999)}},
		searchQuery: map[string]any{"term": map[string]any{"n": 999}},
	}
	pager, err := target.nativePager(s.client.Native, empty)
	if err != nil {
		t.Fatal(err)
	}
	if documents := collectNativePages(t, s.ctx, pager); len(documents) != 0 {
		t.Fatalf("empty Native query returned %d records", len(documents))
	}
	query := nativeQuery{}
	pager, err = missing.nativePager(s.client.Native, query)
	if err != nil {
		t.Fatal(err)
	}
	cleanupNativePager(t, pager)
	if target.name == "mongo" {
		// Mongo find on an absent collection is a successful empty cursor, unlike
		// typed Read's explicit target classification.
		documents := collectNativePages(t, s.ctx, pager)
		if len(documents) != 0 || pager.mongoCursor != 0 {
			t.Fatal("Native Mongo missing collection was not empty/exhausted")
		}
	} else {
		head, body, err := pager.exchange(s.ctx, pager.open)
		if err != nil {
			t.Fatal("missing Search index lost Native completion:", err)
		}
		response, err := weir.ParseHTTPNativeResponse(head)
		var failure struct {
			Error struct {
				Type string `json:"type"`
			} `json:"error"`
		}
		if err != nil || response.StatusCode != http.StatusNotFound || json.Unmarshal(body, &failure) != nil || failure.Error.Type != "index_not_found_exception" {
			t.Fatalf("Native missing Search index: metadata=%v body=%s err=%v", response, body, err)
		}
		native := nativeHTTPGet(t, target.collection, "/_doc/absent")
		head, body, err = pager.exchange(s.ctx, native)
		if err != nil {
			t.Fatal("missing Search record lost Native completion:", err)
		}
		response, err = weir.ParseHTTPNativeResponse(head)
		var record struct {
			Found *bool `json:"found"`
		}
		if err != nil || response.StatusCode != http.StatusNotFound || json.Unmarshal(body, &record) != nil || record.Found == nil || *record.Found {
			t.Fatalf("Native missing Search record: metadata=%v body=%s err=%v", response, body, err)
		}
	}
}
