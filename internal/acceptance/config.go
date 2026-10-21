// Package acceptance runs a finite functional contract against externally owned services.
package acceptance

import (
	"encoding/json"
	"errors"
	"io"
	"net/url"
	"regexp"
	"strings"

	"github.com/batchstream/weir-protocol/api/protocol"
	"github.com/batchstream/weir-tests/internal/workload"
)

const MaxInputBytes = 16 << 10

// Config is supplied by the owner/orchestrator, not discovered from cluster credentials.
// Candidate and Kubernetes identity are assertions the runner cannot independently prove.
type Config struct {
	ExpectedArch       string   `json:"expected_arch"`
	AllowRemote        bool     `json:"allow_remote"`
	ServerSource       string   `json:"server_source"`
	ServerOCI          string   `json:"server_oci"`
	Namespace          string   `json:"namespace"`
	NamespaceUID       string   `json:"namespace_uid"`
	PodName            string   `json:"pod_name"`
	PodUID             string   `json:"pod_uid"`
	RunOwner           string   `json:"run_owner"`
	ApplicationSeeds   []string `json:"application_seeds"`
	MongoURI           string   `json:"mongo_uri"`
	SearchURL          string   `json:"search_url"`
	MongoStore         string   `json:"mongo_store"`
	SearchStore        string   `json:"search_store"`
	OwnedDatasetPrefix string   `json:"owned_dataset_prefix"`
}

var sourcePattern = regexp.MustCompile(`^[0-9a-f]{40}$`)
var digestPattern = regexp.MustCompile(`^sha256:[0-9a-f]{64}$`)
var namePattern = regexp.MustCompile(`^[a-z0-9](?:[a-z0-9-]{0,61}[a-z0-9])?$`)
var ownerPattern = regexp.MustCompile(`^[a-z][a-z0-9]{7,23}$`)
var uidPattern = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)
var storePattern = regexp.MustCompile(`^[a-z][a-z0-9_-]{0,62}$`)

func Decode(input io.Reader) (Config, error) {
	var config Config
	raw, err := io.ReadAll(io.LimitReader(input, MaxInputBytes+1))
	if err != nil || len(raw) > MaxInputBytes {
		return config, errors.New("input_bound")
	}
	decoder := json.NewDecoder(strings.NewReader(string(raw)))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&config); err != nil {
		return config, errors.New("input_json")
	}
	if err := decoder.Decode(new(any)); err != io.EOF {
		return config, errors.New("input_trailing")
	}
	// Duplicate keys would otherwise silently replace the orchestrator's assertions.
	tokens := json.NewDecoder(strings.NewReader(string(raw)))
	opening, err := tokens.Token()
	if err != nil || opening != json.Delim('{') {
		return config, errors.New("input_json")
	}
	allowed := map[string]bool{}
	for _, name := range strings.Fields("expected_arch allow_remote server_source server_oci namespace namespace_uid pod_name pod_uid run_owner application_seeds mongo_uri search_url mongo_store search_store owned_dataset_prefix") {
		allowed[name] = true
	}
	seen := map[string]bool{}
	for tokens.More() {
		key, err := tokens.Token()
		if err != nil {
			return config, errors.New("input_json")
		}
		name, ok := key.(string)
		if !ok || seen[name] || !allowed[name] {
			return config, errors.New("input_duplicate_or_unknown")
		}
		seen[name] = true
		var value json.RawMessage
		if err := tokens.Decode(&value); err != nil {
			return config, errors.New("input_json")
		}
	}
	if len(seen) != len(allowed) {
		return config, errors.New("input_missing_field")
	}
	return config, nil
}

// Validate is offline: seed authorization is not confinement of SDK/Mongo discovered peers.
func Validate(config Config, target string) error {
	if (config.ExpectedArch != "amd64" && config.ExpectedArch != "arm64") || target != "linux/"+config.ExpectedArch {
		return errors.New("native_target")
	}
	if !config.AllowRemote {
		return errors.New("explicit_remote_opt_in")
	}
	if !sourcePattern.MatchString(config.ServerSource) || !digestPattern.MatchString(config.ServerOCI) {
		return errors.New("candidate_identity")
	}
	if !namePattern.MatchString(config.Namespace) || !namePattern.MatchString(config.PodName) ||
		!uidPattern.MatchString(config.NamespaceUID) || !uidPattern.MatchString(config.PodUID) || !ownerPattern.MatchString(config.RunOwner) {
		return errors.New("owner_identity")
	}
	if config.OwnedDatasetPrefix != "weiraccept_"+config.RunOwner+"_" {
		return errors.New("owned_prefix")
	}
	if !storePattern.MatchString(config.MongoStore) || !storePattern.MatchString(config.SearchStore) || config.MongoStore == config.SearchStore {
		return errors.New("stores")
	}
	if len(config.ApplicationSeeds) < 1 || len(config.ApplicationSeeds) > 3 {
		return errors.New("seed_bound")
	}
	seen := map[string]bool{}
	for _, seed := range config.ApplicationSeeds {
		canonical, err := protocol.CanonicalEndpoint(seed)
		if err != nil || len(seed) > 260 || canonical != seed || seen[seed] {
			return errors.New("seed_grammar")
		}
		seen[seed] = true
	}
	if err := workload.ValidateMongoURI(config.MongoURI, true); err != nil {
		return errors.New("mongo_profile")
	}
	address, err := url.Parse(config.SearchURL)
	if err != nil || len(config.SearchURL) > 512 || address.Scheme != "http" || address.Host == "" || address.User != nil || address.RawQuery != "" || address.Fragment != "" || address.Path != "" || address.Opaque != "" {
		return errors.New("search_profile")
	}
	canonical, err := protocol.CanonicalEndpoint(address.Host)
	if err != nil || canonical != address.Host {
		return errors.New("search_endpoint")
	}
	return nil
}
