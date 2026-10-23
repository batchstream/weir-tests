package main

import "testing"

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
		input := settings{backend: "mongo", mongoURI: test.uri, allowRemote: test.remote, weirAddress: "127.0.0.1:7447", store: "mongo", timeout: 1}
		if err := validate(input); (err == nil) != test.valid {
			t.Fatalf("valid=%v: %v", test.valid, err)
		}
	}
}

func TestEndpointValidationRequiresExplicitOptInAndNoCredentials(t *testing.T) {
	valid := settings{backend: "mongo", mongoURI: "mongodb://127.0.0.1:27028/?directConnection=true", weirAddress: "127.0.0.1:7447", store: "mongo", timeout: 1}
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
