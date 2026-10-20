package workload

import (
	"errors"
	"net"
	"net/netip"
	"regexp"
	"strconv"
	"strings"
	"time"

	"go.mongodb.org/mongo-driver/v2/mongo/options"
	"go.mongodb.org/mongo-driver/v2/mongo/readpref"
	"go.mongodb.org/mongo-driver/v2/mongo/writeconcern"
)

type mongoProfile struct {
	Seeds      []string
	ReplicaSet string
	Direct     bool
}

var replicaName = regexp.MustCompile(`^[A-Za-z0-9_-]{1,64}$`)
var dnsLabel = regexp.MustCompile(`^[a-z0-9](?:[a-z0-9-]{0,61}[a-z0-9])?$`)

// ValidateMongoURI checks the bounded credential-free benchmark profile without
// resolving DNS or dialing. Seed authorization does not confine discovered peers.
func ValidateMongoURI(raw string, allowRemote bool) error {
	_, err := parseMongoProfile(raw, allowRemote)
	return err
}

func parseMongoProfile(raw string, allowRemote bool) (mongoProfile, error) {
	profile := mongoProfile{Direct: true}
	invalid := errors.New("invalid credential-free Mongo benchmark profile")
	if len(raw) > 4096 || !strings.HasPrefix(raw, "mongodb://") || strings.ContainsAny(raw, "@%#\\\t\r\n ") {
		return profile, invalid
	}
	rest := strings.TrimPrefix(raw, "mongodb://")
	authority, query, hasQuery := strings.Cut(rest, "?")
	if slash := strings.IndexByte(authority, '/'); slash >= 0 {
		if authority[slash:] != "/" {
			return profile, invalid
		}
		authority = authority[:slash]
	} else if hasQuery {
		return profile, invalid
	}
	directValue := ""
	if hasQuery {
		if query == "" {
			return profile, invalid
		}
		seen := map[string]bool{}
		for _, pair := range strings.Split(query, "&") {
			key, value, ok := strings.Cut(pair, "=")
			if !ok || value == "" || seen[key] {
				return profile, invalid
			}
			seen[key] = true
			switch key {
			case "replicaSet":
				if !replicaName.MatchString(value) {
					return profile, invalid
				}
				profile.ReplicaSet = value
			case "directConnection":
				if value != "true" && value != "false" {
					return profile, invalid
				}
				directValue = value
			default:
				return profile, invalid
			}
		}
	}
	seeds := strings.Split(authority, ",")
	if len(seeds) > 7 {
		return profile, invalid
	}
	seen := map[string]bool{}
	for _, seed := range seeds {
		host, port, err := net.SplitHostPort(seed)
		if err != nil || host == "" || port == "" {
			return profile, invalid
		}
		for _, digit := range port {
			if digit < '0' || digit > '9' {
				return profile, invalid
			}
		}
		number, err := strconv.Atoi(port)
		if err != nil || number < 1 || number > 65535 {
			return profile, invalid
		}
		host = strings.ToLower(host)
		local := host == "localhost"
		ip, err := netip.ParseAddr(host)
		if strings.ContainsAny(seed, "[]") && (err != nil || !ip.Is6()) {
			return profile, invalid
		}
		if err == nil {
			ip = ip.Unmap()
			if ip.Zone() != "" || ip.IsUnspecified() || ip.IsMulticast() {
				return profile, invalid
			}
			host = ip.String()
			local = ip.IsLoopback()
		} else {
			if strings.ContainsAny(seed, "[]") || len(host) > 253 {
				return profile, invalid
			}
			for _, label := range strings.Split(host, ".") {
				if !dnsLabel.MatchString(label) {
					return profile, invalid
				}
			}
		}
		if !allowRemote && !local {
			return profile, errors.New("non-loopback Mongo seed requires explicit --allow-remote")
		}
		canonical := net.JoinHostPort(host, strconv.Itoa(number))
		if seen[canonical] {
			return profile, invalid
		}
		seen[canonical] = true
		profile.Seeds = append(profile.Seeds, canonical)
	}
	if profile.ReplicaSet != "" {
		if directValue == "true" {
			return profile, invalid
		}
		profile.Direct = false
	} else if len(seeds) != 1 || directValue == "false" {
		return profile, invalid
	}
	return profile, nil
}

// nativeMongoOptions is the options builder consumed by openDirect, including
// its discovery choice. The path name "direct" means native client access.
func nativeMongoOptions(config Config) (*options.ClientOptions, mongoProfile, error) {
	profile, err := parseMongoProfile(config.MongoURI, config.AllowRemote)
	if err != nil {
		return nil, profile, err
	}
	opts := options.Client().SetHosts(profile.Seeds).SetDirect(profile.Direct).
		SetMaxPoolSize(uint64(config.Concurrency)).SetMinPoolSize(0).
		SetRetryWrites(false).SetRetryReads(false).SetMaxAdaptiveRetries(0).
		SetEnableOverloadRetargeting(false).SetCompressors(nil).
		SetServerMonitoringMode(options.ServerMonitoringModePoll).
		SetReadPreference(readpref.Primary()).SetWriteConcern(writeconcern.Majority()).
		SetServerSelectionTimeout(2 * time.Second).SetConnectTimeout(2 * time.Second)
	if !profile.Direct {
		opts.SetReplicaSet(profile.ReplicaSet).SetHeartbeatInterval(time.Second)
	}
	return opts, profile, nil
}
