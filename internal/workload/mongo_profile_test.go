package workload

import (
	"context"
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestMongoProfiles(t *testing.T) {
	tests := []struct {
		uri           string
		remote, valid bool
	}{
		{"mongodb://localhost:65535/?replicaSet=" + strings.Repeat("r", 64), false, true},
		{"mongodb://localhost:1,localhost:2,localhost:3,localhost:4,localhost:5,localhost:6,localhost:7/?replicaSet=r", false, true},
		{"mongodb://LOCALHOST:027017", false, true},
		{"mongodb://[::1]:1/?directConnection=true", false, true},
		{"mongodb://127.0.0.1:1,[::1]:2,[::ffff:127.0.0.2]:3/?replicaSet=rs_0-1", false, true},
		{"mongodb://[::1]:1,[::2]:2/?replicaSet=rs&directConnection=false", true, true},
		{"mongodb://nonexistent.invalid:1/?replicaSet=rs", true, true},
		{"mongodb://nonexistent.invalid:1/?replicaSet=rs", false, false},
		{"mongodb://127.0.0.1:1,192.0.2.1:2/?replicaSet=rs", false, false},
		{"mongodb://192.0.2.1:2,127.0.0.1:1/?replicaSet=rs", false, false},
		{"mongodb://localhost:1,LOCALHOST:01/?replicaSet=rs", true, false},
		{"mongodb://127.0.0.1:1,[::ffff:127.0.0.1]:01/?replicaSet=rs", true, false},
		{"mongodb://localhost:1/?replicaSet=rs&directConnection=true", false, false},
		{"mongodb://localhost:1/?directConnection=false", false, false},
		{"mongodb://localhost:1,localhost:2", false, false},
	}
	for _, test := range tests {
		t.Run(test.uri, func(t *testing.T) {
			err := ValidateMongoURI(test.uri, test.remote)
			if (err == nil) != test.valid {
				t.Fatalf("valid=%v: %v", test.valid, err)
			}
		})
	}
	rejected := []string{
		"mongodb://localhost:1?replicaSet=rs", "mongodb://localhost:1?directConnection=true",
		"mongodb://[127.0.0.1]:1", "mongodb://localhost:1/?directConnection=true&directConnection=true",
		"mongodb://localhost:1/?replicaSet=r&replicaset=r", "mongodb://localhost:1/?replicaSet=r&", "mongodb://localhost:1/?replicaSet=r%73",
		"mongodb+srv://localhost:1", "mongodb://user:secret@localhost:1", "mongodb://local%68ost:1",
		"mongodb://[fe80::1%eth0]:1", "mongodb://localhost:1/#x", "mongodb://localhost:1/db",
		"mongodb://localhost", "mongodb://localhost:0", "mongodb://localhost:65536", "mongodb://localhost:+1",
		"mongodb://localhost:1/?", "mongodb://localhost:1/?replicaSet=", "mongodb://localhost:1/?replicaSet=a&replicaSet=b",
		"mongodb://localhost:1/?ReplicaSet=rs", "mongodb://localhost:1/?directConnection=True",
		"mongodb://localhost:1/?authSource=secret", "mongodb://localhost:1/?tls=true",
		"mongodb://localhost:1/?loadBalanced=true", "mongodb://localhost:1/?readPreference=secondary",
		"mongodb://localhost:1/?replicaSet=" + strings.Repeat("a", 65),
		"mongodb://localhost:1/?replicaSet=" + strings.Repeat("a", 4096),
		"mongodb://localhost:1,localhost:2,localhost:3,localhost:4,localhost:5,localhost:6,localhost:7,localhost:8/?replicaSet=r",
		"mongodb://bad..host:1", "mongodb://-bad:1", "mongodb://[localhost]:1", "mongodb://0.0.0.0:1",
	}
	for _, uri := range rejected {
		if err := ValidateMongoURI(uri, true); err == nil {
			t.Fatalf("accepted invalid profile %q", uri)
		}
	}
}

func TestNativeMongoOptionsConsumedByOpenDirect(t *testing.T) {
	config := Config{MongoURI: "mongodb://LOCALHOST:027017,[::1]:27018/?replicaSet=rs", Concurrency: 5}
	opts, profile, err := nativeMongoOptions(config)
	if err != nil {
		t.Fatal(err)
	}
	if opts.Direct == nil || *opts.Direct || opts.ReplicaSet == nil || *opts.ReplicaSet != "rs" || !reflect.DeepEqual(opts.Hosts, []string{"localhost:27017", "[::1]:27018"}) || profile.Direct {
		t.Fatalf("discovery options lost: %+v", opts)
	}
	if opts.HeartbeatInterval == nil || *opts.HeartbeatInterval != time.Second || *opts.MaxPoolSize != 5 || *opts.RetryReads || *opts.RetryWrites || *opts.MaxAdaptiveRetries != 0 || *opts.EnableOverloadRetargeting || *opts.ServerSelectionTimeout != 2*time.Second || *opts.ConnectTimeout != 2*time.Second || len(opts.Compressors) != 0 || opts.ReadPreference.String() != "primary" || opts.WriteConcern.W != "majority" {
		t.Fatalf("native policy changed: %+v", opts)
	}
	if opts.ServerMonitoringMode == nil || *opts.ServerMonitoringMode != "poll" {
		t.Fatal("native monitoring must poll")
	}
	config.MongoURI = "mongodb://localhost:27017"
	opts, _, err = nativeMongoOptions(config)
	if err != nil || opts.Direct == nil || !*opts.Direct || opts.HeartbeatInterval != nil || opts.ReplicaSet != nil {
		t.Fatalf("direct options changed: %v %+v", err, opts)
	}
}

func TestProgrammaticMongoValidation(t *testing.T) {
	config := Config{Backend: "mongo", MongoURI: "mongodb://user:secret@localhost:1", StoreName: "mongo", Namespace: "weirtest_0123456789abcdef01234567", Records: 1, Concurrency: 1}
	if _, err := New(config); err == nil {
		t.Fatal("New bypassed validation")
	}
	dataset := &Dataset{Config: config}
	if _, err := openDirect(context.Background(), dataset); err == nil {
		t.Fatal("openDirect bypassed validation")
	}
}
