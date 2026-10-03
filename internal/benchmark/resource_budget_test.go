package benchmark

import (
	"context"
	"encoding/json"
	"math"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"testing"

	"github.com/batchstream/weir-tests/internal/fixture"
)

func installQuotaDocker(t *testing.T) string {
	t.Helper()
	directory := t.TempDir()
	command := filepath.Join(directory, "docker")
	contents := "#!/bin/sh\nprintf '%s\\n' \"$@\" > \"$WEIR_TEST_QUOTA_ARGUMENTS\"\nprintf '%s\\n' \"$WEIR_TEST_QUOTA_OUTPUT\"\nexit \"$WEIR_TEST_QUOTA_EXIT\"\n"
	if err := os.WriteFile(command, []byte(contents), 0700); err != nil {
		t.Fatal(err)
	}
	arguments := filepath.Join(directory, "arguments.txt")
	t.Setenv("PATH", directory+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("WEIR_TEST_QUOTA_ARGUMENTS", arguments)
	t.Setenv("WEIR_TEST_QUOTA_EXIT", "0")
	return arguments
}

func TestDatabaseQuotaInspectionPreservesRawEvidence(t *testing.T) {
	container := strings.Repeat("a", 64)
	for _, cpus := range []float64{0.5, 1, 2} {
		t.Run(strconv.FormatFloat(cpus, 'f', -1, 64), func(t *testing.T) {
			arguments := installQuotaDocker(t)
			nanoCPUs := int64(cpus * 1e9)
			t.Setenv("WEIR_TEST_QUOTA_OUTPUT", container+" "+strconv.FormatInt(nanoCPUs, 10))
			target := fixture.ResourceTarget{Container: container, AllocatedCPUs: cpus, DockerHost: "unix:///owned/docker.sock", DockerConfig: "/owned/empty-docker-config"}
			quota, err := verifyResourceBudget(t.Context(), target)
			if err != nil || quota == nil || quota.ContainerID != container || quota.NanoCPUs != nanoCPUs || quota.RequestedCPUs != cpus || quota.ObservedCPUs != cpus {
				t.Fatal("inspected quota was not retained", quota, err)
			}
			rawArguments, err := os.ReadFile(arguments)
			if err != nil {
				t.Fatal(err)
			}
			expectedArguments := []string{"--config", target.DockerConfig, "--host", target.DockerHost, "inspect", "--format", "{{.Id}} {{.HostConfig.NanoCpus}}", container}
			actualArguments := strings.Split(strings.TrimSuffix(string(rawArguments), "\n"), "\n")
			if !reflect.DeepEqual(actualArguments, expectedArguments) {
				t.Fatal("inspection used an unexpected endpoint or read extra container properties", actualArguments)
			}
			report := SaturationReport{CPUQuota: quota}
			raw, err := json.Marshal(report)
			if err != nil {
				t.Fatal(err)
			}
			var decoded struct {
				Quota map[string]any `json:"database_cpu_quota"`
			}
			if err := json.Unmarshal(raw, &decoded); err != nil {
				t.Fatal(err)
			}
			if len(decoded.Quota) != 4 || decoded.Quota["container_id"] != container || decoded.Quota["host_config_nano_cpus"] != float64(nanoCPUs) || decoded.Quota["requested_cpus"] != cpus || decoded.Quota["observed_cpus"] != cpus {
				t.Fatal("report lost raw quota evidence or retained unexpected properties", decoded.Quota)
			}
		})
	}
}

func TestDatabaseQuotaInspectionRejectsUnverifiedEvidence(t *testing.T) {
	container := strings.Repeat("a", 64)
	for _, test := range []struct {
		name   string
		output string
		exit   string
		cpus   float64
	}{
		{name: "different container", output: strings.Repeat("b", 64) + " 1000000000", cpus: 1},
		{name: "quota differs", output: container + " 500000000", cpus: 1},
		{name: "no quota", output: container + " 0", cpus: 1},
		{name: "negative quota", output: container + " -1000000000", cpus: 1},
		{name: "malformed quota", output: container + " not-a-number", cpus: 1},
		{name: "missing identity", output: "1000000000", cpus: 1},
		{name: "unexpected properties", output: container + " 1000000000 extra", cpus: 1},
		{name: "failed inspect", output: container + " 1000000000", exit: "1", cpus: 1},
		{name: "invalid requested quota", output: container + " 1000000000", cpus: math.NaN()},
	} {
		t.Run(test.name, func(t *testing.T) {
			installQuotaDocker(t)
			t.Setenv("WEIR_TEST_QUOTA_OUTPUT", test.output)
			if test.exit != "" {
				t.Setenv("WEIR_TEST_QUOTA_EXIT", test.exit)
			}
			target := fixture.ResourceTarget{Container: container, AllocatedCPUs: test.cpus}
			quota, err := verifyResourceBudget(t.Context(), target)
			if err == nil || quota != nil {
				t.Fatal("invalid inspection became verified quota evidence", quota, err)
			}
		})
	}
}

func TestNativeDatabaseDoesNotClaimDockerQuotaInspection(t *testing.T) {
	target := fixture.ResourceTarget{ProcessID: 123, AllocatedCPUs: 8}
	quota, err := verifyResourceBudget(context.Background(), target)
	if err != nil || quota != nil {
		t.Fatal("native host CPU denominator became a Docker quota", quota, err)
	}
}
