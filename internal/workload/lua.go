package workload

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
	"go.mongodb.org/mongo-driver/v2/mongo/readconcern"
	"go.mongodb.org/mongo-driver/v2/mongo/readpref"
	"go.mongodb.org/mongo-driver/v2/mongo/writeconcern"
)

func (d *Dataset) luaSource() string {
	// The fixture stores BSON int32 and JSON 0/1, both read as lua.v1 int32.
	return `return weir.replace(weir.set(current, "revision", weir.sub(weir.i32("1"), weir.get(current, "revision"))))`
}

// The native path performs the same single-record read-modify-write, without
// client batching or write replay. Partitioned worker IDs avoid planned conflicts.
func (p *directPath) executeMongoTransform(ctx context.Context, operation Operation) Outcome {
	session, err := p.mongo.StartSession()
	if err != nil {
		return failed(err, false)
	}
	defer func() {
		cleanup, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		session.EndSession(cleanup)
	}()
	txnOptions := options.Transaction().SetReadConcern(readconcern.Snapshot()).SetWriteConcern(writeconcern.Majority()).SetReadPreference(readpref.Primary())
	if err := session.StartTransaction(txnOptions); err != nil {
		return failed(err, false)
	}
	txctx := mongo.NewSessionContext(ctx, session)
	collection := p.mongo.Database(p.dataset.Config.Namespace).Collection("records")
	filter := bson.D{{Key: "_id", Value: p.dataset.ID(operation.Record)}}
	raw, err := collection.FindOne(txctx, filter).Raw()
	if err != nil {
		return failed(err, false)
	}
	previous := operation
	previous.Revision = 1 - operation.Revision
	if err := p.dataset.Validate(raw, previous); err != nil {
		return failed(err, false)
	}
	var replacement bson.D
	if err := bson.Unmarshal(raw, &replacement); err != nil {
		return failed(err, false)
	}
	for index := range replacement {
		if replacement[index].Key == "revision" {
			replacement[index].Value = int32(operation.Revision)
		}
	}
	result, err := collection.ReplaceOne(txctx, filter, replacement)
	if err != nil {
		return failed(err, false)
	}
	if result == nil || !result.Acknowledged || result.MatchedCount != 1 || result.ModifiedCount != 1 {
		return failed(errors.New("native transform did not change exactly one record"), false)
	}
	if err := session.CommitTransaction(txctx); err != nil {
		return failed(err, true)
	}
	outcome := Outcome{Status: Success, Applied: true, RequestBytes: uint64(len(raw)), ResponseBytes: uint64(len(raw))}
	return outcome
}

func (p *directPath) executeSearchTransform(ctx context.Context, operation Operation) Outcome {
	id := p.dataset.ID(operation.Record)
	readBody := map[string]any{"ids": []string{id}}
	encoded, err := json.Marshal(readBody)
	if err != nil {
		return failed(err, false)
	}
	status, raw, err := p.request(ctx, http.MethodPost, "/"+p.dataset.Config.Namespace+"/_mget?realtime=true", encoded)
	if err != nil || status != http.StatusOK {
		return failed(errors.Join(fmt.Errorf("native transform read HTTP status %d", status), err), false)
	}
	var envelope struct {
		Docs []struct {
			ID          string          `json:"_id"`
			Index       string          `json:"_index"`
			Found       *bool           `json:"found"`
			Source      json.RawMessage `json:"_source"`
			Sequence    *int64          `json:"_seq_no"`
			PrimaryTerm *int64          `json:"_primary_term"`
			Error       json.RawMessage `json:"error"`
		} `json:"docs"`
	}
	if json.Unmarshal(raw, &envelope) != nil || len(envelope.Docs) != 1 {
		return failed(errors.New("native transform requires exactly one observed record"), false)
	}
	observed := envelope.Docs[0]
	if observed.ID != id || observed.Index != p.dataset.Config.Namespace || observed.Found == nil || !*observed.Found || observed.Sequence == nil || *observed.Sequence < 0 || observed.PrimaryTerm == nil || *observed.PrimaryTerm < 1 || len(observed.Error) != 0 {
		return failed(errors.New("native transform lacks matching source and version evidence"), false)
	}
	previous := operation
	previous.Revision = 1 - operation.Revision
	if err := p.dataset.Validate(observed.Source, previous); err != nil {
		return failed(err, false)
	}
	var replacement document
	if err := json.Unmarshal(observed.Source, &replacement); err != nil {
		return failed(err, false)
	}
	replacement.Revision = int32(operation.Revision)
	payload, err := json.Marshal(replacement)
	if err != nil {
		return failed(err, false)
	}
	query := url.Values{"pipeline": {"_none"}, "refresh": {"false"}, "wait_for_active_shards": {"1"}, "timeout": {"1s"}, "if_seq_no": {strconv.FormatInt(*observed.Sequence, 10)}, "if_primary_term": {strconv.FormatInt(*observed.PrimaryTerm, 10)}}
	path := "/" + p.dataset.Config.Namespace + "/_doc/" + url.PathEscape(id) + "?" + query.Encode()
	status, raw, err = p.request(ctx, http.MethodPut, path, payload)
	if err != nil {
		return failed(err, true)
	}
	if status != http.StatusOK {
		return failed(fmt.Errorf("native transform write HTTP status %d", status), status != 400 && status != 404 && status != 409 && status != 429)
	}
	var reply struct {
		ID     string `json:"_id"`
		Index  string `json:"_index"`
		Result string `json:"result"`
		Shards struct {
			Successful int `json:"successful"`
			Failed     int `json:"failed"`
		} `json:"_shards"`
	}
	if json.Unmarshal(raw, &reply) != nil || reply.ID != id || reply.Index != p.dataset.Config.Namespace || reply.Result != "updated" || reply.Shards.Successful < 1 || reply.Shards.Failed != 0 {
		return failed(errors.New("native transform write lacks a complete acknowledgement"), true)
	}
	outcome := Outcome{Status: Success, Applied: true, RequestBytes: uint64(len(payload)), ResponseBytes: uint64(len(observed.Source))}
	return outcome
}
