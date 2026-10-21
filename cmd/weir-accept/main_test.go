package main

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/batchstream/weir-tests/internal/acceptance"
)

func TestOfflineVersionAndInvalidInputNeverOpenServices(t *testing.T) {
	cases := [][]string{{"-version"}, {}, {"-input", "not-a-config.env"}, {"-input", "absent.json"}, {"-unknown", "private://credential"}, {"-version", "-input", "absent.json"}}
	for index, args := range cases {
		var out bytes.Buffer
		opts := cliOptions{args: args, out: &out, run: func(context.Context, acceptance.RunOptions) acceptance.Report {
			t.Fatal("unexpected service access")
			report := acceptance.Report{}
			return report
		}}
		code := execute(context.Background(), opts)
		var report acceptance.Report
		if err := json.Unmarshal(out.Bytes(), &report); err != nil {
			t.Fatal(err)
		}
		if index == 0 {
			if code != 0 || report.Mode != "build" || len(report.Checks) != 0 {
				t.Fatal("version is not distinct from functional execution")
			}
		} else if code == 0 || report.ErrorCode != acceptance.CodeInput {
			t.Fatal("invalid input succeeded")
		}
		if strings.Contains(out.String(), "private://") || strings.Contains(out.String(), "absent.json") {
			t.Fatal("report leaked input/error")
		}
	}
}

func TestSensitiveInputPathsAreRejectedBeforeRead(t *testing.T) {
	for _, path := range []string{".env.json", ".ssh/config.json", ".kube/config.json", "/run/secrets/config.json", "credentials.json"} {
		if inputPathAllowed(path) {
			t.Fatal("sensitive input path accepted")
		}
	}
	if !inputPathAllowed("/run/weir-accept/config.json") {
		t.Fatal("dedicated regular configuration path rejected")
	}
}

func TestDedicatedOutputIsExclusiveAndOwnerPrefixed(t *testing.T) {
	root := t.TempDir()
	config := acceptance.Config{OwnedDatasetPrefix: "weiraccept_run12345_"}
	path := filepath.Join(root, "weiraccept_run12345_report.json")
	file, err := reserveOutput(config, path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := file.WriteString("original"); err != nil {
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
	if file, err := reserveOutput(config, path); err == nil {
		file.Close()
		t.Fatal("existing report overwritten")
	}
	raw, err := os.ReadFile(path)
	if err != nil || string(raw) != "original" {
		t.Fatal("existing report changed")
	}
	info, err := os.Stat(path)
	if err != nil || info.Mode().Perm() != 0600 {
		t.Fatal("report permissions")
	}
	for _, name := range []string{"other_report.json", "weiraccept_run12345_bad\n.json", "weiraccept_run12345_report.txt"} {
		if file, err := reserveOutput(config, filepath.Join(root, name)); err == nil {
			file.Close()
			t.Fatal("invalid output accepted")
		}
	}
	link := filepath.Join(root, "weiraccept_run12345_link.json")
	if err := os.Symlink(path, link); err != nil {
		t.Fatal(err)
	}
	if file, err := reserveOutput(config, link); err == nil {
		file.Close()
		t.Fatal("output symlink accepted")
	}
}

type shortWriter struct{}

func (shortWriter) Write(raw []byte) (int, error) { return len(raw) - 1, nil }
func TestShortReportWriteFails(t *testing.T) {
	report := acceptance.Report{Passed: true}
	out := shortWriter{}
	if emit(out, report) == 0 {
		t.Fatal("short output counted as report delivery")
	}
}
