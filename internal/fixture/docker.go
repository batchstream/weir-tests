package fixture

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"
)

const (
	MongoVersion  = "8.0.32"
	MongoImage    = "mongo@sha256:d0d926f94df099bff534b7ee5b5986458131a22489dfff8664509af0c1e2ca9c"
	SearchVersion = "8.19.22"
	SearchImage   = "docker.elastic.co/elasticsearch/elasticsearch@sha256:e98f9c3b09beb2fbb9eaf667d602df3f0e00bd3644138b8458dc17ba1a675595"
)

type container struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	Image   string `json:"image"`
	Backend string `json:"backend"`
	removed bool
	stopped bool
	exitErr error
}

type inspection struct {
	ID     string `json:"Id"`
	Name   string
	Config struct {
		Image  string
		Labels map[string]string
	}
	State struct {
		Running  bool
		ExitCode int
		Error    string
	}
	NetworkSettings struct {
		Ports map[string][]portBinding
	}
}

type portBinding struct {
	HostIP   string `json:"HostIp"`
	HostPort string
}

func (c *Cluster) docker(ctx context.Context, args ...string) ([]byte, error) {
	commandTimeout := 30 * time.Second
	if len(args) > 0 && args[0] == "pull" {
		commandTimeout = 10 * time.Minute
	}
	commandCtx, cancel := context.WithTimeout(ctx, commandTimeout)
	defer cancel()
	log, err := os.OpenFile(filepath.Join(c.Directory, "docker.log"), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0600)
	if err != nil {
		return nil, err
	}
	defer log.Close()
	_, _ = fmt.Fprintf(log, "%s docker %s\n", time.Now().UTC().Format(time.RFC3339Nano), strings.Join(args, " "))
	var stdout, stderr boundedOutput
	commandArgs := append([]string{"--config", filepath.Join(c.Directory, "docker-config"), "--host", c.dockerHost}, args...)
	cmd := exec.CommandContext(commandCtx, "docker", commandArgs...)
	for _, entry := range os.Environ() {
		if !strings.HasPrefix(entry, "DOCKER_CONTEXT=") && !strings.HasPrefix(entry, "DOCKER_HOST=") && !strings.HasPrefix(entry, "DOCKER_CONFIG=") {
			cmd.Env = append(cmd.Env, entry)
		}
	}
	cmd.Stdout = io.MultiWriter(log, &stdout)
	cmd.Stderr = io.MultiWriter(log, &stderr)
	if err := cmd.Run(); err != nil {
		return stdout.Bytes(), fmt.Errorf("docker %s: %w; stdout: %s; stderr: %s", strings.Join(args, " "), err, strings.TrimSpace(stdout.String()), strings.TrimSpace(stderr.String()))
	}
	return stdout.Bytes(), nil
}

// Command output is fully retained in logs and bounded when parsed in memory.
type boundedOutput struct{ bytes.Buffer }

func (b *boundedOutput) Write(p []byte) (int, error) {
	count := len(p)
	remaining := 64<<10 - b.Len()
	if remaining > 0 {
		if len(p) > remaining {
			p = p[:remaining]
		}
		_, _ = b.Buffer.Write(p)
	}
	return count, nil
}

func (c *Cluster) startBackend(ctx context.Context, backend string) error {
	if backend == "mongo" && c.options.MongoBinary != "" {
		return c.startNativeMongo(ctx)
	}
	image, port := MongoImage, "27017"
	if backend == "search" {
		image, port = SearchImage, "9200"
	}
	if _, err := c.docker(ctx, "image", "inspect", image); err != nil {
		if _, err := c.docker(ctx, "pull", image); err != nil {
			return err
		}
	}
	name := "weir-tests-" + c.owner + "-" + backend
	args := []string{"create", "--pull=never", "--name", name, "--label", ownerLabel + "=" + c.owner, "--label", "io.batchstream.weir-tests.backend=" + backend, "--publish", "127.0.0.1::" + port, "--cpus", "2", "--memory", "1536m"}
	if backend == "mongo" {
		args = append(args, image, "mongod", "--replSet", "weir_tests", "--bind_ip_all", "--wiredTigerCacheSizeGB", "0.25")
	} else {
		args = append(args, "--env", "discovery.type=single-node", "--env", "cluster.name="+name, "--env", "xpack.security.enabled=false", "--env", "action.auto_create_index=false", "--env", "ES_JAVA_OPTS=-Xms512m -Xmx512m", image)
	}
	owned := container{Name: name, Image: image, Backend: backend}
	output, createErr := c.docker(ctx, args...)
	if createErr == nil {
		owned.ID = strings.TrimSpace(string(output))
		if !validContainerID(owned.ID) {
			createErr = errors.New("docker create returned an invalid container ID")
		}
	}
	if createErr != nil {
		// A cancelled CLI can leave a successful daemon-side creation. Recover
		// only this newly generated name and exact ownership label for cleanup.
		recoveryCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		state, err := c.inspectContainer(recoveryCtx, name)
		if err == nil {
			owned.ID = state.ID
			if err := verifyOwned(state, owned, c.owner); err == nil {
				c.containers = append(c.containers, owned)
			}
		}
		return createErr
	}
	c.containers = append(c.containers, owned)
	if _, err := c.docker(ctx, "start", owned.ID); err != nil {
		return err
	}
	state, err := c.inspectContainer(ctx, owned.ID)
	if err != nil {
		return err
	}
	if err := verifyOwned(state, owned, c.owner); err != nil {
		return err
	}
	address, err := publishedLoopback(state.NetworkSettings.Ports[port+"/tcp"])
	if err != nil {
		return err
	}
	if backend == "mongo" {
		c.MongoURI = "mongodb://" + address + "/?directConnection=true"
		startup := mongoStartup{Container: &owned}
		return c.readyMongo(ctx, startup)
	}
	c.SearchURL = "http://" + address
	return c.readySearch(ctx)
}

func validContainerID(id string) bool {
	return len(id) == 64 && strings.Trim(id, "0123456789abcdef") == ""
}

func verifyOwned(state inspection, owned container, owner string) error {
	if !validContainerID(owned.ID) || state.ID != owned.ID || state.Name != "/"+owned.Name || state.Config.Image != owned.Image || state.Config.Labels[ownerLabel] != owner || state.Config.Labels["io.batchstream.weir-tests.backend"] != owned.Backend || owner == "" {
		return fmt.Errorf("refusing to mutate container %s: fixture ownership, ID, name or image mismatch", owned.ID)
	}
	return nil
}

func publishedLoopback(bindings []portBinding) (string, error) {
	if len(bindings) != 1 || bindings[0].HostIP != "127.0.0.1" {
		return "", errors.New("database port must have exactly one IPv4 loopback binding")
	}
	port, err := strconv.Atoi(bindings[0].HostPort)
	if err != nil || port <= 0 || port > 65535 {
		return "", errors.New("Docker returned an invalid database host port")
	}
	return net.JoinHostPort(bindings[0].HostIP, bindings[0].HostPort), nil
}

func (c *Cluster) inspectContainer(ctx context.Context, id string) (inspection, error) {
	var result inspection
	output, err := c.docker(ctx, "container", "inspect", id)
	if err != nil {
		return result, err
	}
	var states []inspection
	if err := json.Unmarshal(output, &states); err != nil {
		return result, err
	}
	if len(states) != 1 {
		return result, errors.New("expected one container inspection")
	}
	return states[0], nil
}

func (c *Cluster) removeContainer(ctx context.Context, owned *container) error {
	if owned.removed {
		return owned.exitErr
	}
	state, err := c.inspectContainer(ctx, owned.ID)
	if err != nil {
		if strings.Contains(err.Error(), "No such container:") || strings.Contains(err.Error(), "No such object:") {
			owned.removed = true
			return owned.exitErr
		}
		return err
	}
	if err := verifyOwned(state, *owned, c.owner); err != nil {
		return err
	}
	if !state.State.Running && !owned.stopped && (state.State.ExitCode != 0 || state.State.Error != "") {
		owned.exitErr = fmt.Errorf("owned %s container %s exited before fixture cleanup (exit %d, error %q); logs: %s", owned.Backend, owned.ID, state.State.ExitCode, state.State.Error, filepath.Join(c.Directory, owned.Backend+"-container.log"))
	}
	logPath := filepath.Join(c.Directory, owned.Backend+"-container.log")
	output, logErr := c.docker(ctx, "logs", owned.ID)
	writeErr := os.WriteFile(logPath, output, 0600)
	var stopErr error
	if state.State.Running {
		_, stopErr = c.docker(ctx, "stop", "--time", "10", owned.ID)
		if stopErr == nil {
			owned.stopped = true
		}
	}
	_, removeErr := c.docker(ctx, "rm", "--volumes", owned.ID)
	if removeErr == nil {
		owned.removed = true
	}
	return errors.Join(owned.exitErr, logErr, writeErr, stopErr, removeErr)
}

func stopProcess(ctx context.Context, p *process) error {
	if p == nil {
		return nil
	}
	select {
	case <-p.done:
		return processExitError(p)
	default:
	}
	signalErr := p.cmd.Process.Signal(syscall.SIGTERM)
	if signalErr != nil && !errors.Is(signalErr, os.ErrProcessDone) {
		return signalErr
	}
	timer := time.NewTimer(20 * time.Second)
	defer timer.Stop()
	var waitErr error
	select {
	case <-p.done:
		return processExitError(p)
	case <-ctx.Done():
		waitErr = ctx.Err()
	case <-timer.C:
		waitErr = errors.New("fixture graceful shutdown exceeded 20s")
	}
	killErr := p.cmd.Process.Kill()
	if errors.Is(killErr, os.ErrProcessDone) {
		killErr = nil
	}
	select {
	case <-p.done:
	case <-time.After(5 * time.Second):
		waitErr = errors.Join(waitErr, errors.New("owned fixture process did not exit after Kill"))
	}
	return errors.Join(waitErr, killErr)
}

func processExitError(p *process) error {
	if p.err != nil {
		return fmt.Errorf("fixture process exited with failure (%s): %w", p.logPath, p.err)
	}
	return nil
}
