package acceptance

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"net/http"
	"runtime"
	"runtime/debug"
	"time"

	weir "github.com/batchstream/weir-go"
	"go.mongodb.org/mongo-driver/v2/bson"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

const callBudget = 3 * time.Second
const overallBudget = 120 * time.Second
const workBudget = 75 * time.Second

// Codes are deliberately coarse: never serialize transport errors, addresses or payloads.
const (
	CodeNone = iota
	CodeFailure
	CodeDeadline
	CodeCanceled
	CodeInput
)

type Build struct {
	Go             string `json:"go"`
	Target         string `json:"target"`
	SDK            string `json:"sdk"`
	Protocol       string `json:"protocol"`
	VCS            string `json:"vcs_revision"`
	Modified       *bool  `json:"vcs_modified,omitempty"`
	InjectedSource string `json:"injected_source"`
}

type Supplied struct {
	ServerSource string `json:"server_source"`
	ServerOCI    string `json:"server_oci"`
	Namespace    string `json:"namespace"`
	NamespaceUID string `json:"namespace_uid"`
	PodName      string `json:"pod_name"`
	PodUID       string `json:"pod_uid"`
	RunOwner     string `json:"run_owner"`
}

type Check struct {
	Kind        string `json:"kind"`
	Backend     string `json:"backend,omitempty"`
	Seed        int    `json:"seed,omitempty"`
	Passed      bool   `json:"passed"`
	ErrorCode   int    `json:"error_code"`
	Outcome     int32  `json:"mutation_outcome,omitempty"`
	Unknown     bool   `json:"mutation_unknown,omitempty"`
	FailureCode int32  `json:"failure_code,omitempty"`
	Completion  int32  `json:"native_completion,omitempty"`
	Bytes       int    `json:"response_bytes,omitempty"`
	ElapsedMS   int64  `json:"elapsed_ms"`
}

type Report struct {
	Schema    int       `json:"schema"`
	Mode      string    `json:"mode"`
	Passed    bool      `json:"passed"`
	ErrorCode int       `json:"error_code"`
	ElapsedMS int64     `json:"elapsed_ms"`
	Build     Build     `json:"runner_build"`
	Supplied  *Supplied `json:"caller_asserted_provenance,omitempty"`
	Dataset   string    `json:"owned_dataset_base,omitempty"`
	SeedCount int       `json:"seed_count"`
	Checks    []Check   `json:"checks"`
}

func BuildInfo(injected string) Build {
	if !sourcePattern.MatchString(injected) {
		injected = "unknown"
	}
	build := Build{Go: runtime.Version(), Target: runtime.GOOS + "/" + runtime.GOARCH, SDK: "unknown", Protocol: "unknown", VCS: "unknown", InjectedSource: injected}
	if info, ok := debug.ReadBuildInfo(); ok {
		for _, dep := range info.Deps {
			if dep.Replace != nil {
				continue
			}
			if dep.Path == "github.com/batchstream/weir-go" {
				build.SDK = dep.Version
			}
			if dep.Path == "github.com/batchstream/weir-protocol" {
				build.Protocol = dep.Version
			}
		}
		for _, setting := range info.Settings {
			if setting.Key == "vcs.revision" && sourcePattern.MatchString(setting.Value) {
				build.VCS = setting.Value
			}
			if setting.Key == "vcs.modified" {
				modified := setting.Value == "true"
				build.Modified = &modified
			}
		}
	}
	return build
}

type sdk interface {
	Put(context.Context, weir.WriteOptions) (*weir.MutationResult, error)
	ReadOne(context.Context, weir.ReadOneOptions) (*weir.ReadResult, error)
	AtomicTransform(context.Context, weir.AtomicTransformOptions) (*weir.MutationResult, error)
	Native(context.Context, weir.NativeOptions) (*weir.NativeResult, error)
	Close() error
}

type dependencies struct {
	target  string
	connect func(Config, string) ([]*target, error)
	open    func(context.Context, weir.OpenOptions) (sdk, error)
}

type RunOptions struct {
	Config         Config
	InjectedSource string
}

func Run(ctx context.Context, options RunOptions) Report {
	deps := dependencies{target: runtime.GOOS + "/" + runtime.GOARCH, connect: connect,
		open: func(ctx context.Context, opts weir.OpenOptions) (sdk, error) { return weir.Open(ctx, opts) }}
	return run(ctx, options, deps)
}

func run(ctx context.Context, options RunOptions, deps dependencies) Report {
	start := time.Now()
	r := Report{Schema: 1, Mode: "functional", Build: BuildInfo(options.InjectedSource), Checks: make([]Check, 0, 72)}
	if Validate(options.Config, deps.target) != nil {
		r.ErrorCode = CodeInput
		return r
	}
	cfg := options.Config
	r.Supplied = &Supplied{ServerSource: cfg.ServerSource, ServerOCI: cfg.ServerOCI, Namespace: cfg.Namespace, NamespaceUID: cfg.NamespaceUID, PodName: cfg.PodName, PodUID: cfg.PodUID, RunOwner: cfg.RunOwner}
	r.SeedCount = len(cfg.ApplicationSeeds)
	overall, cancel := context.WithTimeout(context.WithoutCancel(ctx), overallBudget)
	defer cancel()
	work, stop := context.WithTimeout(overall, workBudget)
	defer stop()
	stopParent := context.AfterFunc(ctx, stop)
	defer stopParent()
	var nonce [8]byte
	if _, err := rand.Read(nonce[:]); err != nil {
		r.ErrorCode = CodeFailure
		return r
	}
	r.Dataset = cfg.OwnedDatasetPrefix + hex.EncodeToString(nonce[:])
	targets, err := deps.connect(cfg, r.Dataset)
	if err != nil {
		r.ErrorCode = errorCode(err)
		return r
	}
	ok := true
	for _, t := range targets {
		check := Check{Kind: "prepare", Backend: t.kind}
		passed := r.call(work, check, func(call context.Context, result *Check) bool {
			err := t.backend.Prepare(call)
			result.ErrorCode = errorCode(err)
			return err == nil
		})
		if !passed {
			ok = false
			break
		}
	}
	for seedIndex, seed := range cfg.ApplicationSeeds {
		if !ok {
			break
		}
		var client sdk
		check := Check{Kind: "resolve_open", Seed: seedIndex + 1}
		ok = r.call(work, check, func(call context.Context, result *Check) bool {
			opts := weir.OpenOptions{Seed: seed, Stores: []string{cfg.MongoStore, cfg.SearchStore}, ResolveTimeout: callBudget, RefreshInterval: time.Hour}
			var err error
			client, err = deps.open(call, opts)
			result.ErrorCode = errorCode(err)
			return err == nil && client != nil
		})
		if client == nil {
			break
		}
		if ok {
			for _, t := range targets {
				opts := exerciseOptions{client: client, target: t, report: &r, seed: seedIndex + 1}
				if !exercise(work, opts) {
					ok = false
					break
				}
			}
		}
		check = Check{Kind: "sdk_close", Seed: seedIndex + 1}
		closed := r.call(overall, check, func(_ context.Context, result *Check) bool {
			err := client.Close()
			result.ErrorCode = errorCode(err)
			return err == nil
		})
		if !closed {
			ok = false
		}
		if !closed { // Never delete data while an SDK closure is unconfirmed.
			stop()
			closeTargets(overall, targets, &r)
			r.ErrorCode = CodeFailure
			r.ElapsedMS = time.Since(start).Milliseconds()
			return r
		}
	}
	stop()
	closed := closeTargets(overall, targets, &r)
	if !closed {
		ok = false
	}
	if closed {
		for _, t := range targets {
			kind := "cleanup_verified"
			if !t.backend.Owned() {
				kind = "cleanup_not_owned"
			}
			check := Check{Kind: kind, Backend: t.kind}
			passed := r.call(overall, check, func(call context.Context, result *Check) bool {
				err := t.backend.Cleanup(call)
				result.ErrorCode = errorCode(err)
				return err == nil
			})
			if !passed {
				ok = false
			}
		}
	}
	r.Passed = ok
	if !ok {
		r.ErrorCode = CodeFailure
	}
	r.ElapsedMS = time.Since(start).Milliseconds()
	return r
}

func closeTargets(ctx context.Context, targets []*target, report *Report) bool {
	ok := true
	for _, t := range targets {
		check := Check{Kind: "driver_close", Backend: t.kind}
		passed := report.call(ctx, check, func(call context.Context, result *Check) bool {
			err := t.backend.Close(call)
			result.ErrorCode = errorCode(err)
			return err == nil
		})
		if !passed {
			ok = false
		}
	}
	return ok
}

func (r *Report) call(ctx context.Context, check Check, fn func(context.Context, *Check) bool) bool {
	call, cancel := context.WithTimeout(ctx, callBudget)
	defer cancel()
	start := time.Now()
	if call.Err() != nil {
		check.ErrorCode = errorCode(call.Err())
	} else {
		check.Passed = fn(call, &check)
	}
	deadline, _ := call.Deadline()
	if !time.Now().Before(deadline) {
		check.Passed = false
		check.ErrorCode = CodeDeadline
	} else if err := call.Err(); err != nil {
		check.Passed = false
		check.ErrorCode = errorCode(err)
	}
	check.ElapsedMS = time.Since(start).Milliseconds()
	r.Checks = append(r.Checks, check)
	return check.Passed
}

func errorCode(err error) int {
	if err == nil {
		return CodeNone
	}
	if errors.Is(err, context.Canceled) || status.Code(err) == codes.Canceled {
		return CodeCanceled
	}
	if errors.Is(err, context.DeadlineExceeded) || status.Code(err) == codes.DeadlineExceeded {
		return CodeDeadline
	}
	return CodeFailure
}

type exerciseOptions struct {
	client sdk
	target *target
	report *Report
	seed   int
}

// Each explicit seed initializes a client; this does not simulate node loss or prove HA.
func exercise(ctx context.Context, opts exerciseOptions) bool {
	client, t, report, seed := opts.client, opts.target, opts.report, opts.seed
	for _, kind := range []string{"missing_read", "put", "lua", "typed_read", "lua_rejected", "typed_read_after_rejection", "native_count", "cancel_admission", "independent_oracle"} {
		check := Check{Kind: kind, Backend: t.kind, Seed: seed}
		passed := report.call(ctx, check, func(call context.Context, result *Check) bool {
			switch kind {
			case "put":
				doc, err := t.document(0)
				if err != nil {
					result.ErrorCode = errorCode(err)
					return false
				}
				visibility := &weir.MutationOptions{Visibility: weir.MutationQueryVisible}
				request := &weir.WriteRequest{Resource: t.resource("record"), Document: doc, Options: visibility}
				opts := weir.WriteOptions{StoreName: t.store, Request: request}
				mutation, err := client.Put(call, opts)
				return mutationCheck(result, mutation, err, false)
			case "lua", "lua_rejected":
				source := `return function(current) current.value = current.value + 1; return current end`
				if kind == "lua_rejected" {
					source = `return function(current) current.value = 99; return weir.reject("acceptance_expected") end`
				}
				program := &weir.LuaTransform{Source: []byte(source)}
				visibility := &weir.MutationOptions{Visibility: weir.MutationQueryVisible}
				request := &weir.AtomicTransformRequest{Resource: t.resource("record"), Lua: program, Options: visibility}
				opts := weir.AtomicTransformOptions{StoreName: t.store, Request: request}
				mutation, err := client.AtomicTransform(call, opts)
				return mutationCheck(result, mutation, err, kind == "lua_rejected")
			case "missing_read", "typed_read", "typed_read_after_rejection", "cancel_admission":
				id := "record"
				if kind == "missing_read" {
					id = "missing"
				}
				request := &weir.ReadRequest{Resource: t.resource(id)}
				opts := weir.ReadOneOptions{StoreName: t.store, Request: request}
				if kind == "cancel_admission" {
					var cancel context.CancelFunc
					call, cancel = context.WithCancel(call)
					cancel()
				}
				read, err := client.ReadOne(call, opts)
				result.ErrorCode = errorCode(err)
				if kind == "cancel_admission" {
					return result.ErrorCode == CodeCanceled && read == nil
				}
				if err != nil || read == nil {
					return false
				}
				if read.Failure != nil {
					result.FailureCode = int32(read.Failure.Code)
					return false
				}
				if kind == "missing_read" {
					return read.Missing && read.Document == nil
				}
				return !read.Missing && t.validDocument(read.Document, 1)
			case "native_count":
				return nativeCount(call, client, t, result)
			case "independent_oracle":
				err := t.backend.Oracle(call, 1)
				result.ErrorCode = errorCode(err)
				return err == nil
			}
			return false
		})
		if !passed {
			return false
		}
	}
	return true
}

func mutationCheck(check *Check, result *weir.MutationResult, err error, reject bool) bool {
	check.ErrorCode = errorCode(err)
	check.Unknown = result == nil
	if result == nil {
		return false
	}
	check.Outcome = int32(result.Outcome)
	check.Unknown = result.Outcome == weir.MutationUnknown
	if result.Failure != nil {
		check.FailureCode = int32(result.Failure.Code)
	}
	if reject {
		return err == nil && result.Failure != nil && result.Failure.Code == weir.FailurePreconditionFailed && result.Outcome == weir.MutationNotApplied
	}
	return err == nil && result.Failure == nil && result.Outcome == weir.MutationApplied
}

func nativeCount(ctx context.Context, client sdk, t *target, check *Check) bool {
	var request *weir.NativeRequest
	if t.kind == "mongo" {
		command := bson.D{{Key: "count", Value: "records"}}
		raw, err := bson.Marshal(command)
		if err != nil {
			return false
		}
		doc := &weir.Document{ContentType: "application/bson", Data: raw}
		request = &weir.NativeRequest{Resource: t.name, Request: doc}
	} else {
		httpRequest, err := http.NewRequest(http.MethodGet, "http://weir.invalid/_count", nil)
		if err != nil {
			return false
		}
		request, err = weir.NewHTTPNativeRequest(t.name, httpRequest)
		if err != nil {
			return false
		}
	}
	body := make([]byte, 0, 1024)
	opts := weir.NativeOptions{StoreName: t.store, Request: request}
	opts.Consume = func(call context.Context, _ *weir.NativeResponse, chunk []byte) error {
		if err := call.Err(); err != nil {
			return err
		}
		if len(chunk) > maxResponse-len(body) {
			return errors.New("native_response_bound")
		}
		body = append(body, chunk...)
		return nil
	}
	result, err := client.Native(ctx, opts)
	check.ErrorCode = errorCode(err)
	check.Bytes = len(body)
	if result == nil {
		return false
	}
	check.Completion = int32(result.Completion)
	if result.Failure != nil {
		check.FailureCode = int32(result.Failure.Code)
	}
	if err != nil || result.Failure != nil || result.Completion != weir.NativeResponseComplete || result.Response == nil {
		return false
	}
	if t.kind == "mongo" {
		var fields bson.M
		return bson.Unmarshal(body, &fields) == nil && number(fields["ok"]) == 1 && number(fields["n"]) == 1
	}
	metadata, err := weir.ParseHTTPNativeResponse(result.Response)
	return err == nil && metadata.StatusCode == http.StatusOK && validSearchCount(body)
}
