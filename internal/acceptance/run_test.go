package acceptance

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	weir "github.com/batchstream/weir-go"
	"go.mongodb.org/mongo-driver/v2/bson"
)

func validConfig() Config {
	config := Config{ExpectedArch: "arm64", AllowRemote: true, ServerSource: "0c41296e5c6585480249f34d9d714fbda962c466", ServerOCI: "sha256:e559ba975b05369853ff7f7528a487044ad1ff0674cacd17867f7c7669aa438b", Namespace: "weir-accept-test", NamespaceUID: "11111111-1111-1111-1111-111111111111", PodName: "weir-0", PodUID: "22222222-2222-2222-2222-222222222222", RunOwner: "run12345", ApplicationSeeds: []string{"weir-0.weir-accept-test.svc.cluster.local:7777"}, MongoURI: "mongodb://mongo.weir-accept-test.svc.cluster.local:27017/?replicaSet=rs0", SearchURL: "http://search.weir-accept-test.svc.cluster.local:9200", MongoStore: "mongo", SearchStore: "search", OwnedDatasetPrefix: "weiraccept_run12345_"}
	return config
}

func TestOfflineConfigContract(t *testing.T) {
	config := validConfig()
	if err := Validate(config, "linux/arm64"); err != nil {
		t.Fatal(err)
	}
	config.ExpectedArch = "amd64"
	if err := Validate(config, "linux/amd64"); err != nil {
		t.Fatal(err)
	}
	if Validate(config, "linux/arm64") == nil || Validate(config, "darwin/amd64") == nil {
		t.Fatal("native runtime mismatch accepted")
	}
	changes := []func(*Config){
		func(c *Config) { c.AllowRemote = false }, func(c *Config) { c.ServerSource = "main" }, func(c *Config) { c.ServerOCI = "latest" },
		func(c *Config) { c.NamespaceUID = "unknown" }, func(c *Config) { c.PodUID = "" }, func(c *Config) { c.RunOwner = "bad/owner" },
		func(c *Config) { c.OwnedDatasetPrefix = "production_" }, func(c *Config) { c.MongoStore = c.SearchStore },
		func(c *Config) { c.ApplicationSeeds = nil }, func(c *Config) { c.ApplicationSeeds = []string{"host:7777", "host:7777"} },
		func(c *Config) { c.ApplicationSeeds = []string{"http://host:7777"} },
		func(c *Config) { c.MongoURI = "mongodb://user:password@host:27017/" },
		func(c *Config) { c.SearchURL = "http://user:password@host:9200" }, func(c *Config) { c.SearchURL = "http://host:9200/path" },
	}
	for index, change := range changes {
		config := validConfig()
		change(&config)
		if Validate(config, "linux/arm64") == nil {
			t.Fatalf("case %d accepted", index)
		}
	}
	raw, err := json.Marshal(validConfig())
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := Decode(strings.NewReader(string(raw)))
	if err != nil || decoded.RunOwner != "run12345" {
		t.Fatal("valid JSON rejected", err)
	}
	for _, raw := range []string{`{"unknown":1}`, `{"run_owner":"first","run_owner":"second"}`, `{} {}`, strings.Repeat(" ", MaxInputBytes+1)} {
		if _, err := Decode(strings.NewReader(raw)); err == nil {
			t.Fatal("invalid JSON accepted")
		}
	}
}

type fakeBackend struct {
	test          *testing.T
	fixture       *fakeFixture
	kind          string
	owned, closed bool
	value         int
	failPrepare   bool
}

func (f *fakeBackend) Owned() bool { return f.owned }
func (f *fakeBackend) Prepare(ctx context.Context) error {
	checkDeadline(f.test, ctx)
	f.owned = true
	f.fixture.events = append(f.fixture.events, "prepare_"+f.kind)
	if f.failPrepare {
		return errors.New("private://do-not-report")
	}
	return nil
}
func (f *fakeBackend) Oracle(ctx context.Context, value int) error {
	checkDeadline(f.test, ctx)
	if f.value != value {
		return errors.New("oracle")
	}
	return nil
}
func (f *fakeBackend) Close(ctx context.Context) error {
	checkDeadline(f.test, ctx)
	f.closed = true
	f.fixture.events = append(f.fixture.events, "close_"+f.kind)
	return nil
}
func (f *fakeBackend) Cleanup(ctx context.Context) error {
	checkDeadline(f.test, ctx)
	for _, backend := range f.fixture.backends {
		if !backend.closed {
			f.test.Fatal("delete before driver closure")
		}
	}
	for _, client := range f.fixture.clients {
		if !client.closed {
			f.test.Fatal("delete before SDK join")
		}
	}
	f.fixture.events = append(f.fixture.events, "delete_"+f.kind)
	if f.fixture.cleanupFailure {
		return errors.New("leftovers")
	}
	return nil
}

type fakeFixture struct {
	test                                            *testing.T
	targets                                         []*target
	backends                                        []*fakeBackend
	clients                                         []*fakeSDK
	events                                          []string
	puts                                            int
	rejection                                       *weir.MutationResult
	unknown, oversize, closeFailure, cleanupFailure bool
}

func fixture(t *testing.T) *fakeFixture { f := &fakeFixture{test: t}; return f }
func (f *fakeFixture) connect(config Config, name string) ([]*target, error) {
	for _, kind := range []string{"mongo", "search"} {
		b := &fakeBackend{test: f.test, fixture: f, kind: kind}
		target := &target{kind: kind, name: name, store: kind, owner: config.RunOwner, backend: b}
		f.backends = append(f.backends, b)
		f.targets = append(f.targets, target)
	}
	return f.targets, nil
}
func (f *fakeFixture) open(ctx context.Context, opts weir.OpenOptions) (sdk, error) {
	checkDeadline(f.test, ctx)
	if opts.ResolveTimeout != callBudget || len(opts.Stores) != 2 {
		f.test.Fatal("unbounded discovery")
	}
	client := &fakeSDK{fixture: f}
	f.clients = append(f.clients, client)
	return client, nil
}
func (f *fakeFixture) deps() dependencies {
	deps := dependencies{target: "linux/arm64", connect: f.connect, open: f.open}
	return deps
}
func checkDeadline(t *testing.T, ctx context.Context) {
	t.Helper()
	deadline, ok := ctx.Deadline()
	if !ok || time.Until(deadline) > callBudget {
		t.Fatal("ordinary call has no 3s bound")
	}
}

type fakeSDK struct {
	fixture *fakeFixture
	closed  bool
}

func (f *fakeSDK) target(store string) *target {
	for _, target := range f.fixture.targets {
		if target.store == store {
			return target
		}
	}
	f.fixture.test.Fatal("unknown store")
	return nil
}
func (f *fakeSDK) Put(ctx context.Context, opts weir.WriteOptions) (*weir.MutationResult, error) {
	checkDeadline(f.fixture.test, ctx)
	f.fixture.puts++
	t := f.target(opts.StoreName)
	b := t.backend.(*fakeBackend)
	b.value = 0
	if !t.validDocument(opts.Request.Document, 0) || opts.Request.Options.Visibility != weir.MutationQueryVisible {
		f.fixture.test.Fatal("Put contract changed")
	}
	if f.fixture.unknown {
		return nil, errors.New("http://credentials-do-not-report")
	}
	result := &weir.MutationResult{Outcome: weir.MutationApplied}
	return result, nil
}
func (f *fakeSDK) ReadOne(ctx context.Context, opts weir.ReadOneOptions) (*weir.ReadResult, error) {
	checkDeadline(f.fixture.test, ctx)
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}
	if strings.HasSuffix(opts.Request.Resource, "s:missing") {
		result := &weir.ReadResult{Missing: true}
		return result, nil
	}
	t := f.target(opts.StoreName)
	doc, err := t.document(t.backend.(*fakeBackend).value)
	if err != nil {
		return nil, err
	}
	result := &weir.ReadResult{Document: doc}
	return result, nil
}
func (f *fakeSDK) AtomicTransform(ctx context.Context, opts weir.AtomicTransformOptions) (*weir.MutationResult, error) {
	checkDeadline(f.fixture.test, ctx)
	if strings.Contains(string(opts.Request.Lua.Source), "acceptance_expected") {
		if string(opts.Request.Lua.Source) != `return function(current) current.value = 99; return weir.reject("acceptance_expected") end` {
			f.fixture.test.Fatal("negative case did not explicitly mutate then reject")
		}
		if f.fixture.rejection != nil {
			return f.fixture.rejection, nil
		}
		failure := &weir.Failure{Code: weir.FailurePreconditionFailed}
		result := &weir.MutationResult{Outcome: weir.MutationNotApplied, Failure: failure}
		return result, nil
	}
	f.target(opts.StoreName).backend.(*fakeBackend).value++
	result := &weir.MutationResult{Outcome: weir.MutationApplied}
	return result, nil
}
func (f *fakeSDK) Native(ctx context.Context, opts weir.NativeOptions) (*weir.NativeResult, error) {
	checkDeadline(f.fixture.test, ctx)
	head := &weir.NativeResponse{BodyContentType: "application/bson"}
	var body []byte
	if opts.StoreName == "mongo" {
		fields := bson.D{{Key: "ok", Value: float64(1)}, {Key: "n", Value: int32(1)}}
		body, _ = bson.Marshal(fields)
	} else {
		metadata := &weir.Document{ContentType: "application/http", Data: []byte("HTTP/1.1 200 OK\r\n\r\n")}
		head.Metadata = metadata
		head.BodyContentType = "application/json"
		body = []byte(`{"count":1,"_shards":{"total":1,"successful":1,"failed":0}}`)
	}
	if f.fixture.oversize {
		body = make([]byte, maxResponse+1)
	}
	err := opts.Consume(ctx, head, body)
	result := &weir.NativeResult{Response: head, Completion: weir.NativeResponseComplete}
	return result, err
}
func (f *fakeSDK) Close() error {
	f.closed = true
	f.fixture.events = append(f.fixture.events, "sdk_close")
	if f.fixture.closeFailure {
		return errors.New("closure unconfirmed")
	}
	return nil
}

func TestFiniteFlowAndActualJoinBeforeCleanup(t *testing.T) {
	f := fixture(t)
	config := validConfig()
	config.ApplicationSeeds = append(config.ApplicationSeeds, "weir-1.weir-accept-test.svc.cluster.local:7777")
	opts := RunOptions{Config: config}
	report := run(context.Background(), opts, f.deps())
	if !report.Passed || f.puts != 4 || len(f.clients) != 2 {
		t.Fatalf("finite functional contract failed: %+v", report)
	}
	if !strings.HasPrefix(report.Dataset, config.OwnedDatasetPrefix) || len(report.Checks) > 72 {
		t.Fatal("unbounded report/dataset")
	}
	if overallBudget > 120*time.Second || callBudget > 3*time.Second || workBudget >= overallBudget {
		t.Fatal("fixed budgets regressed")
	}
	for _, check := range report.Checks {
		if !check.Passed {
			t.Fatal(check)
		}
	}
}

func TestLuaRejectionCannotBeSatisfiedByCapacityOrUnavailability(t *testing.T) {
	cases := []struct {
		name    string
		code    weir.FailureCode
		outcome weir.MutationOutcome
	}{
		{name: "capacity_not_applied", code: weir.FailureResourceExhausted, outcome: weir.MutationNotApplied},
		{name: "capacity_not_started", code: weir.FailureResourceExhausted, outcome: weir.MutationNotStarted},
		{name: "unavailable_not_applied", code: weir.FailureUnavailable, outcome: weir.MutationNotApplied},
		{name: "unavailable_not_started", code: weir.FailureUnavailable, outcome: weir.MutationNotStarted},
		{name: "precondition_not_started", code: weir.FailurePreconditionFailed, outcome: weir.MutationNotStarted},
		{name: "internal_not_applied", code: weir.FailureInternal, outcome: weir.MutationNotApplied},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			f := fixture(t)
			failure := &weir.Failure{Code: test.code}
			f.rejection = &weir.MutationResult{Outcome: test.outcome, Failure: failure}
			opts := RunOptions{Config: validConfig()}
			report := run(context.Background(), opts, f.deps())
			if report.Passed || f.puts != 1 {
				t.Fatal("non-Lua rejection passed or caused replay")
			}
			found := false
			for _, check := range report.Checks {
				if check.Kind == "lua_rejected" {
					found = true
					if check.Passed || check.FailureCode != int32(test.code) || check.Outcome != int32(test.outcome) {
						t.Fatal("failure evidence lost")
					}
				}
			}
			if !found || !strings.Contains(strings.Join(f.events, ","), "delete_mongo") {
				t.Fatal("negative outcome or owned cleanup missing")
			}
		})
	}
}

func TestUnknownIsNotReplayedAndStillCleansOwnedData(t *testing.T) {
	f := fixture(t)
	f.unknown = true
	opts := RunOptions{Config: validConfig()}
	report := run(context.Background(), opts, f.deps())
	if report.Passed || f.puts != 1 {
		t.Fatal("failed mutation was retried or accepted")
	}
	found := false
	for _, check := range report.Checks {
		if check.Unknown {
			found = true
		}
	}
	if !found || !strings.Contains(strings.Join(f.events, ","), "delete_mongo") {
		t.Fatal("unknown or cleanup evidence missing")
	}
	raw, err := json.Marshal(report)
	if err != nil {
		t.Fatal(err)
	}
	for _, secret := range []string{"http://", "mongodb://", "svc.cluster.local", "credentials-do-not-report", "application_seeds", "mongo_uri", "search_url"} {
		if strings.Contains(string(raw), secret) {
			t.Fatal("report leaked endpoint/error")
		}
	}
}

func TestNativeBoundFailsAndDoesNotReplay(t *testing.T) {
	f := fixture(t)
	f.oversize = true
	opts := RunOptions{Config: validConfig()}
	report := run(context.Background(), opts, f.deps())
	if report.Passed || f.puts != 1 {
		t.Fatal("oversized Native response accepted or mutation replayed")
	}
}

func TestUnconfirmedSDKClosePreventsDeletion(t *testing.T) {
	f := fixture(t)
	f.closeFailure = true
	opts := RunOptions{Config: validConfig()}
	report := run(context.Background(), opts, f.deps())
	if report.Passed || strings.Contains(strings.Join(f.events, ","), "delete_") {
		t.Fatal("deleted before confirmed SDK closure")
	}
}

func TestInvalidConfigNeverOpensServices(t *testing.T) {
	f := fixture(t)
	config := validConfig()
	config.OwnedDatasetPrefix = "production_"
	opts := RunOptions{Config: config}
	report := run(context.Background(), opts, f.deps())
	if report.Passed || len(f.targets) != 0 || report.Supplied != nil {
		t.Fatal("invalid contract reached service boundary")
	}
}

func TestCallCancellationCannotBecomeALatePass(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	report := Report{}
	check := Check{Kind: "test_cancel"}
	passed := report.call(ctx, check, func(context.Context, *Check) bool { cancel(); return true })
	if passed || len(report.Checks) != 1 || report.Checks[0].ErrorCode != CodeCanceled {
		t.Fatal("canceled call counted as success")
	}
}

func TestCleanupFailureNeverBecomesAFunctionalPass(t *testing.T) {
	f := fixture(t)
	f.cleanupFailure = true
	opts := RunOptions{Config: validConfig()}
	report := run(context.Background(), opts, f.deps())
	if report.Passed || report.ErrorCode == CodeNone {
		t.Fatal("unverified cleanup passed")
	}
	failed := 0
	for _, check := range report.Checks {
		if check.Kind == "cleanup_verified" && !check.Passed {
			failed++
		}
	}
	if failed != 2 {
		t.Fatal("cleanup failures not retained")
	}
}

func TestPartialPreparationStillClosesAndCleansAmbiguousOwnedAttempts(t *testing.T) {
	f := fixture(t)
	deps := f.deps()
	deps.connect = func(config Config, name string) ([]*target, error) {
		targets, err := f.connect(config, name)
		f.backends[1].failPrepare = true
		return targets, err
	}
	opts := RunOptions{Config: validConfig()}
	report := run(context.Background(), opts, deps)
	if report.Passed || f.puts != 0 || len(f.clients) != 0 {
		t.Fatal("functional work ran after preparation failure")
	}
	if !strings.Contains(strings.Join(f.events, ","), "delete_search") {
		t.Fatal("ambiguous owned create attempt was abandoned")
	}
}

func TestDocumentOracleRejectsWrongFieldsAndTypes(t *testing.T) {
	for _, kind := range []string{"mongo", "search"} {
		target := &target{kind: kind, owner: "run12345"}
		doc, err := target.document(1)
		if err != nil || !target.validDocument(doc, 1) || target.validDocument(doc, 2) {
			t.Fatal("typed oracle mismatch")
		}
		doc.ContentType = "text/plain"
		if target.validDocument(doc, 1) {
			t.Fatal("wrong type accepted")
		}
	}
	for _, raw := range []string{`{"count":1,"_shards":{"total":1,"successful":0,"failed":1}}`, `{"count":2,"_shards":{"total":1,"successful":1,"failed":0}}`, `{"count":1}`} {
		if validSearchCount([]byte(raw)) {
			t.Fatal("partial/wrong count accepted")
		}
	}
}
