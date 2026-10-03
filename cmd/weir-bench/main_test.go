package main

import "testing"

func TestEndpointValidationRequiresExplicitOptInAndNoCredentials(t *testing.T) {
	valid := settings{backend: "mongo", mongoURI: "mongodb://127.0.0.1:27028/?directConnection=true", weirSeed: "127.0.0.1:7447", store: "mongo", timeout: 1}
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
