package workload

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
)

func (d *Dataset) luaSource() string {
	// Reassigning the existing revision field preserves its BSON int32 width.
	return `return function(current)
  current.revision = 1 - current.revision
  return current
end`
}

// This fixed transformation has a native atomic implementation on both backends.
// It is not a baseline for arbitrary Lua programs.
func (p *directPath) executeMongoTransform(ctx context.Context, operation Operation) Outcome {
	collection := p.mongo.Database(p.dataset.Config.Namespace).Collection("records")
	filter := bson.D{{Key: "_id", Value: p.dataset.ID(operation.Record)}}
	subtract := bson.D{{Key: "$subtract", Value: bson.A{int32(1), "$revision"}}}
	fields := bson.D{{Key: "revision", Value: subtract}}
	stage := bson.D{{Key: "$set", Value: fields}}
	pipeline := mongo.Pipeline{stage}
	result, err := collection.UpdateOne(ctx, filter, pipeline)
	if err != nil {
		var rejection mongo.WriteException
		uncertain := !errors.As(err, &rejection) || rejection.WriteConcernError != nil || len(rejection.WriteErrors) == 0
		return failed(err, uncertain)
	}
	if result == nil || !result.Acknowledged || result.MatchedCount != 1 || result.ModifiedCount != 1 {
		return failed(errors.New("native transform did not acknowledge exactly one changed record"), true)
	}
	outcome := Outcome{Status: Success, Applied: true}
	return outcome
}

func (p *directPath) executeSearchTransform(ctx context.Context, operation Operation) Outcome {
	id := p.dataset.ID(operation.Record)
	payload := []byte(`{"script":{"lang":"painless","source":"ctx._source.revision = 1 - ctx._source.revision"}}`)
	query := url.Values{"refresh": {"false"}, "wait_for_active_shards": {"1"}, "timeout": {"1s"}, "retry_on_conflict": {"0"}}
	path := "/" + p.dataset.Config.Namespace + "/_update/" + url.PathEscape(id) + "?" + query.Encode()
	status, raw, err := p.request(ctx, http.MethodPost, path, payload)
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
	outcome := Outcome{Status: Success, Applied: true}
	return outcome
}
