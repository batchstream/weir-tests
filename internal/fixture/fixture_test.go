package fixture

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
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
		{WeirBinary: binary, Replicas: -1},
		{WeirBinary: binary, Replicas: 9},
		{WeirBinary: binary, BatchMaxWait: time.Millisecond},
		{WeirBinary: binary, BatchMaxWait: -time.Nanosecond},
		{WeirBinary: binary, BatchMaxWait: time.Second + time.Nanosecond},
		{WeirBinary: binary, BatchSize: -1},
		{WeirBinary: binary, BatchSize: 32, BatchBytes: -1},
		{WeirBinary: binary, BatchBytes: 1},
		{WeirBinary: binary, BatchSize: 32, BatchBytes: (32 << 20) + 1},
		{WeirBinary: binary, BatchSize: 1025},
		{WeirBinary: binary, MaxInFlight: 4},
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
	if validated.Backends[0] != "mongo" || validated.Replicas != 1 || validated.BatchSize != 0 {
		t.Fatalf("invalid defaults or retained caller slice: %+v", validated)
	}
	large := Options{WeirBinary: binary, BatchSize: 513}
	if _, err := validateOptions(large); err != nil {
		t.Fatal("valid physical batch greater than 128 rejected:", err)
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

func TestEveryReplicaHostsTheSameStoresOnItsOwnListener(t *testing.T) {
	options := Options{Backends: []string{"mongo", "search"}, Replicas: 2, BatchSize: 16}
	c := &Cluster{Directory: t.TempDir(), options: options, owner: "test", MongoURI: "mongodb://127.0.0.1:50001/?directConnection=true", SearchURL: "http://127.0.0.1:50002"}
	c.Nodes = []Node{
		{Application: "127.0.0.1:50100", Diagnostics: "127.0.0.1:50102"},
		{Application: "127.0.0.1:50200", Diagnostics: "127.0.0.1:50202"},
	}
	for i, node := range c.Nodes {
		if err := c.writeConfiguration(i); err != nil {
			t.Fatal(err)
		}
		data, err := os.ReadFile(c.ConfigPath(i))
		if err != nil {
			t.Fatal(err)
		}
		var configuration struct {
			Listeners map[string]any `json:"listeners"`
			Stores    []struct {
				Name     string         `json:"name"`
				MongoDB  map[string]any `json:"mongodb"`
				Search   map[string]any `json:"search"`
				Batching struct {
					MaxOperations int `json:"max_operations"`
				} `json:"batching"`
			} `json:"stores"`
		}
		if err := json.Unmarshal(data, &configuration); err != nil {
			t.Fatal(err)
		}
		listeners := map[string]any{"application": node.Application, "diagnostics": node.Diagnostics}
		if !reflect.DeepEqual(configuration.Listeners, listeners) {
			t.Fatalf("node configuration differs from its own listeners: %s", data)
		}
		if len(configuration.Stores) != 2 {
			t.Fatal("replica does not host every Store", string(data))
		}
		for position, store := range configuration.Stores {
			if store.Name != options.Backends[position] || store.Batching.MaxOperations != 16 {
				t.Fatal("replica Store did not receive the current batching override", string(data))
			}
		}
		if configuration.Stores[0].MongoDB["uri"] != c.MongoURI || configuration.Stores[1].Search["url"] != c.SearchURL {
			t.Fatal("replica Stores do not name their backends", string(data))
		}
	}
	if c.Address() != c.Nodes[0].Application {
		t.Fatal("cluster address is not the first replica")
	}
	var replicas [2]map[string]any
	for i := range replicas {
		data, err := os.ReadFile(c.ConfigPath(i))
		if err != nil {
			t.Fatal(err)
		}
		if err := json.Unmarshal(data, &replicas[i]); err != nil {
			t.Fatal(err)
		}
	}
	if reflect.DeepEqual(replicas[0]["listeners"], replicas[1]["listeners"]) {
		t.Fatal("replicas share listeners")
	}
	delete(replicas[0], "listeners")
	delete(replicas[1], "listeners")
	if !reflect.DeepEqual(replicas[0], replicas[1]) {
		t.Fatal("replicas must differ only in their listeners")
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
	node := Node{Application: "127.0.0.1:50001", Diagnostics: "127.0.0.1:50003"}
	banner := "Weir listening on 127.0.0.1:50001\nDiagnostics listening on 127.0.0.1:50003\n"
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

func TestStoppedProcessFailureIsPreservedInCloseAndCleanupRecord(t *testing.T) {
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
		t.Fatal("cleanup record hid crash or did not confirm exit", string(data), err)
	}
	p.err = nil
	if err := stopProcess(context.Background(), p); err != nil {
		t.Fatal("successful exited process rejected", err)
	}
}

func TestOrdinaryConfigurationLeavesServerDefaultsUnchanged(t *testing.T) {
	input := Options{WeirBinary: executableFile(t), Backends: []string{"mongo"}, Replicas: 1}
	options, err := validateOptions(input)
	if err != nil {
		t.Fatal(err)
	}
	c := &Cluster{Directory: t.TempDir(), options: options, owner: "test", MongoURI: "mongodb://127.0.0.1:50001/?directConnection=true"}
	node := Node{Application: "127.0.0.1:50100", Diagnostics: "127.0.0.1:50102"}
	c.Nodes = []Node{node}
	if err := c.writeConfiguration(0); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(c.ConfigPath(0))
	if err != nil || bytes.Contains(data, []byte(`"batching"`)) || bytes.Contains(data, []byte(`"shutdown_timeout"`)) {
		t.Fatal("ordinary fixture must leave server defaults unchanged", string(data), err)
	}
}

func TestContainerCrashIsPreservedWhileOwnedCleanupCompletes(t *testing.T) {
	cases := []struct {
		name              string
		running           bool
		exitCode          int
		stateError        string
		wantFailure       bool
		previouslyStopped bool
	}{
		{name: "already_crashed", exitCode: 137, wantFailure: true},
		{name: "already_failed", stateError: "runtime failure", wantFailure: true},
		{name: "running_graceful_stop", running: true},
		{name: "already_stopped_cleanly"},
		{name: "previous_successful_own_stop", exitCode: 137, previouslyStopped: true},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			directory := t.TempDir()
			cliDirectory := filepath.Join(directory, "bin")
			if err := os.Mkdir(cliDirectory, 0700); err != nil {
				t.Fatal(err)
			}
			statePath := filepath.Join(directory, "state.json")
			commandsPath := filepath.Join(directory, "commands.log")
			owner := "test-owner"
			owned := container{ID: strings.Repeat("a", 64), Name: "weir-tests-owned-mongo", Image: MongoImage, Backend: "mongo", stopped: test.previouslyStopped}
			var state inspection
			state.ID, state.Name, state.Config.Image = owned.ID, "/"+owned.Name, owned.Image
			state.Config.Labels = map[string]string{ownerLabel: owner, "io.batchstream.weir-tests.backend": owned.Backend}
			state.State.Running, state.State.ExitCode, state.State.Error = test.running, test.exitCode, test.stateError
			states := []inspection{state}
			if err := writeJSON(statePath, states); err != nil {
				t.Fatal(err)
			}
			script := `#!/bin/sh
shift 4
printf '%s\n' "$*" >> "$WEIR_FIXTURE_FAKE_DOCKER_COMMANDS"
case "$1" in
  container) cat "$WEIR_FIXTURE_FAKE_DOCKER_STATE" ;;
  logs) printf '%s\n' 'owned backend log' ;;
  stop) printf '%s\n' 'stopped' ;;
  rm) printf '%s\n' 'removed' ;;
  *) exit 2 ;;
esac
`
			if err := os.WriteFile(filepath.Join(cliDirectory, "docker"), []byte(script), 0700); err != nil {
				t.Fatal(err)
			}
			t.Setenv("PATH", cliDirectory+string(os.PathListSeparator)+os.Getenv("PATH"))
			t.Setenv("WEIR_FIXTURE_FAKE_DOCKER_STATE", statePath)
			t.Setenv("WEIR_FIXTURE_FAKE_DOCKER_COMMANDS", commandsPath)
			c := &Cluster{Directory: directory, owner: owner, dockerHost: "unix:///unused-fixture.sock", containers: []container{owned}}
			err := c.Close(context.Background())
			if (err != nil) != test.wantFailure {
				t.Fatal("wrong cleanup outcome", err)
			}
			if !c.containers[0].removed {
				t.Fatal("owned crashed container was not removed")
			}
			commands, readErr := os.ReadFile(commandsPath)
			if readErr != nil || !bytes.Contains(commands, []byte("rm --volumes "+owned.ID)) {
				t.Fatal("cleanup did not remove exact owned ID", string(commands), readErr)
			}
			if bytes.Contains(commands, []byte("stop --time 10 "+owned.ID)) != test.running {
				t.Fatal("cleanup stop does not match initial running state", string(commands))
			}
			if err := c.Close(context.Background()); (err != nil) != test.wantFailure {
				t.Fatal("repeat cleanup lost backend failure", err)
			}
			data, readErr := os.ReadFile(filepath.Join(directory, "cleanup.json"))
			if readErr != nil {
				t.Fatal(readErr)
			}
			var cleanup struct {
				Errors     string
				Containers []struct{ Removed bool }
			}
			if err := json.Unmarshal(data, &cleanup); err != nil {
				t.Fatal(err)
			}
			if len(cleanup.Containers) != 1 || !cleanup.Containers[0].Removed || (cleanup.Errors != "") != test.wantFailure {
				t.Fatal("cleanup record hides backend crash or owned removal", string(data))
			}
		})
	}
}

func TestDockerWarningsDoNotPolluteMachineReadableStdout(t *testing.T) {
	directory := t.TempDir()
	cliDirectory := filepath.Join(directory, "bin")
	if err := os.Mkdir(cliDirectory, 0700); err != nil {
		t.Fatal(err)
	}
	id := strings.Repeat("a", 64)
	t.Setenv("WEIR_FIXTURE_FAKE_DOCKER_ID", id)
	script := `#!/bin/sh
shift 4
case "$1" in
  create)
    printf '%s\n' 'WARNING: fixture example warning' >&2
    printf '%s\n' "$WEIR_FIXTURE_FAKE_DOCKER_ID"
    ;;
  container)
    printf '%s\n' 'WARNING: fixture inspect warning' >&2
    printf '[{"Id":"%s"}]\n' "$WEIR_FIXTURE_FAKE_DOCKER_ID"
    ;;
  fail)
    printf '%s\n' 'partial standard output'
    printf '%s\n' 'daemon failure diagnostic' >&2
    exit 17
    ;;
  *) exit 2 ;;
esac
`
	if err := os.WriteFile(filepath.Join(cliDirectory, "docker"), []byte(script), 0700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", cliDirectory+string(os.PathListSeparator)+os.Getenv("PATH"))
	c := &Cluster{Directory: directory, dockerHost: "unix:///unused-fixture.sock"}
	output, err := c.docker(context.Background(), "create")
	if err != nil || !validContainerID(strings.TrimSpace(string(output))) || strings.TrimSpace(string(output)) != id {
		t.Fatal("stderr warning corrupted Docker create ID", string(output), err)
	}
	state, err := c.inspectContainer(context.Background(), id)
	if err != nil || state.ID != id {
		t.Fatal("stderr warning corrupted Docker inspect JSON", state.ID, err)
	}
	output, err = c.docker(context.Background(), "fail")
	if err == nil || !strings.Contains(err.Error(), "daemon failure diagnostic") || !strings.Contains(err.Error(), "partial standard output") || !strings.Contains(err.Error(), "exit status 17") || strings.TrimSpace(string(output)) != "partial standard output" {
		t.Fatal("Docker command failure lost stream diagnostics", string(output), err)
	}
	log, err := os.ReadFile(filepath.Join(directory, "docker.log"))
	if err != nil || !bytes.Contains(log, []byte("WARNING: fixture example warning")) || !bytes.Contains(log, []byte("WARNING: fixture inspect warning")) || !bytes.Contains(log, []byte("daemon failure diagnostic")) || !bytes.Contains(log, []byte(id)) {
		t.Fatal("Docker log lost stdout or stderr", string(log), err)
	}
}

func TestSearchStartupWaitsForSlowTemplatePublication(t *testing.T) {
	templateEntered := make(chan struct{}, 2)
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		switch request.URL.Path {
		case "/":
			_, _ = fmt.Fprintf(writer, `{"version":{"number":%q}}`, SearchVersion)
		case "/_cluster/health":
			_, _ = writer.Write([]byte(`{"timed_out":false,"status":"green","number_of_nodes":1}`))
		case "/_index_template/weir-tests":
			templateEntered <- struct{}{}
			select {
			case <-time.After(3100 * time.Millisecond):
				_, _ = writer.Write([]byte(`{"acknowledged":true}`))
			case <-request.Context().Done():
			}
		default:
			writer.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()
	cluster := &Cluster{SearchURL: server.URL}
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()
	if err := cluster.readySearch(ctx); err != nil {
		t.Fatal(err)
	}
	<-templateEntered
	canceled, stop := context.WithCancel(t.Context())
	defer stop()
	finished := make(chan error, 1)
	go func() { finished <- cluster.readySearch(canceled) }()
	select {
	case <-templateEntered:
		stop()
	case <-time.After(2 * time.Second):
		t.Fatal("startup did not reach template publication")
	}
	select {
	case err := <-finished:
		if !errors.Is(err, context.Canceled) {
			t.Fatal("template publication lost caller cancellation", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("template publication ignored caller cancellation")
	}
}

func TestConfigurationExposesBatchCollectionWaitWithoutChangingDefaults(t *testing.T) {
	for _, wait := range []time.Duration{0, 20 * time.Millisecond} {
		opts := Options{Backends: []string{"search"}, BatchSize: 64, BatchBytes: 65536, MaxInFlight: 4, BatchMaxWait: wait}
		cluster := &Cluster{Directory: t.TempDir(), options: opts, SearchURL: "http://127.0.0.1:50002", owner: "test"}
		node := Node{Application: "127.0.0.1:50100", Diagnostics: "127.0.0.1:50102"}
		cluster.Nodes = []Node{node}
		if err := cluster.writeConfiguration(0); err != nil {
			t.Fatal(err)
		}
		raw, err := os.ReadFile(cluster.ConfigPath(0))
		if err != nil {
			t.Fatal(err)
		}
		var configuration struct {
			Stores []struct {
				Batching map[string]any `json:"batching"`
			} `json:"stores"`
		}
		if err := json.Unmarshal(raw, &configuration); err != nil {
			t.Fatal(err)
		}
		batching := configuration.Stores[0].Batching
		if batching["max_operations"] != float64(64) || batching["max_bytes"] != "65536B" || batching["max_in_flight"] != float64(4) {
			t.Fatal("explicit batch controls were dropped", string(raw))
		}
		configured, exists := batching["max_wait"]
		if !exists || configured != wait.String() {
			t.Fatal("collection wait hidden, dropped, or enabled by default", string(raw))
		}
	}
}

func TestBatchLimitWritesEveryBatchingControl(t *testing.T) {
	for _, operations := range []int{0, 32} {
		opts := Options{Backends: []string{"search"}, BatchSize: operations}
		cluster := &Cluster{Directory: t.TempDir(), options: opts, SearchURL: "http://127.0.0.1:50002", owner: "test"}
		node := Node{Application: "127.0.0.1:50100", Diagnostics: "127.0.0.1:50102"}
		cluster.Nodes = []Node{node}
		if err := cluster.writeConfiguration(0); err != nil {
			t.Fatal(err)
		}
		raw, err := os.ReadFile(cluster.ConfigPath(0))
		if err != nil {
			t.Fatal(err)
		}
		var configuration struct {
			Stores []struct {
				Batching map[string]any `json:"batching"`
			} `json:"stores"`
		}
		if err := json.Unmarshal(raw, &configuration); err != nil {
			t.Fatal(err)
		}
		batch := configuration.Stores[0].Batching
		if operations == 0 {
			if batch != nil {
				t.Fatal("default path emitted a hidden batching override", string(raw))
			}
			continue
		}
		if len(batch) != 4 || batch["max_operations"] != float64(32) || batch["max_bytes"] != "8388608B" || batch["max_wait"] != "0s" || batch["max_in_flight"] != float64(0) {
			t.Fatal("explicit controls were not all written", string(raw))
		}
	}
}
