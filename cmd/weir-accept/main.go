// weir-accept consumes caller-owned endpoints; it never starts fixtures or contacts Kubernetes.
package main

import (
	"context"
	"encoding/json"
	"flag"
	"io"
	"os"
	"os/signal"
	"path/filepath"
	"runtime"
	"runtime/debug"
	"strings"
	"syscall"

	"github.com/batchstream/weir-tests/internal/acceptance"
)

// Optional build assertion; runtime VCS build information is reported separately.
var sourceRevision = "unknown"

func main() {
	runtime.GOMAXPROCS(1)
	debug.SetMemoryLimit(64 << 20)
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	opts := cliOptions{args: os.Args[1:], out: os.Stdout, run: acceptance.Run}
	os.Exit(execute(ctx, opts))
}

type cliOptions struct {
	args []string
	out  io.Writer
	run  func(context.Context, acceptance.RunOptions) acceptance.Report
}

func execute(ctx context.Context, options cliOptions) int {
	flags := flag.NewFlagSet("weir-accept", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	input := flags.String("input", "", "explicit caller-owned JSON configuration (required)")
	output := flags.String("output", "", "optional new owner-prefixed JSON report path; never overwrite")
	version := flags.Bool("version", false, "report actual runner build identity without opening services")
	report := acceptance.Report{Schema: 1, Mode: "input", ErrorCode: acceptance.CodeInput, Build: acceptance.BuildInfo(sourceRevision), Checks: []acceptance.Check{}}
	if flags.Parse(options.args) != nil || flags.NArg() != 0 {
		return emit(options.out, report)
	}
	if *version && *input == "" && *output == "" {
		report.Passed = true
		report.Mode = "build"
		report.ErrorCode = acceptance.CodeNone
		return emit(options.out, report)
	}
	if *version || *input == "" || !inputPathAllowed(*input) {
		return emit(options.out, report)
	}
	info, err := os.Lstat(*input)
	if err != nil || !info.Mode().IsRegular() || info.Size() > acceptance.MaxInputBytes {
		return emit(options.out, report)
	}
	file, err := os.Open(*input)
	if err != nil {
		return emit(options.out, report)
	}
	config, decodeErr := acceptance.Decode(file)
	closeErr := file.Close()
	if decodeErr != nil || closeErr != nil {
		return emit(options.out, report)
	}
	// Validate before reserving an output path or creating network clients.
	if acceptance.Validate(config, runtime.GOOS+"/"+runtime.GOARCH) != nil {
		return emit(options.out, report)
	}
	var destination *os.File
	if *output != "" {
		destination, err = reserveOutput(config, *output)
		if err != nil {
			return emit(options.out, report)
		}
	}
	opts := acceptance.RunOptions{Config: config, InjectedSource: sourceRevision}
	report = options.run(ctx, opts)
	if destination != nil {
		if emit(destination, report) != 0 && report.Passed {
			report.Passed = false
			report.ErrorCode = acceptance.CodeFailure
		}
		if destination.Close() != nil {
			report.Passed = false
			report.ErrorCode = acceptance.CodeFailure
		}
	}
	return emit(options.out, report)
}

func inputPathAllowed(path string) bool {
	if filepath.Ext(path) != ".json" {
		return false
	}
	for _, part := range strings.Split(filepath.ToSlash(filepath.Clean(path)), "/") {
		part = strings.ToLower(part)
		if part == ".ssh" || part == ".kube" || part == "secrets" || strings.HasPrefix(part, ".env") || strings.HasPrefix(part, "credentials") {
			return false
		}
	}
	return true
}

func reserveOutput(config acceptance.Config, path string) (*os.File, error) {
	name := filepath.Base(path)
	if filepath.Ext(name) != ".json" || !strings.HasPrefix(name, config.OwnedDatasetPrefix) || len(name) > 96 {
		return nil, os.ErrInvalid
	}
	for _, c := range strings.TrimSuffix(name, ".json") {
		if c != '_' && (c < 'a' || c > 'z') && (c < '0' || c > '9') {
			return nil, os.ErrInvalid
		}
	}
	return os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
}

func emit(out io.Writer, report acceptance.Report) int {
	raw, err := json.Marshal(report)
	if err != nil || len(raw) > 64<<10 {
		return 1
	}
	raw = append(raw, '\n')
	n, err := out.Write(raw)
	if err != nil || n != len(raw) {
		return 1
	}
	if !report.Passed {
		return 1
	}
	return 0
}
