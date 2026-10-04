// Package fixture owns isolated, real loopback databases and Weir processes.
// Importing the package and running its ordinary tests never starts a fixture.
package fixture

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"sync"
	"time"

	pb "github.com/batchstream/weir-protocol/api/weir/v1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

const ownerLabel = "io.batchstream.weir-tests.owner"

type Options struct {
	WeirBinary string
	// MongoBinary explicitly selects a native pinned mongod; empty uses Docker.
	MongoBinary      string
	Backends         []string
	OwnerCount       int
	DiscoveryOnly    bool
	StoreConcurrency int
	BatchSize        int
	MaxReadSizeBytes int
	BackendTimeout   time.Duration
	IngressSessions  int
	DatabaseCPUs     float64
	ProcessMemoryMiB int
	WorkingMemoryMiB map[string]int
}

type Node struct {
	ProcessID   int
	Application string
	Peer        string
	Diagnostics string
	Owner       bool
}

type Cluster struct {
	MongoURI  string
	SearchURL string
	Nodes     []Node
	Directory string

	mu          sync.Mutex
	options     Options
	owner       string
	dockerHost  string
	containers  []container
	processes   []*process
	nativeMongo *process
	closed      bool
}

type process struct {
	cmd     *exec.Cmd
	done    chan struct{}
	err     error
	logPath string
}

// Start creates new resources; it never adopts an existing container or process.
// All owners share the same logical Stores and backend, but each advertises
// only its own endpoint.
func Start(ctx context.Context, options Options) (*Cluster, error) {
	ctx, cancel := context.WithTimeout(ctx, 8*time.Minute)
	defer cancel()
	options, err := validateOptions(options)
	if err != nil {
		return nil, err
	}
	dockerHost := ""
	if needsDocker(options) {
		dockerHost, err = localDocker(ctx)
		if err != nil {
			return nil, err
		}
	}
	id := make([]byte, 12)
	if _, err := rand.Read(id); err != nil {
		return nil, err
	}
	directory, err := os.MkdirTemp("", "weir-tests-")
	if err != nil {
		return nil, err
	}
	c := &Cluster{Directory: directory, options: options, owner: hex.EncodeToString(id), dockerHost: dockerHost, containers: make([]container, 0, 2)}
	if err := c.start(ctx); err != nil {
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 75*time.Second)
		defer cleanupCancel()
		cleanupErr := c.Close(cleanupCtx)
		return nil, errors.Join(fmt.Errorf("fixture startup failed; logs: %s: %w", directory, err), cleanupErr)
	}
	return c, nil
}

func validateOptions(options Options) (Options, error) {
	if options.BackendTimeout == 0 {
		options.BackendTimeout = 2 * time.Second
	}
	if options.BackendTimeout < 0 {
		return options, errors.New("backend timeout must be positive")
	}
	if options.IngressSessions == 0 {
		options.IngressSessions = 64
	}
	if options.DatabaseCPUs == 0 {
		options.DatabaseCPUs = 2
	}
	if options.IngressSessions < 1 || options.IngressSessions > 512 || options.DatabaseCPUs <= 0 || options.DatabaseCPUs > float64(runtime.NumCPU()) {
		return options, errors.New("invalid ingress sessions or database CPU budget")
	}
	if runtime.GOOS != "linux" && runtime.GOOS != "darwin" {
		return options, errors.New("fixture requires Linux or macOS")
	}
	if options.WeirBinary == "" {
		return options, errors.New("WeirBinary is required; build the pinned Weir source first")
	}
	binary, err := filepath.Abs(options.WeirBinary)
	if err != nil {
		return options, err
	}
	info, err := os.Stat(binary)
	if err != nil || !info.Mode().IsRegular() || info.Mode().Perm()&0111 == 0 {
		return options, fmt.Errorf("WeirBinary must be an executable regular file: %s", binary)
	}
	options.WeirBinary = binary
	if options.MongoBinary != "" {
		mongoBinary, err := filepath.Abs(options.MongoBinary)
		if err != nil {
			return options, err
		}
		info, err := os.Stat(mongoBinary)
		if err != nil || !info.Mode().IsRegular() || info.Mode().Perm()&0111 == 0 {
			return options, fmt.Errorf("MongoBinary must be an executable regular file: %s", mongoBinary)
		}
		options.MongoBinary = mongoBinary
	}
	if len(options.Backends) == 0 {
		options.Backends = []string{"mongo", "search"}
	} else {
		options.Backends = slices.Clone(options.Backends)
	}
	seen := make(map[string]bool)
	for _, backend := range options.Backends {
		if backend != "mongo" && backend != "search" || seen[backend] {
			return options, errors.New("Backends must contain unique mongo and/or search entries")
		}
		seen[backend] = true
	}
	if options.OwnerCount == 0 {
		options.OwnerCount = 1
	}
	if options.OwnerCount < 1 || options.OwnerCount > 8 {
		return options, errors.New("OwnerCount must be between 1 and 8")
	}
	if options.StoreConcurrency == 0 {
		options.StoreConcurrency = 2
	}
	if options.MaxReadSizeBytes == 0 {
		options.MaxReadSizeBytes = 2 << 20
	}
	if options.MaxReadSizeBytes < 1024 || options.MaxReadSizeBytes > 2<<20 {
		return options, errors.New("invalid maximum read document size")
	}
	if options.BatchSize == 0 {
		options.BatchSize = 32
	}
	if options.StoreConcurrency < 1 || options.BatchSize < 1 {
		return options, errors.New("StoreConcurrency or BatchSize exceeds Weir bounds")
	}
	if options.ProcessMemoryMiB == 0 {
		options.ProcessMemoryMiB = 8192
	}
	if options.ProcessMemoryMiB < 64 || options.ProcessMemoryMiB > 65536 {
		return options, errors.New("invalid process memory envelope")
	}
	workspace := make(map[string]int, len(options.WorkingMemoryMiB))
	for backend, memory := range options.WorkingMemoryMiB {
		if !slices.Contains(options.Backends, backend) || memory < 24 || memory > options.ProcessMemoryMiB {
			return options, errors.New("invalid backend working memory envelope")
		}
		workspace[backend] = memory
	}
	options.WorkingMemoryMiB = workspace
	return options, nil
}

func (c *Cluster) start(ctx context.Context) error {
	if c.dockerHost != "" {
		configDirectory := filepath.Join(c.Directory, "docker-config")
		if err := os.Mkdir(configDirectory, 0700); err != nil {
			return err
		}
		if err := os.WriteFile(filepath.Join(configDirectory, "config.json"), []byte("{\"auths\":{}}\n"), 0600); err != nil {
			return err
		}
		engineFormat := `{"os":"{{.OSType}}","architecture":"{{.Architecture}}","version":"{{.ServerVersion}}","cpus":{{.NCPU}},"memory_bytes":{{.MemTotal}},"cgroup_version":"{{.CgroupVersion}}"}`
		engine, err := c.docker(ctx, "info", "--format", engineFormat)
		if err != nil {
			return err
		}
		if !json.Valid(engine) {
			return errors.New("Docker engine metadata is not valid JSON")
		}
		if err := os.WriteFile(filepath.Join(c.Directory, "docker-engine.json"), engine, 0600); err != nil {
			return err
		}
	}
	for _, backend := range c.options.Backends {
		if err := c.startBackend(ctx, backend); err != nil {
			return err
		}
	}
	count := c.options.OwnerCount
	if c.options.DiscoveryOnly {
		count++
	}
	var reservations []net.Listener
	defer func() {
		for _, listener := range reservations {
			_ = listener.Close()
		}
	}()
	for i := range count {
		var addresses []string
		for range 3 {
			listener, err := net.Listen("tcp4", "127.0.0.1:0")
			if err != nil {
				return err
			}
			reservations = append(reservations, listener)
			addresses = append(addresses, listener.Addr().String())
		}
		node := Node{Application: addresses[0], Peer: addresses[1], Diagnostics: addresses[2], Owner: i < c.options.OwnerCount}
		c.Nodes = append(c.Nodes, node)
		c.processes = append(c.processes, nil)
	}
	for i := range c.Nodes {
		if err := c.writeConfiguration(i); err != nil {
			return err
		}
		for _, listener := range reservations[i*3 : i*3+3] {
			_ = listener.Close()
		}
		if err := c.startNode(ctx, i); err != nil {
			return err
		}
	}
	if err := c.waitDiscovery(ctx); err != nil {
		return err
	}
	return c.writeManifest()
}

func (c *Cluster) writeConfiguration(index int) error {
	node := c.Nodes[index]
	seeds := make([]string, 0, len(c.Nodes)-1)
	for i, peer := range c.Nodes {
		if i != index {
			seeds = append(seeds, peer.Peer)
		}
	}
	listeners := map[string]any{"application": node.Application, "peer": node.Peer}
	diagnostics := map[string]any{"address": node.Diagnostics}
	discovery := map[string]any{"group": "owners-" + c.owner, "peer_address": node.Peer, "seeds": seeds, "advertise": []string{node.Application}}
	if !node.Owner {
		discovery["group"] = "directory-" + c.owner
	}
	transport := map[string]any{"max_connections": 64, "max_sessions": c.options.IngressSessions}
	basic := map[string]any{"listeners": listeners, "diagnostics": diagnostics, "discovery": discovery, "transport": transport, "memory": fmt.Sprintf("%dMiB", c.options.ProcessMemoryMiB)}
	if err := writeJSON(c.nodeFile(index, "config.json"), basic); err != nil {
		return err
	}
	stores := make([]map[string]any, 0, len(c.options.Backends))
	if node.Owner {
		for _, backend := range c.options.Backends {
			store := map[string]any{"name": backend, "max_concurrency": c.options.StoreConcurrency, "max_batch_operations": c.options.BatchSize, "max_read_size": fmt.Sprintf("%dB", c.options.MaxReadSizeBytes), "backend_timeout": c.options.BackendTimeout.String()}
			if memory, configured := c.options.WorkingMemoryMiB[backend]; configured {
				store["working_memory"] = fmt.Sprintf("%dMiB", memory)
			}
			if backend == "mongo" {
				store["mongodb"] = map[string]any{"uri": c.MongoURI}
			} else {
				store["search"] = map[string]any{"url": c.SearchURL}
			}
			stores = append(stores, store)
		}
	}
	routes := map[string]any{"stores": stores}
	return writeJSON(c.nodeFile(index, "routes.json"), routes)
}

func (c *Cluster) nodeFile(index int, suffix string) string {
	return filepath.Join(c.Directory, fmt.Sprintf("node-%d-%s", index, suffix))
}

func writeJSON(path string, value any) error {
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(data, '\n'), 0600)
}

func (c *Cluster) startNode(ctx context.Context, index int) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	logPath := c.nodeFile(index, "process.log")
	log, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0600)
	if err != nil {
		return err
	}
	info, err := log.Stat()
	if err != nil {
		_ = log.Close()
		return err
	}
	logOffset := info.Size()
	cmd := exec.Command(c.options.WeirBinary, "serve", "--config", c.nodeFile(index, "config.json"), "--routes", c.nodeFile(index, "routes.json"))
	cmd.Stdout, cmd.Stderr = log, log
	if err := cmd.Start(); err != nil {
		_ = log.Close()
		return err
	}
	p := &process{cmd: cmd, done: make(chan struct{}), logPath: logPath}
	c.processes[index] = p
	c.Nodes[index].ProcessID = cmd.Process.Pid
	go func() {
		p.err = cmd.Wait()
		_ = log.Close()
		close(p.done)
	}()
	readyCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	client := loopbackHTTPClient()
	defer client.CloseIdleConnections()
	for {
		select {
		case <-p.done:
			return fmt.Errorf("Weir node %d exited before readiness (%s): %w", index, logPath, p.err)
		default:
		}
		announced, err := processAnnounced(logPath, logOffset, c.Nodes[index])
		if err != nil {
			return err
		}
		if !announced {
			if err := pause(readyCtx); err != nil {
				return fmt.Errorf("Weir node %d did not announce its listeners (%s): %w", index, logPath, err)
			}
			continue
		}
		request, err := http.NewRequestWithContext(readyCtx, http.MethodGet, "http://"+c.Nodes[index].Diagnostics+"/readyz", nil)
		if err != nil {
			return err
		}
		response, err := client.Do(request)
		if err == nil {
			_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, 4096))
			_ = response.Body.Close()
			if response.StatusCode == http.StatusOK {
				return nil
			}
		}
		if err := pause(readyCtx); err != nil {
			return fmt.Errorf("Weir node %d not ready (%s): %w", index, logPath, err)
		}
	}
}

func processAnnounced(path string, offset int64, node Node) (bool, error) {
	log, err := os.Open(path)
	if err != nil {
		return false, err
	}
	defer log.Close()
	if _, err := log.Seek(offset, io.SeekStart); err != nil {
		return false, err
	}
	data, err := io.ReadAll(io.LimitReader(log, 64<<10))
	if err != nil {
		return false, err
	}
	announcement := fmt.Sprintf("Weir listening on [%s %s]; local execution and peer directory discovery\n", node.Application, node.Peer)
	diagnostics := "Diagnostics listening on " + node.Diagnostics + "\n"
	return strings.Contains(string(data), announcement) && strings.Contains(string(data), diagnostics), nil
}

func (c *Cluster) waitDiscovery(ctx context.Context) error {
	readyCtx, cancel := context.WithTimeout(ctx, 25*time.Second)
	defer cancel()
	var expected []string
	for _, node := range c.Nodes {
		if node.Owner {
			expected = append(expected, node.Application)
		}
	}
	slices.Sort(expected)
	for _, node := range c.Nodes {
		conn, err := grpc.NewClient("passthrough:///"+node.Application, grpc.WithTransportCredentials(insecure.NewCredentials()), grpc.WithNoProxy(), grpc.WithDisableRetry(), grpc.WithDisableServiceConfig())
		if err != nil {
			return err
		}
		err = awaitStores(readyCtx, conn, c.options.Backends, expected)
		_ = conn.Close()
		if err != nil {
			return fmt.Errorf("directory did not converge on %s: %w", node.Application, err)
		}
	}
	return nil
}

func awaitStores(ctx context.Context, conn *grpc.ClientConn, stores, expected []string) error {
	client := pb.NewStoreServiceClient(conn)
	for _, store := range stores {
		var last error
		for {
			request := &pb.ResolveStoreRequest{StoreName: store}
			rpcCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
			response, err := client.ResolveStore(rpcCtx, request)
			cancel()
			if err == nil {
				endpoints := slices.Clone(response.Endpoints)
				slices.Sort(endpoints)
				if response.StoreName == store && slices.Equal(endpoints, expected) {
					break
				}
				last = fmt.Errorf("store %s endpoints %v, expected %v", store, endpoints, expected)
			} else {
				last = err
			}
			if err := pause(ctx); err != nil {
				return errors.Join(err, last)
			}
		}
	}
	return nil
}

func (c *Cluster) Seed() string {
	if len(c.Nodes) == 0 {
		return ""
	}
	if c.options.DiscoveryOnly {
		return c.Nodes[len(c.Nodes)-1].Application
	}
	return c.Nodes[0].Application
}

func (c *Cluster) StopNode(ctx context.Context, index int) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if index < 0 || index >= len(c.processes) {
		return errors.New("node index out of range")
	}
	return stopProcess(ctx, c.processes[index])
}

func (c *Cluster) RestartNode(ctx context.Context, index int) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.closed {
		return errors.New("fixture is closed")
	}
	if index < 0 || index >= len(c.processes) {
		return errors.New("node index out of range")
	}
	p := c.processes[index]
	if p != nil {
		select {
		case <-p.done:
		default:
			return errors.New("node is still running; stop it before restart")
		}
	}
	return c.startNode(ctx, index)
}

// Close is repeatable. It retains logs and configuration; it only removes
// containers with this exact fixture owner, recorded ID, name and pinned image.
func (c *Cluster) Close(ctx context.Context) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.closed = true
	var failures []error
	for i := len(c.processes) - 1; i >= 0; i-- {
		if err := stopProcess(ctx, c.processes[i]); err != nil {
			failures = append(failures, err)
		}
	}
	if err := stopProcess(ctx, c.nativeMongo); err != nil {
		failures = append(failures, err)
	}
	for i := len(c.containers) - 1; i >= 0; i-- {
		if err := c.removeContainer(ctx, &c.containers[i]); err != nil {
			failures = append(failures, err)
		}
	}
	if c.Directory != "" {
		states := make([]map[string]any, 0, len(c.containers))
		for _, owned := range c.containers {
			state := map[string]any{"id": owned.ID, "name": owned.Name, "image": owned.Image, "removed": owned.removed}
			states = append(states, state)
		}
		processes := make([]map[string]any, 0, len(c.processes)+1)
		for i, process := range c.processes {
			if process == nil {
				continue
			}
			stopped := false
			select {
			case <-process.done:
				stopped = true
			default:
			}
			state := map[string]any{"index": i, "pid": process.cmd.Process.Pid, "stopped": stopped}
			processes = append(processes, state)
		}
		if c.nativeMongo != nil {
			stopped := false
			select {
			case <-c.nativeMongo.done:
				stopped = true
			default:
			}
			state := map[string]any{"kind": "native_mongo", "pid": c.nativeMongo.cmd.Process.Pid, "stopped": stopped}
			processes = append(processes, state)
		}
		message := ""
		if failure := errors.Join(failures...); failure != nil {
			message = failure.Error()
		}
		cleanup := map[string]any{"owner": c.owner, "finished_at": time.Now().UTC().Format(time.RFC3339Nano), "containers": states, "processes": processes, "errors": message}
		if err := writeJSON(filepath.Join(c.Directory, "cleanup.json"), cleanup); err != nil {
			failures = append(failures, err)
		}
	}
	return errors.Join(failures...)
}

func pause(ctx context.Context) error {
	timer := time.NewTimer(100 * time.Millisecond)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

func loopbackHTTPClient() *http.Client {
	transport := &http.Transport{Proxy: nil, MaxIdleConnsPerHost: 4}
	client := &http.Client{Transport: transport, Timeout: 3 * time.Second}
	return client
}

func (c *Cluster) writeManifest() error {
	binary, err := os.Open(c.options.WeirBinary)
	if err != nil {
		return err
	}
	defer binary.Close()
	hash := sha256.New()
	if _, err := io.Copy(hash, binary); err != nil {
		return err
	}
	manifest := map[string]any{"owner": c.owner, "created_at": time.Now().UTC().Format(time.RFC3339Nano), "binary": c.options.WeirBinary, "binary_sha256": hex.EncodeToString(hash.Sum(nil)), "mongo_uri": c.MongoURI, "search_url": c.SearchURL, "nodes": c.Nodes, "containers": c.containers, "options": c.options, "host_os": runtime.GOOS, "host_arch": runtime.GOARCH}
	if c.nativeMongo != nil {
		native, err := os.Open(c.options.MongoBinary)
		if err != nil {
			return err
		}
		defer native.Close()
		hash := sha256.New()
		if _, err := io.Copy(hash, native); err != nil {
			return err
		}
		manifest["native_mongo"] = map[string]any{"binary": c.options.MongoBinary, "binary_sha256": hex.EncodeToString(hash.Sum(nil)), "version": MongoVersion, "host_os": runtime.GOOS, "host_arch": runtime.GOARCH, "pid": c.nativeMongo.cmd.Process.Pid, "data_directory": filepath.Join(c.Directory, "mongo-data")}
	}
	return writeJSON(filepath.Join(c.Directory, "manifest.json"), manifest)
}

func needsDocker(options Options) bool {
	for _, backend := range options.Backends {
		if backend == "search" || backend == "mongo" && options.MongoBinary == "" {
			return true
		}
	}
	return false
}

func localDocker(ctx context.Context) (string, error) {
	hostOverride := os.Getenv("DOCKER_HOST")
	if hostOverride != "" && !localDockerHost(hostOverride) {
		return "", errors.New("remote Docker daemon is forbidden; use a local Unix socket")
	}
	cmd := exec.CommandContext(ctx, "docker", "context", "inspect", "--format", "{{.Endpoints.docker.Host}}")
	output, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("inspect Docker context: %w: %s", err, strings.TrimSpace(string(output)))
	}
	host := strings.TrimSpace(string(output))
	if !localDockerHost(host) {
		return "", errors.New("remote Docker context is forbidden; use Docker Desktop or a local Linux daemon")
	}
	if hostOverride != "" && os.Getenv("DOCKER_CONTEXT") == "" {
		host = hostOverride
	}
	return host, nil
}

func localDockerHost(host string) bool {
	return strings.HasPrefix(host, "unix:///") && !strings.ContainsAny(host, "\r\n\x00")
}
