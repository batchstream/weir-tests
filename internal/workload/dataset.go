// Package workload defines the identical logical database work used by both
// benchmark paths. It imports only published SDK and database client APIs.
package workload

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strings"

	"go.mongodb.org/mongo-driver/v2/bson"
)

type Config struct {
	Backend      string `json:"backend"`
	MongoURI     string `json:"-"`
	SearchURL    string `json:"-"`
	WeirSeed     string `json:"weir_seed"`
	StoreName    string `json:"store"`
	Namespace    string `json:"namespace"`
	Records      int    `json:"records"`
	PayloadBytes int    `json:"padding_bytes"`
	Concurrency  int    `json:"concurrency"`
	LuaMutations bool   `json:"lua_mutations,omitempty"`
}

type Operation struct {
	Record   int
	Write    bool
	Revision int
}

type Plan struct {
	Workers  [][]Operation
	Expected []int
	Count    int
	Reads    int
	Writes   int
}

type PlanOptions struct {
	Operations   int
	WritePercent int
}

type Dataset struct {
	Config  Config
	docs    [][2][]byte
	padding string
}

type document struct {
	ID       string `json:"fixture_id" bson:"fixture_id"`
	Revision int32  `json:"revision" bson:"revision"`
	Padding  string `json:"padding" bson:"padding"`
}

var namespacePattern = regexp.MustCompile(`^weirtest_[a-f0-9]{24}$`)
var storePattern = regexp.MustCompile(`^[a-z][a-z0-9_-]{0,62}$`)

// NewNamespace is fresh for every run; existing databases/indexes are never
// adopted, reset, or removed by the harness.
func NewNamespace() (string, error) {
	var nonce [12]byte
	if _, err := rand.Read(nonce[:]); err != nil {
		return "", err
	}
	return "weirtest_" + hex.EncodeToString(nonce[:]), nil
}

func New(config Config) (*Dataset, error) {
	if config.Backend != "mongo" && config.Backend != "search" {
		return nil, errors.New("backend must be mongo or search")
	}
	if !namespacePattern.MatchString(config.Namespace) || !storePattern.MatchString(config.StoreName) {
		return nil, errors.New("requires a generated namespace and valid Store name")
	}
	if config.Concurrency < 1 || config.Concurrency > 1024 || config.Records < config.Concurrency || config.Records > 100000 || config.PayloadBytes < 0 || config.PayloadBytes > 1<<20 {
		return nil, errors.New("invalid concurrency, record count, or padding size")
	}
	if int64(config.Records)*int64(config.PayloadBytes+128)*2 > 512<<20 {
		return nil, errors.New("precomputed document workspace exceeds 512 MiB")
	}
	dataset := &Dataset{Config: config, docs: make([][2][]byte, config.Records), padding: strings.Repeat("x", config.PayloadBytes)}
	for record := range config.Records {
		for revision := range 2 {
			doc := document{ID: dataset.ID(record), Revision: int32(revision), Padding: dataset.padding}
			var raw []byte
			var err error
			if config.Backend == "mongo" {
				fields := bson.D{{Key: "_id", Value: doc.ID}, {Key: "fixture_id", Value: doc.ID}, {Key: "revision", Value: doc.Revision}, {Key: "padding", Value: doc.Padding}}
				raw, err = bson.Marshal(fields)
			} else {
				raw, err = json.Marshal(doc)
			}
			if err != nil {
				return nil, err
			}
			dataset.docs[record][revision] = raw
		}
	}
	return dataset, nil
}

func (d *Dataset) ID(record int) string { return fmt.Sprintf("record-%08d", record) }

func (d *Dataset) Resource(record int) string {
	base := d.Config.Namespace
	if d.Config.Backend == "mongo" {
		base += "/records"
	}
	return base + "/s:" + d.ID(record)
}

// Document bytes are immutable and precomputed outside the timed region.
func (d *Dataset) Document(operation Operation) []byte {
	return d.docs[operation.Record][operation.Revision]
}

func (d *Dataset) ContentType() string {
	if d.Config.Backend == "mongo" {
		return "application/bson"
	}
	return "application/json"
}

// Plan partitions IDs between workers. Each worker is sequential, so read
// validation is deterministic even while other workers execute concurrently.
func (d *Dataset) Plan(options PlanOptions) (*Plan, error) {
	if options.Operations < d.Config.Concurrency || options.Operations > 10000000 || options.WritePercent < 0 || options.WritePercent > 100 {
		return nil, errors.New("invalid fixed operation count or write percentage")
	}
	plan := &Plan{Workers: make([][]Operation, d.Config.Concurrency), Expected: make([]int, d.Config.Records), Count: options.Operations}
	for worker := range d.Config.Concurrency {
		start := worker * d.Config.Records / d.Config.Concurrency
		end := (worker + 1) * d.Config.Records / d.Config.Concurrency
		count := options.Operations / d.Config.Concurrency
		if worker < options.Operations%d.Config.Concurrency {
			count++
		}
		operations := make([]Operation, count)
		for sequence := range count {
			// A fixed mixer avoids one long initial write burst. Identical plans
			// are reused by both paths and every repetition.
			x := uint64(sequence)*0x9e3779b97f4a7c15 + uint64(worker+1)*0xbf58476d1ce4e5b9
			x ^= x >> 30
			x *= 0xbf58476d1ce4e5b9
			x ^= x >> 27
			record := start + int(x%uint64(end-start))
			write := int((x>>32)%100) < options.WritePercent
			if write {
				// Each acknowledged replacement changes the persisted body,
				// including repeated writes to one worker-owned record.
				plan.Expected[record] = 1 - plan.Expected[record]
				plan.Writes++
			} else {
				plan.Reads++
			}
			operation := Operation{Record: record, Write: write, Revision: plan.Expected[record]}
			operations[sequence] = operation
		}
		plan.Workers[worker] = operations
	}
	return plan, nil
}

func (d *Dataset) Validate(raw []byte, operation Operation) error {
	var actual document
	var err error
	if d.Config.Backend == "mongo" {
		fields, parseErr := bson.Raw(raw).Elements()
		if parseErr != nil || len(fields) != 4 {
			return errors.New("persisted BSON field count mismatch")
		}
		for _, key := range []string{"_id", "fixture_id", "revision", "padding"} {
			if _, err := bson.Raw(raw).LookupErr(key); err != nil {
				return errors.New("persisted BSON field missing")
			}
		}
		id, ok := bson.Raw(raw).Lookup("_id").StringValueOK()
		if !ok || id != d.ID(operation.Record) {
			return errors.New("persisted BSON _id mismatch")
		}
		err = bson.Unmarshal(raw, &actual)
	} else {
		var fields map[string]json.RawMessage
		if err := json.Unmarshal(raw, &fields); err != nil || len(fields) != 3 || fields["fixture_id"] == nil || fields["revision"] == nil || fields["padding"] == nil {
			return errors.New("persisted JSON fields mismatch")
		}
		err = json.Unmarshal(raw, &actual)
	}
	if err != nil {
		return fmt.Errorf("decode persisted document: %w", err)
	}
	if actual.ID != d.ID(operation.Record) || actual.Revision != int32(operation.Revision) || actual.Padding != d.padding {
		return fmt.Errorf("document mismatch for %s revision %d", d.ID(operation.Record), operation.Revision)
	}
	return nil
}
