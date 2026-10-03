package fixture

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

func (c *Cluster) startNativeMongo(ctx context.Context) error {
	versionCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	versionCmd := exec.CommandContext(versionCtx, c.options.MongoBinary, "--version")
	output, err := versionCmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("inspect native MongoDB version: %w: %s", err, output)
	}
	if !pinnedMongoVersion(output) {
		return fmt.Errorf("native MongoDB binary must be %s: %s", MongoVersion, output)
	}
	if err := os.WriteFile(filepath.Join(c.Directory, "mongo-native-version.txt"), output, 0600); err != nil {
		return err
	}
	dbpath := filepath.Join(c.Directory, "mongo-data")
	if err := os.Mkdir(dbpath, 0700); err != nil {
		return err
	}
	listener, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		return err
	}
	defer listener.Close()
	address := listener.Addr().String()
	_, port, err := net.SplitHostPort(address)
	if err != nil {
		return err
	}
	logPath := filepath.Join(c.Directory, "mongo-native.log")
	log, err := os.OpenFile(logPath, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		return err
	}
	cmd := exec.Command(c.options.MongoBinary, "--dbpath", dbpath, "--bind_ip", "127.0.0.1", "--port", port, "--replSet", "weir_tests", "--wiredTigerCacheSizeGB", "0.25")
	cmd.Stdout, cmd.Stderr = log, log
	_ = listener.Close()
	if err := cmd.Start(); err != nil {
		_ = log.Close()
		return err
	}
	p := &process{cmd: cmd, done: make(chan struct{}), logPath: logPath}
	c.nativeMongo = p
	go func() {
		p.err = cmd.Wait()
		_ = log.Close()
		close(p.done)
	}()
	c.MongoURI = "mongodb://" + address + "/?directConnection=true"
	startup := mongoStartup{Process: p}
	return c.readyMongo(ctx, startup)
}

func pinnedMongoVersion(output []byte) bool {
	first, _, _ := strings.Cut(string(output), "\n")
	return strings.TrimSpace(first) == "db version v"+MongoVersion
}

// Require this owned process's listener announcement before contacting its
// dynamic port; a failed bind must never initialize a different local MongoDB.
func mongoAnnounced(path, uri string) (bool, error) {
	address, err := url.Parse(uri)
	if err != nil {
		return false, err
	}
	port, err := strconv.Atoi(address.Port())
	if err != nil {
		return false, err
	}
	log, err := os.Open(path)
	if err != nil {
		return false, err
	}
	defer log.Close()
	data, err := io.ReadAll(io.LimitReader(log, 512<<10))
	if err != nil {
		return false, err
	}
	scanner := bufio.NewScanner(bytes.NewReader(data))
	scanner.Buffer(make([]byte, 4096), 64<<10)
	for scanner.Scan() {
		var event struct {
			Message    string `json:"msg"`
			Attributes struct {
				Port int `json:"port"`
			} `json:"attr"`
		}
		if err := json.Unmarshal(scanner.Bytes(), &event); err != nil {
			continue
		}
		if event.Message == "Waiting for connections" && event.Attributes.Port == port {
			return true, nil
		}
	}
	if err := scanner.Err(); err != nil {
		return false, errors.Join(errors.New("native MongoDB startup log exceeds parser bounds"), err)
	}
	return false, nil
}
