package fixture

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func executableFile(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "unused-weir")
	if err := os.WriteFile(path, []byte("#!/bin/sh\nexit 1\n"), 0700); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestValidateOptionsRejectsInvalidBeforeExternalWork(t *testing.T) {
	binary := executableFile(t)
	cases := []Options{
		{},
		{WeirBinary: filepath.Join(t.TempDir(), "missing")},
		{WeirBinary: binary, Backends: []string{"mongo", "mongo"}},
		{WeirBinary: binary, Backends: []string{"redis"}},
		{WeirBinary: binary, OwnerCount: -1},
		{WeirBinary: binary, OwnerCount: 9},
		{WeirBinary: binary, StoreConcurrency: 33},
		{WeirBinary: binary, BatchSize: 129},
		{WeirBinary: binary, BatchCollect: -1},
		{WeirBinary: binary, BatchCollect: 11 * time.Millisecond},
		{WeirBinary: binary, MongoBinary: filepath.Join(t.TempDir(), "missing-mongod")},
	}
	for _, options := range cases {
		if _, err := validateOptions(options); err == nil {
			t.Errorf("invalid options accepted: %+v", options)
		}
	}
	backends := []string{"mongo"}
	options := Options{WeirBinary: binary, Backends: backends}
	validated, err := validateOptions(options)
	if err != nil {
		t.Fatal(err)
	}
	backends[0] = "search"
	if validated.Backends[0] != "mongo" || validated.OwnerCount != 1 || validated.StoreConcurrency != 2 || validated.BatchSize != 32 || validated.BatchCollect != 0 {
		t.Fatalf("invalid defaults or retained caller slice: %+v", validated)
	}
	ctx := context.Background()
	missing := Options{}
	if _, err := Start(ctx, missing); err == nil {
		t.Fatal("missing binary accepted by Start")
	}
}

func TestRemoteDockerRejectedBeforeInvokingDocker(t *testing.T) {
	t.Setenv("DOCKER_HOST", "ssh://production.example")
	options := Options{WeirBinary: executableFile(t)}
	if _, err := Start(context.Background(), options); err == nil || !strings.Contains(err.Error(), "remote Docker") {
		t.Fatalf("expected remote rejection before Docker invocation, got %v", err)
	}
}

func TestLocalDockerHost(t *testing.T) {
	for _, host := range []string{"unix:///var/run/docker.sock", "unix:///home/test/.docker/run/docker.sock"} {
		if !localDockerHost(host) {
			t.Fatal("local socket rejected", host)
		}
	}
	for _, host := range []string{"", "tcp://127.0.0.1:2375", "ssh://localhost", "unix://remote/path", "unix:///tmp/socket\nssh://example", "unix:///tmp/socket\x00"} {
		if localDockerHost(host) {
			t.Fatal("unsafe Docker endpoint accepted", host)
		}
	}
}

func TestContainerOwnershipRequiresEveryIdentity(t *testing.T) {
	owner := "0123456789abcdef"
	owned := container{ID: strings.Repeat("a", 64), Name: "weir-tests-owned-mongo", Image: MongoImage, Backend: "mongo"}
	var state inspection
	state.ID, state.Name, state.Config.Image = owned.ID, "/"+owned.Name, owned.Image
	state.Config.Labels = map[string]string{ownerLabel: owner, "io.batchstream.weir-tests.backend": owned.Backend}
	if err := verifyOwned(state, owned, owner); err != nil {
		t.Fatal(err)
	}
	for _, mismatch := range []string{"id", "name", "image", "owner", "backend", "empty_owner"} {
		changed := state
		changed.Config.Labels = map[string]string{ownerLabel: owner, "io.batchstream.weir-tests.backend": owned.Backend}
		expectedOwner := owner
		switch mismatch {
		case "id":
			changed.ID = strings.Repeat("b", 64)
		case "name":
			changed.Name = "/unowned"
		case "image":
			changed.Config.Image = "mongo:latest"
		case "owner":
			changed.Config.Labels[ownerLabel] = "another-owner"
		case "backend":
			changed.Config.Labels["io.batchstream.weir-tests.backend"] = "search"
		case "empty_owner":
			expectedOwner = ""
		}
		if err := verifyOwned(changed, owned, expectedOwner); err == nil {
			t.Fatal("ownership mismatch accepted", mismatch)
		}
	}
}

func TestPublishedPortMustBeOneLoopbackBinding(t *testing.T) {
	valid := []portBinding{{HostIP: "127.0.0.1", HostPort: "54321"}}
	address, err := publishedLoopback(valid)
	if err != nil || address != "127.0.0.1:54321" {
		t.Fatal(address, err)
	}
	cases := [][]portBinding{
		nil,
		{{HostIP: "0.0.0.0", HostPort: "54321"}},
		{{HostIP: "127.0.0.1", HostPort: "0"}},
		{{HostIP: "127.0.0.1", HostPort: "65536"}},
		{{HostIP: "127.0.0.1", HostPort: "not-a-port"}},
		{{HostIP: "127.0.0.1", HostPort: "54321"}, {HostIP: "::1", HostPort: "54321"}},
	}
	for _, bindings := range cases {
		if _, err := publishedLoopback(bindings); err == nil {
			t.Fatal("unsafe port binding accepted", bindings)
		}
	}
}

func TestConfigurationSharesStoresWithoutRetainingDeadEndpoints(t *testing.T) {
	options := Options{Backends: []string{"mongo", "search"}, OwnerCount: 2, DiscoveryOnly: true, StoreConcurrency: 4, BatchSize: 16, BatchCollect: 3 * time.Millisecond}
	c := &Cluster{Directory: t.TempDir(), options: options, owner: "test", MongoURI: "mongodb://127.0.0.1:50001/?directConnection=true", SearchURL: "http://127.0.0.1:50002"}
	c.Nodes = []Node{
		{Application: "127.0.0.1:50100", Peer: "127.0.0.1:50101", Diagnostics: "127.0.0.1:50102", Owner: true},
		{Application: "127.0.0.1:50200", Peer: "127.0.0.1:50201", Diagnostics: "127.0.0.1:50202", Owner: true},
		{Application: "127.0.0.1:50300", Peer: "127.0.0.1:50301", Diagnostics: "127.0.0.1:50302"},
	}
	var groups []string
	for i, node := range c.Nodes {
		if err := c.writeConfiguration(i); err != nil {
			t.Fatal(err)
		}
		data, err := os.ReadFile(c.nodeFile(i, "config.json"))
		if err != nil {
			t.Fatal(err)
		}
		var configuration struct {
			Discovery struct {
				Group     string
				Seeds     []string
				Advertise []string
			}
		}
		if err := json.Unmarshal(data, &configuration); err != nil {
			t.Fatal(err)
		}
		if len(configuration.Discovery.Advertise) != 1 || configuration.Discovery.Advertise[0] != node.Application || len(configuration.Discovery.Seeds) != 2 {
			t.Fatalf("node published other owners' endpoints or omitted seeds: %s", data)
		}
		groups = append(groups, configuration.Discovery.Group)
		routes, err := os.ReadFile(c.nodeFile(i, "routes.json"))
		if err != nil {
			t.Fatal(err)
		}
		var routing struct{ Stores []json.RawMessage }
		if err := json.Unmarshal(routes, &routing); err != nil {
			t.Fatal(err)
		}
		if node.Owner && len(routing.Stores) != 2 || !node.Owner && len(routing.Stores) != 0 {
			t.Fatal("wrong Store ownership", string(routes))
		}
	}
	if groups[0] != groups[1] || groups[0] == groups[2] || c.Seed() != c.Nodes[2].Application {
		t.Fatal("incorrect replica groups or discovery seed")
	}
	first, _ := os.ReadFile(c.nodeFile(0, "routes.json"))
	second, _ := os.ReadFile(c.nodeFile(1, "routes.json"))
	if !bytes.Equal(first, second) {
		t.Fatal("owners have different Store definitions")
	}
}

func TestPinnedImagesMatchVersionLock(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("..", "..", "versions.json"))
	if err != nil {
		t.Fatal(err)
	}
	var lock struct {
		MongoImage    string `json:"mongodb_image"`
		MongoVersion  string `json:"mongodb_version"`
		SearchImage   string `json:"elasticsearch_image"`
		SearchVersion string `json:"elasticsearch_version"`
	}
	if err := json.Unmarshal(data, &lock); err != nil {
		t.Fatal(err)
	}
	if lock.MongoImage != MongoImage || lock.MongoVersion != MongoVersion || lock.SearchImage != SearchImage || lock.SearchVersion != SearchVersion {
		t.Fatalf("fixture image/version pins differ from versions.json: %+v", lock)
	}
}

func TestCloseEmptyIsRepeatableAndRejectsRestart(t *testing.T) {
	c := &Cluster{}
	if err := c.Close(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := c.Close(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := c.StopNode(context.Background(), 0); err == nil {
		t.Fatal("invalid node index accepted")
	}
	if err := c.RestartNode(context.Background(), 0); err == nil {
		t.Fatal("closed fixture restarted")
	}
}

func TestSearchReadinessResponsesAreBounded(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		switch request.URL.Path {
		case "/valid":
			_, _ = writer.Write([]byte(`{"acknowledged":true}`))
		case "/oversized":
			_, _ = writer.Write(bytes.Repeat([]byte("x"), (2<<20)+1))
		default:
			writer.WriteHeader(http.StatusServiceUnavailable)
			_, _ = writer.Write([]byte("unavailable"))
		}
	}))
	defer server.Close()
	client := loopbackHTTPClient()
	defer client.CloseIdleConnections()
	var result struct{ Acknowledged bool }
	valid := searchRequest{Method: http.MethodGet, URL: server.URL + "/valid", Result: &result}
	if err := searchJSON(context.Background(), client, valid); err != nil || !result.Acknowledged {
		t.Fatal(result, err)
	}
	for _, path := range []string{"/oversized", "/unavailable"} {
		request := searchRequest{Method: http.MethodGet, URL: server.URL + path, Result: &result}
		if err := searchJSON(context.Background(), client, request); err == nil {
			t.Fatal("invalid readiness response accepted", path)
		}
	}
	if client.Transport.(*http.Transport).Proxy != nil {
		t.Fatal("fixture HTTP must ignore ambient proxies")
	}
}

func TestBoundedCommandCapture(t *testing.T) {
	var output boundedOutput
	data := bytes.Repeat([]byte("a"), 128<<10)
	n, err := output.Write(data)
	if err != nil || n != len(data) || output.Len() != 64<<10 {
		t.Fatal(n, err, output.Len())
	}
}

func TestRestartReadinessDoesNotAcceptPreviousProcessAnnouncements(t *testing.T) {
	path := filepath.Join(t.TempDir(), "process.log")
	node := Node{Application: "127.0.0.1:50001", Peer: "127.0.0.1:50002", Diagnostics: "127.0.0.1:50003"}
	banner := "Weir listening on [127.0.0.1:50001 127.0.0.1:50002]; local execution and peer directory discovery\nDiagnostics listening on 127.0.0.1:50003\n"
	if err := os.WriteFile(path, []byte(banner), 0600); err != nil {
		t.Fatal(err)
	}
	if announced, err := processAnnounced(path, 0, node); err != nil || !announced {
		t.Fatal(announced, err)
	}
	if announced, err := processAnnounced(path, int64(len(banner)), node); err != nil || announced {
		t.Fatal("restart accepted stale announcement", announced, err)
	}
	node.Application = "127.0.0.1:51001"
	if announced, err := processAnnounced(path, 0, node); err != nil || announced {
		t.Fatal("readiness accepted a different process's endpoint", announced, err)
	}
}

func TestNativeMongoSelectionAndVersionMustBeExplicit(t *testing.T) {
	options := Options{Backends: []string{"mongo"}, MongoBinary: "/explicit/mongod"}
	if needsDocker(options) {
		t.Fatal("native Mongo-only fixture unexpectedly needs Docker")
	}
	options.Backends = append(options.Backends, "search")
	if !needsDocker(options) {
		t.Fatal("Search fixture omitted Docker")
	}
	options.Backends, options.MongoBinary = []string{"mongo"}, ""
	if !needsDocker(options) {
		t.Fatal("empty MongoBinary changed the Docker default")
	}
	if !pinnedMongoVersion([]byte("db version v8.0.32\nBuild Info: {}\n")) {
		t.Fatal("pinned native Mongo version rejected")
	}
	for _, output := range []string{"db version v8.0.31\n", "db version v8.0.32-dev\n", "db version v8.0.320\n", "error\ndb version v8.0.32\n"} {
		if pinnedMongoVersion([]byte(output)) {
			t.Fatal("unpinned binary output accepted", output)
		}
	}
}

func TestNativeMongoAnnouncementRequiresOwnedPort(t *testing.T) {
	path := filepath.Join(t.TempDir(), "mongo.log")
	log := `{"msg":"Waiting for connections","attr":{"port":50001}}` + "\n"
	if err := os.WriteFile(path, []byte(log), 0600); err != nil {
		t.Fatal(err)
	}
	announced, err := mongoAnnounced(path, "mongodb://127.0.0.1:50001/?directConnection=true")
	if err != nil || !announced {
		t.Fatal(announced, err)
	}
	announced, err = mongoAnnounced(path, "mongodb://127.0.0.1:50002/?directConnection=true")
	if err != nil || announced {
		t.Fatal("different local Mongo announcement accepted", announced, err)
	}
	if err := os.WriteFile(path, bytes.Repeat([]byte("x"), 65<<10), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := mongoAnnounced(path, "mongodb://127.0.0.1:50001"); err == nil {
		t.Fatal("unbounded Mongo announcement accepted")
	}
}

func TestStoppedProcessFailureIsPreservedInCloseAndReceipt(t *testing.T) {
	failure := errors.New("database process crashed")
	done := make(chan struct{})
	close(done)
	// This impossible Linux/macOS PID is never signalled: done is already closed.
	handle := &os.Process{Pid: 1 << 30}
	command := &exec.Cmd{Process: handle}
	p := &process{cmd: command, done: done, err: failure, logPath: "owned-mongo.log"}
	if err := stopProcess(context.Background(), p); !errors.Is(err, failure) || !strings.Contains(err.Error(), p.logPath) {
		t.Fatal("already exited process failure hidden", err)
	}
	c := &Cluster{Directory: t.TempDir(), nativeMongo: p}
	if err := c.Close(context.Background()); !errors.Is(err, failure) {
		t.Fatal("fixture close hid backend crash", err)
	}
	data, err := os.ReadFile(filepath.Join(c.Directory, "cleanup.json"))
	if err != nil || !bytes.Contains(data, []byte(failure.Error())) || !bytes.Contains(data, []byte(`"stopped": true`)) {
		t.Fatal("cleanup receipt hid crash or did not confirm exit", string(data), err)
	}
	p.err = nil
	if err := stopProcess(context.Background(), p); err != nil {
		t.Fatal("successful exited process rejected", err)
	}
}

func TestSingleOwnerConfigurationUsesAnEmptySeedList(t *testing.T) {
	options := Options{Backends: []string{"mongo"}, OwnerCount: 1, StoreConcurrency: 2, BatchSize: 32}
	c := &Cluster{Directory: t.TempDir(), options: options, owner: "test", MongoURI: "mongodb://127.0.0.1:50001/?directConnection=true"}
	node := Node{Application: "127.0.0.1:50100", Peer: "127.0.0.1:50101", Diagnostics: "127.0.0.1:50102", Owner: true}
	c.Nodes = []Node{node}
	if err := c.writeConfiguration(0); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(c.nodeFile(0, "config.json"))
	if err != nil || bytes.Contains(data, []byte(`"seeds": null`)) || !bytes.Contains(data, []byte(`"seeds": []`)) {
		t.Fatal("single-owner config emits unsupported null seeds", string(data), err)
	}
}
