package workload

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

// ExecuteBatch uses the database's native bulk API. The benchmark gives each
// worker a homogeneous batch of distinct IDs so no write/read dependencies are
// hidden by unordered bulk execution.
func (p *directPath) ExecuteBatch(ctx context.Context, operations []Operation) []Outcome {
	if p.dataset.Config.LuaMutations {
		if len(operations) != 1 {
			err := errors.New("Lua comparison requires one record per client call")
			outcomes := make([]Outcome, len(operations))
			for index := range outcomes {
				outcomes[index] = failed(err, false)
			}
			return outcomes
		}
		outcomes := []Outcome{p.Execute(ctx, operations[0])}
		return outcomes
	}
	if err := validateBatch(operations); err != nil {
		return batchFailure(operations, err)
	}
	if p.mongo != nil {
		return p.mongoBatch(ctx, operations, true)
	}
	if len(operations) == 1 {
		return []Outcome{p.Execute(ctx, operations[0])}
	}
	return p.searchBatch(ctx, operations)
}

func validateBatch(operations []Operation) error {
	if len(operations) < 1 || len(operations) > 64 {
		return errors.New("batch requires 1 through 64 records")
	}
	seen := make(map[int]bool, len(operations))
	for _, operation := range operations {
		if operation.Write != operations[0].Write || seen[operation.Record] {
			return errors.New("benchmark batch must be homogeneous with distinct records")
		}
		seen[operation.Record] = true
	}
	return nil
}

func batchFailure(operations []Operation, err error) []Outcome {
	results := make([]Outcome, len(operations))
	for index, operation := range operations {
		results[index] = failed(err, operation.Write)
	}
	return results
}

func (p *directPath) mongoBatch(ctx context.Context, operations []Operation, requireChanges bool) []Outcome {
	collection := p.mongo.Database(p.dataset.Config.Namespace).Collection("records")
	results := make([]Outcome, len(operations))
	if operations[0].Write {
		models := make([]mongo.WriteModel, len(operations))
		for index, operation := range operations {
			filter := bson.D{{Key: "_id", Value: p.dataset.ID(operation.Record)}}
			models[index] = mongo.NewReplaceOneModel().SetFilter(filter).SetReplacement(bson.Raw(p.dataset.Document(operation))).SetUpsert(true)
		}
		opts := options.BulkWrite().SetOrdered(false)
		reply, err := collection.BulkWrite(ctx, models, opts)
		if err != nil {
			return batchFailure(operations, err)
		}
		if reply == nil || !reply.Acknowledged || reply.MatchedCount+reply.UpsertedCount != int64(len(operations)) {
			return batchFailure(operations, errors.New("native bulk replacement lacks acknowledgements for all records"))
		}
		if requireChanges && reply.ModifiedCount+reply.UpsertedCount != int64(len(operations)) {
			return batchFailure(operations, errors.New("timed native bulk replacement did not change every record"))
		}
		for index, operation := range operations {
			outcome := Outcome{Status: Success, Applied: true, RequestBytes: uint64(len(p.dataset.Document(operation)))}
			results[index] = outcome
		}
		return results
	}
	ids := make([]string, len(operations))
	for index, operation := range operations {
		ids[index] = p.dataset.ID(operation.Record)
	}
	filter := bson.D{{Key: "_id", Value: bson.D{{Key: "$in", Value: ids}}}}
	cursor, err := collection.Find(ctx, filter)
	if err != nil {
		return batchFailure(operations, err)
	}
	defer cursor.Close(ctx)
	documents := make(map[string][]byte, len(operations))
	for cursor.Next(ctx) {
		raw := append([]byte(nil), cursor.Current...)
		id, ok := bson.Raw(raw).Lookup("_id").StringValueOK()
		if !ok || documents[id] != nil {
			return batchFailure(operations, errors.New("invalid native bulk read identity"))
		}
		documents[id] = raw
	}
	if err := cursor.Err(); err != nil {
		return batchFailure(operations, err)
	}
	if len(documents) != len(operations) {
		return batchFailure(operations, errors.New("native bulk read count mismatch"))
	}
	for index, operation := range operations {
		raw := documents[p.dataset.ID(operation.Record)]
		if err := p.dataset.Validate(raw, operation); err != nil {
			results[index] = failed(err, false)
			continue
		}
		outcome := Outcome{Status: Success, ResponseBytes: uint64(len(raw))}
		results[index] = outcome
	}
	return results
}

func (p *directPath) searchBatch(ctx context.Context, operations []Operation) []Outcome {
	var payload []byte
	path := "/" + p.dataset.Config.Namespace
	if operations[0].Write {
		var body bytes.Buffer
		for _, operation := range operations {
			fmt.Fprintf(&body, "{\"index\":{\"_id\":%q}}\n", p.dataset.ID(operation.Record))
			body.Write(p.dataset.Document(operation))
			body.WriteByte('\n')
		}
		payload = body.Bytes()
		path += "/_bulk?pipeline=_none&refresh=false&wait_for_active_shards=1&timeout=1s"
	} else {
		ids := make([]string, len(operations))
		for index, operation := range operations {
			ids[index] = p.dataset.ID(operation.Record)
		}
		request := struct {
			IDs []string `json:"ids"`
		}{IDs: ids}
		var err error
		payload, err = json.Marshal(request)
		if err != nil {
			return batchFailure(operations, err)
		}
		path += "/_mget?realtime=true"
	}
	status, body, err := p.request(ctx, http.MethodPost, path, payload)
	if err != nil {
		return batchFailure(operations, err)
	}
	if status != http.StatusOK {
		return batchFailure(operations, fmt.Errorf("Search bulk HTTP status %d", status))
	}
	results := make([]Outcome, len(operations))
	if operations[0].Write {
		var reply struct {
			Items []struct {
				Index struct {
					Index  string          `json:"_index"`
					ID     string          `json:"_id"`
					Status int             `json:"status"`
					Result string          `json:"result"`
					Error  json.RawMessage `json:"error"`
					Shards struct {
						Successful int `json:"successful"`
						Failed     int `json:"failed"`
					} `json:"_shards"`
				} `json:"index"`
			} `json:"items"`
		}
		if err := json.Unmarshal(body, &reply); err != nil || len(reply.Items) != len(operations) {
			return batchFailure(operations, errors.New("Search bulk acknowledgement count mismatch"))
		}
		for index, operation := range operations {
			item := reply.Items[index].Index
			if item.Index != p.dataset.Config.Namespace || item.ID != p.dataset.ID(operation.Record) || item.Status != 200 && item.Status != 201 || item.Result != "created" && item.Result != "updated" || len(item.Error) > 0 || item.Shards.Failed != 0 || item.Shards.Successful < 1 {
				results[index] = failed(errors.New("Search bulk item lacks matching successful acknowledgement"), true)
				continue
			}
			outcome := Outcome{Status: Success, Applied: true, RequestBytes: uint64(len(p.dataset.Document(operation)))}
			results[index] = outcome
		}
		return results
	}
	var reply struct {
		Docs []struct {
			Index  string          `json:"_index"`
			ID     string          `json:"_id"`
			Found  bool            `json:"found"`
			Source json.RawMessage `json:"_source"`
			Error  json.RawMessage `json:"error"`
		} `json:"docs"`
	}
	if err := json.Unmarshal(body, &reply); err != nil || len(reply.Docs) != len(operations) {
		return batchFailure(operations, errors.New("Search bulk read count mismatch"))
	}
	for index, operation := range operations {
		item := reply.Docs[index]
		if !item.Found || item.Index != p.dataset.Config.Namespace || item.ID != p.dataset.ID(operation.Record) || len(item.Error) > 0 {
			results[index] = failed(errors.New("Search bulk read lacks matching document"), false)
			continue
		}
		if err := p.dataset.Validate(item.Source, operation); err != nil {
			results[index] = failed(err, false)
			continue
		}
		outcome := Outcome{Status: Success, ResponseBytes: uint64(len(item.Source))}
		results[index] = outcome
	}
	return results
}
