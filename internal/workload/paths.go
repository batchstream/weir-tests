package workload

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	weir "github.com/batchstream/weir-go"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
	"go.mongodb.org/mongo-driver/v2/mongo/readpref"
	"go.mongodb.org/mongo-driver/v2/mongo/writeconcern"
)

type Status string

const (
	Success       Status = "success"
	Failed        Status = "error"
	Indeterminate Status = "indeterminate"
)

type Outcome struct {
	Status        Status
	Error         string
	RequestBytes  uint64
	ResponseBytes uint64
	Applied       bool
}

type Evidence struct {
	Protocol    string `json:"protocol"`
	Endpoint    string `json:"endpoint"`
	Version     string `json:"backend_version"`
	ByteScope   string `json:"byte_scope"`
	WritePolicy string `json:"write_policy"`
	RetryPolicy string `json:"retry_policy"`
	ClientBatch string `json:"client_batch"`
}

// Executor is the real implementation boundary between native database access
// and the public Weir SDK. Setup and independent verification are separate.
type Executor interface {
	Name() string
	Evidence() Evidence
	Execute(context.Context, Operation) Outcome
	ExecuteBatch(context.Context, []Operation) []Outcome
}

type Paths struct {
	Direct Executor
	Weir   Executor
	direct *directPath
	client *weir.Client
	owned  bool
}

type directPath struct {
	dataset *Dataset
	mongo   *mongo.Client
	search  *http.Client
	baseURL string
	info    Evidence
}

type weirPath struct {
	dataset *Dataset
	client  *weir.Client
	info    Evidence
}

func Open(ctx context.Context, dataset *Dataset) (*Paths, error) {
	if dataset == nil {
		return nil, errors.New("missing dataset")
	}
	direct, err := openDirect(ctx, dataset)
	if err != nil {
		return nil, err
	}
	paths := &Paths{Direct: direct, direct: direct}
	if err := direct.create(ctx); err != nil {
		return nil, errors.Join(err, paths.Close())
	}
	paths.owned = true
	open := weir.OpenOptions{Seed: dataset.Config.WeirSeed, Stores: []string{dataset.Config.StoreName}}
	client, err := weir.Open(ctx, open)
	if err != nil {
		cleanup, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		cleanupErr := paths.Cleanup(cleanup)
		closeErr := paths.Close()
		return nil, errors.Join(fmt.Errorf("initialize Weir: %w", err), cleanupErr, closeErr)
	}
	paths.client = client
	info := direct.info
	info.Protocol = "gRPC unary Read/Mutate batches via published Weir SDK"
	info.Endpoint = dataset.Config.WeirSeed
	info.RetryPolicy = "SDK never replays business requests"
	if dataset.Config.LuaMutations {
		info.WritePolicy = "one Lua AtomicTransform per RPC; revision computed from current document; server batches snapshot transactions or native OCC; no custom document fields"
	}
	via := &weirPath{dataset: dataset, client: client, info: info}
	paths.Weir = via
	return paths, nil
}

func openDirect(ctx context.Context, dataset *Dataset) (*directPath, error) {
	info := Evidence{ByteScope: "logical document bytes from APPLIED write acknowledgements and validated reads; excludes URI, commands, acknowledgement bodies, framing, compression and discovery; unavailable evidence contributes zero", ClientBatch: "one logical operation per client call; no client bulk API", RetryPolicy: "no application retries"}
	direct := &directPath{dataset: dataset, info: info}
	if dataset.Config.Backend == "mongo" {
		address, err := url.Parse(dataset.Config.MongoURI)
		if err != nil || address.Scheme != "mongodb" || address.Host == "" || address.User != nil {
			return nil, errors.New("requires a credential-free mongodb URI")
		}
		opts := options.Client().ApplyURI(dataset.Config.MongoURI).SetDirect(true).
			SetMaxPoolSize(uint64(dataset.Config.Concurrency)).SetMinPoolSize(0).
			SetRetryWrites(false).SetRetryReads(false).SetMaxAdaptiveRetries(0).
			SetEnableOverloadRetargeting(false).SetCompressors(nil).
			SetServerMonitoringMode(options.ServerMonitoringModePoll).
			SetReadPreference(readpref.Primary()).SetWriteConcern(writeconcern.Majority()).
			SetServerSelectionTimeout(2 * time.Second).SetConnectTimeout(2 * time.Second)
		client, err := mongo.Connect(opts)
		if err != nil {
			return nil, errors.New("direct Mongo client configuration rejected")
		}
		direct.mongo = client
		if err := client.Ping(ctx, readpref.Primary()); err != nil {
			_ = client.Disconnect(ctx)
			return nil, fmt.Errorf("direct Mongo ping: %w", err)
		}
		var build struct {
			Version string `bson:"version"`
		}
		command := bson.D{{Key: "buildInfo", Value: 1}}
		if err := client.Database("admin").RunCommand(ctx, command).Decode(&build); err != nil {
			_ = client.Disconnect(ctx)
			return nil, fmt.Errorf("Mongo version evidence: %w", err)
		}
		direct.info.Protocol = "MongoDB driver OP_MSG"
		direct.info.Endpoint = address.Host
		direct.info.Version = build.Version
		direct.info.WritePolicy = "ReplaceOne upsert=true; primary reads; w=majority; no compression; pool=worker concurrency"
		if dataset.Config.LuaMutations {
			direct.info.WritePolicy = "one snapshot transaction per business mutation: FindOne, compute revision toggle, ReplaceOne, majority commit; no client bulk; pool=worker concurrency"
		}
		direct.info.RetryPolicy = "retryReads=false; retryWrites=false; maxAdaptiveRetries=0"
		return direct, nil
	}
	address, err := url.Parse(dataset.Config.SearchURL)
	if err != nil || (address.Scheme != "http" && address.Scheme != "https") || address.Host == "" || address.User != nil || address.RawQuery != "" || (address.Path != "" && address.Path != "/") {
		return nil, errors.New("requires a credential-free Search root URL")
	}
	transport := &http.Transport{Proxy: nil, MaxConnsPerHost: dataset.Config.Concurrency, MaxIdleConnsPerHost: dataset.Config.Concurrency, MaxIdleConns: dataset.Config.Concurrency, DisableCompression: true, IdleConnTimeout: 30 * time.Second, ResponseHeaderTimeout: 10 * time.Second}
	direct.search = &http.Client{Transport: transport, CheckRedirect: rejectRedirect}
	direct.baseURL = strings.TrimSuffix(address.String(), "/")
	status, body, err := direct.request(ctx, http.MethodGet, "/", nil)
	if err != nil || status != http.StatusOK {
		_ = direct.close()
		return nil, errors.New("direct Search version query failed")
	}
	var root struct {
		Version struct {
			Number string `json:"number"`
		} `json:"version"`
	}
	if err := json.Unmarshal(body, &root); err != nil || root.Version.Number == "" {
		_ = direct.close()
		return nil, errors.New("invalid Search version evidence")
	}
	direct.info.Protocol = "Elasticsearch HTTP/1.1 JSON"
	direct.info.Endpoint = address.Host
	direct.info.Version = root.Version.Number
	direct.info.WritePolicy = "PUT _doc; pipeline=_none; refresh=false; wait_for_active_shards=1; timeout=1s; POST _mget?realtime=true with one ID; no compression; connections=worker concurrency"
	if dataset.Config.LuaMutations {
		direct.info.WritePolicy = "single-ID real-time _mget, compute revision toggle, PUT _doc with observed if_seq_no/if_primary_term; pipeline=_none; refresh=false; wait_for_active_shards=1; timeout=1s; no client bulk"
	}
	direct.info.RetryPolicy = "no application retries; business POST/PUT have no idempotency key and are not transparently replayed by http.Transport"
	return direct, nil
}

func rejectRedirect(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }

func (p *directPath) Name() string       { return "direct" }
func (p *directPath) Evidence() Evidence { return p.info }
func (p *weirPath) Name() string         { return "weir" }
func (p *weirPath) Evidence() Evidence   { return p.info }

func (p *directPath) create(ctx context.Context) error {
	if p.mongo != nil {
		db := p.mongo.Database(p.dataset.Config.Namespace)
		filter := bson.D{}
		collections, err := db.ListCollectionNames(ctx, filter)
		if err != nil || len(collections) != 0 {
			return errors.New("fresh Mongo namespace is unavailable; refusing adoption")
		}
		if err := db.CreateCollection(ctx, "records"); err != nil {
			return fmt.Errorf("exclusive Mongo collection creation: %w", err)
		}
		return nil
	}
	body := []byte(`{"settings":{"number_of_shards":1,"number_of_replicas":0},"mappings":{"properties":{"fixture_id":{"type":"keyword"},"revision":{"type":"integer"},"padding":{"type":"keyword","index":false,"doc_values":false}}}}`)
	status, _, err := p.request(ctx, http.MethodPut, "/"+p.dataset.Config.Namespace, body)
	if err != nil || status != http.StatusOK {
		return errors.New("exclusive Search index creation failed; refusing adoption")
	}
	return nil
}

func (p *Paths) Prepare(ctx context.Context, batchSize int) error {
	if !p.owned || batchSize < 1 || batchSize > 64 {
		return errors.New("dataset is not owned")
	}
	for start := 0; start < p.direct.dataset.Config.Records; start += batchSize {
		operations := make([]Operation, min(batchSize, p.direct.dataset.Config.Records-start))
		for index := range operations {
			operation := Operation{Record: start + index, Write: true}
			operations[index] = operation
		}
		var outcomes []Outcome
		if p.direct.mongo != nil {
			// Reset is deliberately idempotent. Timed native writes require
			// actual changes, but repeating revision zero during setup is valid.
			outcomes = p.direct.mongoBatch(ctx, operations, false)
		} else {
			outcomes = p.direct.searchBatch(ctx, operations)
		}
		if len(outcomes) != len(operations) {
			return errors.New("seed result count mismatch")
		}
		for index, outcome := range outcomes {
			if outcome.Status != Success {
				return fmt.Errorf("seed %d: %s", start+index, outcome.Error)
			}
		}
	}
	plan := &Plan{Expected: make([]int, p.direct.dataset.Config.Records)}
	return p.Verify(ctx, plan, batchSize)
}

// Verify reads every expected record through the independent direct path and
// checks collection/index count. Search refresh affects only postflight count.
func (p *Paths) Verify(ctx context.Context, plan *Plan, batchSize int) error {
	if !p.owned || plan == nil || len(plan.Expected) != p.direct.dataset.Config.Records || batchSize < 1 || batchSize > 64 {
		return errors.New("invalid verification plan or unowned dataset")
	}
	for start := 0; start < len(plan.Expected); start += batchSize {
		operations := make([]Operation, min(batchSize, len(plan.Expected)-start))
		for index := range operations {
			operation := Operation{Record: start + index, Revision: plan.Expected[start+index]}
			operations[index] = operation
		}
		outcomes := p.Direct.ExecuteBatch(ctx, operations)
		if len(outcomes) != len(operations) {
			return errors.New("postflight result count mismatch")
		}
		for index, outcome := range outcomes {
			if outcome.Status != Success {
				return fmt.Errorf("postflight %d: %s", start+index, outcome.Error)
			}
		}
	}
	var count int64
	if p.direct.mongo != nil {
		filter := bson.D{}
		var err error
		count, err = p.direct.mongo.Database(p.direct.dataset.Config.Namespace).Collection("records").CountDocuments(ctx, filter)
		if err != nil {
			return fmt.Errorf("postflight count: %w", err)
		}
	} else {
		status, _, err := p.direct.request(ctx, http.MethodPost, "/"+p.direct.dataset.Config.Namespace+"/_refresh", nil)
		if err != nil || status != http.StatusOK {
			return errors.New("postflight Search refresh failed")
		}
		status, body, err := p.direct.request(ctx, http.MethodGet, "/"+p.direct.dataset.Config.Namespace+"/_count", nil)
		var response struct {
			Count  int64 `json:"count"`
			Shards struct {
				Failed int `json:"failed"`
			} `json:"_shards"`
		}
		if err != nil || status != http.StatusOK || json.Unmarshal(body, &response) != nil || response.Shards.Failed != 0 {
			return errors.New("postflight Search count failed")
		}
		count = response.Count
	}
	if count != int64(p.direct.dataset.Config.Records) {
		return fmt.Errorf("persisted count=%d, expected=%d", count, p.direct.dataset.Config.Records)
	}
	return nil
}

func (p *Paths) Cleanup(ctx context.Context) error {
	if !p.owned {
		return nil
	}
	if p.direct.mongo != nil {
		err := p.direct.mongo.Database(p.direct.dataset.Config.Namespace).Collection("records").Drop(ctx)
		if err == nil {
			p.owned = false
		}
		return err
	}
	status, _, err := p.direct.request(ctx, http.MethodDelete, "/"+p.direct.dataset.Config.Namespace, nil)
	if err != nil || status != http.StatusOK {
		return errors.New("owned Search index cleanup failed")
	}
	p.owned = false
	return nil
}

func (p *Paths) Close() error {
	var sdkErr error
	if p.client != nil {
		sdkErr = p.client.Close()
	}
	return errors.Join(sdkErr, p.direct.close())
}

func (p *directPath) close() error {
	if p.search != nil {
		p.search.CloseIdleConnections()
	}
	if p.mongo != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		return p.mongo.Disconnect(ctx)
	}
	return nil
}

func (p *directPath) Execute(ctx context.Context, operation Operation) Outcome {
	if err := ctx.Err(); err != nil {
		return failed(err, false)
	}
	if operation.Write && p.dataset.Config.LuaMutations {
		if p.mongo != nil {
			return p.executeMongoTransform(ctx, operation)
		}
		return p.executeSearchTransform(ctx, operation)
	}
	if p.mongo != nil {
		return p.executeMongo(ctx, operation)
	}
	return p.executeSearch(ctx, operation)
}

func (p *directPath) executeMongo(ctx context.Context, operation Operation) Outcome {
	collection := p.mongo.Database(p.dataset.Config.Namespace).Collection("records")
	filter := bson.D{{Key: "_id", Value: p.dataset.ID(operation.Record)}}
	if operation.Write {
		document := bson.Raw(p.dataset.Document(operation))
		opts := options.Replace().SetUpsert(true)
		result, err := collection.ReplaceOne(ctx, filter, document, opts)
		if err != nil {
			var rejection mongo.WriteException
			uncertain := !errors.As(err, &rejection) || rejection.WriteConcernError != nil || len(rejection.WriteErrors) == 0
			return failed(err, uncertain)
		}
		if result == nil || !result.Acknowledged || result.MatchedCount+result.UpsertedCount != 1 {
			return failed(errors.New("missing acknowledged single-document replacement"), true)
		}
		if result.ModifiedCount+result.UpsertedCount != 1 {
			return failed(errors.New("timed native replacement did not change the record"), true)
		}
		outcome := Outcome{Status: Success, Applied: true, RequestBytes: uint64(len(document))}
		return outcome
	}
	result := collection.FindOne(ctx, filter)
	raw, err := result.Raw()
	if err != nil {
		return failed(err, false)
	}
	if err := p.dataset.Validate(raw, operation); err != nil {
		return failed(err, false)
	}
	outcome := Outcome{Status: Success, ResponseBytes: uint64(len(raw))}
	return outcome
}

func (p *directPath) executeSearch(ctx context.Context, operation Operation) Outcome {
	path := "/" + p.dataset.Config.Namespace + "/_doc/" + url.PathEscape(p.dataset.ID(operation.Record))
	method := http.MethodPost
	var payload []byte
	if operation.Write {
		method = http.MethodPut
		path += "?pipeline=_none&refresh=false&wait_for_active_shards=1&timeout=1s"
		payload = p.dataset.Document(operation)
	} else {
		path = "/" + p.dataset.Config.Namespace + "/_mget?realtime=true"
		request := struct {
			IDs []string `json:"ids"`
		}{IDs: []string{p.dataset.ID(operation.Record)}}
		var err error
		payload, err = json.Marshal(request)
		if err != nil {
			return failed(err, false)
		}
	}
	status, body, err := p.request(ctx, method, path, payload)
	if err != nil {
		return failed(err, operation.Write)
	}
	if status != http.StatusOK && (!operation.Write || status != http.StatusCreated) {
		uncertain := operation.Write && status != 400 && status != 404 && status != 409 && status != 429
		return failed(fmt.Errorf("Search HTTP status %d", status), uncertain)
	}
	if operation.Write {
		var reply struct {
			Index  string `json:"_index"`
			ID     string `json:"_id"`
			Result string `json:"result"`
			Shards struct {
				Successful int `json:"successful"`
				Failed     int `json:"failed"`
			} `json:"_shards"`
		}
		if json.Unmarshal(body, &reply) != nil || reply.Index != p.dataset.Config.Namespace || reply.ID != p.dataset.ID(operation.Record) || (reply.Result != "created" && reply.Result != "updated") || reply.Shards.Failed != 0 || reply.Shards.Successful < 1 {
			return failed(errors.New("invalid Search replacement acknowledgement"), true)
		}
		outcome := Outcome{Status: Success, Applied: true, RequestBytes: uint64(len(payload))}
		return outcome
	}
	var envelope struct {
		Docs []struct {
			Index  string          `json:"_index"`
			ID     string          `json:"_id"`
			Found  *bool           `json:"found"`
			Source json.RawMessage `json:"_source"`
			Error  json.RawMessage `json:"error"`
		} `json:"docs"`
	}
	if json.Unmarshal(body, &envelope) != nil || len(envelope.Docs) != 1 {
		return failed(errors.New("Search single-ID mget must return exactly one document"), false)
	}
	reply := envelope.Docs[0]
	if reply.Found == nil || !*reply.Found || reply.Index != p.dataset.Config.Namespace || reply.ID != p.dataset.ID(operation.Record) || len(reply.Error) != 0 {
		return failed(errors.New("invalid Search document response"), false)
	}
	if err := p.dataset.Validate(reply.Source, operation); err != nil {
		return failed(err, false)
	}
	outcome := Outcome{Status: Success, ResponseBytes: uint64(len(reply.Source))}
	return outcome
}

func (p *directPath) request(ctx context.Context, method, path string, body []byte) (int, []byte, error) {
	request, err := http.NewRequestWithContext(ctx, method, p.baseURL+path, bytes.NewReader(body))
	if err != nil {
		return 0, nil, err
	}
	// NewRequest makes bytes.Reader bodies replayable. Disable even the
	// transport's zero-byte write retry so each call offers one request only.
	request.GetBody = nil
	if len(body) != 0 {
		request.Header.Set("Content-Type", "application/json")
	}
	response, err := p.search.Do(request)
	if err != nil {
		return 0, nil, err
	}
	defer response.Body.Close()
	limit := int64(64*(p.dataset.Config.PayloadBytes+1024) + 64<<10)
	raw, err := io.ReadAll(io.LimitReader(response.Body, limit+1))
	if err != nil || int64(len(raw)) > limit {
		return response.StatusCode, nil, errors.New("Search response exceeds bounded document workspace or is incomplete")
	}
	return response.StatusCode, raw, nil
}

func (p *weirPath) Execute(ctx context.Context, operation Operation) Outcome {
	if err := ctx.Err(); err != nil {
		return failed(err, false)
	}
	if operation.Write {
		if p.dataset.Config.LuaMutations {
			program := &weir.ProgramTransform{Runtime: "lua.v1", Source: []byte(p.dataset.luaSource())}
			request := &weir.AtomicTransformRequest{Resource: p.dataset.Resource(operation.Record), Program: program}
			opts := weir.AtomicTransformOptions{StoreName: p.dataset.Config.StoreName, Request: request}
			result, err := p.client.AtomicTransform(ctx, opts)
			return mutationOutcome(result, len(program.Source), err)
		}
		document := &weir.Document{MediaType: p.dataset.MediaType(), Data: p.dataset.Document(operation)}
		request := &weir.WriteRequest{Resource: p.dataset.Resource(operation.Record), Document: document}
		opts := weir.WriteOptions{StoreName: p.dataset.Config.StoreName, Request: request}
		result, err := p.client.Put(ctx, opts)
		return mutationOutcome(result, len(document.Data), err)
	}
	request := &weir.ReadRequest{Resource: p.dataset.Resource(operation.Record)}
	opts := weir.ReadOneOptions{StoreName: p.dataset.Config.StoreName, Request: request}
	result, err := p.client.ReadOne(ctx, opts)
	if err != nil {
		return failed(err, false)
	}
	if result == nil || result.Document == nil || result.Failure != nil || result.Missing {
		return failed(errors.New("Weir Read lacks a successful document"), false)
	}
	if result.Document.MediaType != p.dataset.MediaType() {
		return failed(errors.New("Weir document media type mismatch"), false)
	}
	if err := p.dataset.Validate(result.Document.Data, operation); err != nil {
		return failed(err, false)
	}
	outcome := Outcome{Status: Success, ResponseBytes: uint64(len(result.Document.Data))}
	return outcome
}

func failed(err error, uncertain bool) Outcome {
	status := Failed
	if uncertain {
		status = Indeterminate
	}
	outcome := Outcome{Status: status, Error: err.Error()}
	return outcome
}
