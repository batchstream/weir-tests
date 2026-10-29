package main

import (
	"math"
	"testing"
	"time"
)

func TestMongoCLISharedProfileValidation(t *testing.T) {
	tests := []struct {
		uri           string
		remote, valid bool
	}{
		{"mongodb://localhost:1,[::1]:2/?replicaSet=rs", false, true},
		{"mongodb://[::1]:1,[::2]:2/?replicaSet=rs", true, true},
		{"mongodb://localhost:1,remote.invalid:2/?replicaSet=rs", false, false},
		{"mongodb://localhost:1,remote.invalid:2/?replicaSet=rs", true, true},
		{"mongodb://localhost:1/?replicaSet=rs&directConnection=true", false, false},
		{"mongodb://localhost:0", false, false},
		{"mongodb://localhost:1,LOCALHOST:01/?replicaSet=rs", false, false},
		{"mongodb://localhost:1/?password=secret", false, false},
	}
	for _, test := range tests {
		input := settings{mode: "fixed", backend: "mongo", mongoURI: test.uri, allowRemote: test.remote, weirAddress: "127.0.0.1:7447", store: "mongo", timeout: 1}
		if err := validate(input); (err == nil) != test.valid {
			t.Fatalf("valid=%v: %v", test.valid, err)
		}
	}
}

func TestEndpointValidationRequiresExplicitOptInAndNoCredentials(t *testing.T) {
	valid := settings{mode: "fixed", backend: "mongo", mongoURI: "mongodb://127.0.0.1:27028/?directConnection=true", weirAddress: "127.0.0.1:7447", store: "mongo", timeout: 1}
	if err := validate(valid); err != nil {
		t.Fatal(err)
	}
	remote := valid
	remote.mongoURI = "mongodb://192.0.2.1:27017/"
	if err := validate(remote); err == nil {
		t.Fatal("remote endpoint lacked opt-in")
	}
	remote.allowRemote = true
	if err := validate(remote); err != nil {
		t.Fatal(err)
	}
	credentialed := valid
	credentialed.mongoURI = "mongodb://user:password@127.0.0.1:27028/"
	if err := validate(credentialed); err == nil {
		t.Fatal("accepted secret in URI")
	}
	credentialed.mongoURI = "mongodb://127.0.0.1:27028/?authMechanismProperties=password"
	if err := validate(credentialed); err == nil {
		t.Fatal("accepted secret query options")
	}
	valid.backend, valid.searchURL = "search", "http://127.0.0.1:19200"
	if err := validate(valid); err != nil {
		t.Fatal(err)
	}
	valid.searchURL += "/production"
	if err := validate(valid); err == nil {
		t.Fatal("accepted existing backend namespace URL")
	}
}

func TestSaturationModeRequiresDiagnosticsCPUBudgetAndEvenLadder(t *testing.T) {
	valid := settings{mode: "saturation", backend: "mongo", mongoURI: "mongodb://127.0.0.1:27028/?directConnection=true", weirAddress: "127.0.0.1:7447", weirDiagnostics: "127.0.0.1:7449", store: "mongo", databaseCPUs: 2, levels: "64,256", clientProcesses: 16, records: 256, writes: 100, rounds: 1, warmupDuration: time.Second, duration: 5 * time.Second, operationTimeout: 10 * time.Second}
	if err := validate(valid); err != nil {
		t.Fatal(err)
	}
	invalid := map[string]func(*settings){
		"unknown mode":            func(s *settings) { s.mode = "bulk" },
		"missing diagnostics":     func(s *settings) { s.weirDiagnostics = "" },
		"hostname diagnostics":    func(s *settings) { s.weirDiagnostics = "localhost:7449" },
		"remote diagnostics":      func(s *settings) { s.weirDiagnostics = "192.0.2.1:7449" },
		"missing CPU budget":      func(s *settings) { s.databaseCPUs = 0 },
		"descending ladder":       func(s *settings) { s.levels = "256,64" },
		"uneven process split":    func(s *settings) { s.levels = "64,100" },
		"too many per process":    func(s *settings) { s.clientProcesses = 1; s.levels = "2048" },
		"records below workers":   func(s *settings) { s.records = 255 },
		"short measurement stage": func(s *settings) { s.duration = time.Second },
		"unbound diagnostics":     func(s *settings) { s.weirDiagnostics = "127.0.0.1:0" },
		"NaN CPU budget":          func(s *settings) { s.databaseCPUs = math.NaN() },
		"too many rounds":         func(s *settings) { s.rounds = 11 },
		"no operation timeout":    func(s *settings) { s.operationTimeout = 0 },
		"long operation timeout":  func(s *settings) { s.operationTimeout = 2 * time.Minute },
	}
	for name, mutate := range invalid {
		input := valid
		mutate(&input)
		if err := validate(input); err == nil {
			t.Fatal("accepted", name)
		}
	}
	remote := valid
	remote.weirDiagnostics, remote.allowRemote = "192.0.2.1:7449", true
	if err := validate(remote); err != nil {
		t.Fatal("explicit remote diagnostics rejected:", err)
	}
}

func TestRunTimeoutCoversEverySaturationStageUnlessSet(t *testing.T) {
	input := settings{mode: "saturation", rounds: 3, warmupDuration: 10 * time.Second, duration: 20 * time.Second}
	// 4 levels x 3 rounds x 2 paths, each with a minute of slack, plus 5 minutes.
	if got, want := runTimeout(input, 4), 24*(90*time.Second)+5*time.Minute; got != want {
		t.Fatal("derived saturation timeout", got, want)
	}
	input.timeout = time.Minute
	if runTimeout(input, 4) != time.Minute {
		t.Fatal("explicit timeout was not honoured")
	}
	fixed := settings{mode: "fixed"}
	if runTimeout(fixed, 1) != 3*time.Minute {
		t.Fatal("fixed mode lost its default timeout")
	}
}
