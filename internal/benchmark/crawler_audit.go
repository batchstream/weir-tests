package benchmark

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"time"
)

// AuditCrawlerReport checks arithmetic and reconstructs all expected persisted fields
// from acknowledged page ordinals without a database or a benchmark process.
func AuditCrawlerReport(report *CrawlerReport) error {
	if report == nil || report.Schema != 1 || report.Incomplete != "" || report.CleanupError != "" {
		return errors.New("crawler report is incomplete or cleanup failed")
	}
	if !reflect.DeepEqual(report.Sources, crawlerSources()) {
		return errors.New("crawler report Lua/native fixture hashes differ from this auditor")
	}
	for _, name := range []string{"current", "optimized"} {
		server, ok := report.Servers[name]
		if !ok {
			return errors.New("crawler server receipt missing")
		}
		var receipt struct {
			Source, SHA256 string
			SourceDirty    bool `json:"source_dirty"`
			Identity       struct {
				Revision string `json:"revision"`
			} `json:"identity"`
		}
		if err := json.Unmarshal(server.Receipt, &receipt); err != nil {
			return err
		}
		source, sourceErr := hex.DecodeString(receipt.Source)
		digest, digestErr := hex.DecodeString(receipt.SHA256)
		if sourceErr != nil || len(source) != 20 || digestErr != nil || len(digest) != 32 || receipt.SHA256 != server.SHA256 || receipt.SourceDirty || receipt.Identity.Revision != receipt.Source {
			return errors.New("crawler server receipt has invalid immutable identity")
		}
	}
	options := report.Parameters
	if options.HTTPObserver != "" && options.HTTPObserver != "enabled" && options.HTTPObserver != "disabled" {
		return errors.New("unknown HTTP observer mode")
	}
	if len(options.Concurrency) == 0 || options.PageSize < 1 || options.Processes < 1 {
		return errors.New("invalid crawler report parameters")
	}
	cases, err := crawlerCases(options)
	if err != nil {
		return err
	}
	expectedStages := len(cases) * options.Rounds * 3
	if len(report.Stages) != expectedStages {
		return errors.New("crawler stage matrix is incomplete")
	}
	paths := []string{"direct", "current", "optimized"}
	stageIndex := 0
	for caseIndex, test := range cases {
		for round := range options.Rounds {
			for offset := range paths {
				stage := report.Stages[stageIndex]
				stageIndex++
				path := paths[(caseIndex+round+offset)%3]
				if stage.Path != path || stage.Round != round+1 || stage.Profile != test.Profile || stage.Mode != test.Mode || stage.Concurrency != test.Workers || stage.ProductsPerSecond != test.Rate {
					return fmt.Errorf("crawler stage %d does not follow the declared same-runner rotation", stageIndex)
				}
				if !stage.BeforeWarmupSettling.Settled || !stage.BeforeMeasurementSettling.Settled || stage.BeforeWarmupSettling.RequiredQuietNS < int64(2*time.Second) || stage.BeforeMeasurementSettling.RequiredQuietNS < int64(2*time.Second) || !stage.Verified || !stage.ClientsExited || stage.VerificationError != "" || len(stage.ClientExitErrors) != 0 || stage.Failed != 0 || stage.Indeterminate != 0 || stage.TimedOut != 0 || stage.ElapsedNS <= 0 || len(stage.Clients) != options.Processes {
					return fmt.Errorf("crawler stage %d is unqualified", stageIndex)
				}
				server := "current"
				if path == "optimized" {
					server = "optimized"
				}
				if stage.ServerSHA256 != report.Servers[server].SHA256 {
					return errors.New("crawler stage used a different server binary")
				}
				if stage.CPUQuota == nil || stage.CPUQuota.ObservedCPUs != options.CPUs || stage.CPUQuota.RequestedCPUs != options.CPUs {
					return errors.New("crawler database CPU quota mismatch")
				}
				expected := map[string]map[string]any{}
				for worker := range options.Concurrency[len(options.Concurrency)-1] {
					for record := range options.PageSize * 2 {
						for _, suffix := range []string{"product", "seller-0", "seller-1"} {
							id := crawlerID(worker, record, suffix)
							expected[id] = crawlerDocument(id, suffix == "product")
						}
					}
				}
				expected["shared-seller"] = crawlerDocument("shared-seller", false)
				var offered, rejected, completed, failed, unknown, timedout, late, within, mutations uint64
				var latency, service histogram
				seenPIDs := map[int]bool{}
				seenWorkers := map[int]bool{}
				for process, reply := range stage.Clients {
					if reply.Phase != "timed" || reply.PID < 1 || seenPIDs[reply.PID] || len(reply.Workers) != stage.Concurrency/options.Processes {
						return errors.New("crawler client identity or goroutine count mismatch")
					}
					seenPIDs[reply.PID] = true
					for index, worker := range reply.Workers {
						if worker.PID != reply.PID || worker.Worker != process*(stage.Concurrency/options.Processes)+index || seenWorkers[worker.Worker] {
							return errors.New("crawler worker identity mismatch")
						}
						seenWorkers[worker.Worker] = true
						if worker.Offered != worker.Rejected+worker.Completed+worker.Failed || worker.Completed != uint64(len(worker.AppliedPages)*options.PageSize) || worker.Completed != worker.WithinWindow+worker.Late {
							return errors.New("crawler worker offered/completed accounting mismatch")
						}
						if worker.QueuePeak > options.QueuePages || worker.QueuePeak < 0 {
							return errors.New("crawler worker queue exceeded its bound")
						}
						for index, page := range worker.AppliedPages {
							if page < 0 || index > 0 && page <= worker.AppliedPages[index-1] {
								return errors.New("crawler acknowledged pages are out of order")
							}
							for _, mutation := range crawlerPage(worker.Worker, page, options.PageSize) {
								expected[mutation.ID] = crawlerMerge(expected[mutation.ID], mutation)
							}
						}
						child := histogram{buckets: worker.Buckets, count: worker.Latency.Samples, max: worker.Latency.MaxNS}
						childService := histogram{buckets: worker.ServiceBuckets, count: worker.ServiceLatency.Samples, max: worker.ServiceLatency.MaxNS}
						var samples uint64
						for _, count := range worker.Buckets {
							samples += count
						}
						if samples != worker.Latency.Samples || worker.Latency.Samples != (worker.Completed+worker.Failed)/uint64(options.PageSize) || !reflect.DeepEqual(worker.Latency, child.summary()) || !reflect.DeepEqual(worker.ServiceLatency, childService.summary()) {
							return errors.New("crawler worker latency histogram mismatch")
						}
						latency.merge(&child)
						service.merge(&childService)
						offered += worker.Offered
						rejected += worker.Rejected
						completed += worker.Completed
						failed += worker.Failed
						unknown += worker.Indeterminate
						timedout += worker.TimedOut
						late += worker.Late
						within += worker.WithinWindow
						mutations += worker.RequestedMutations
					}
				}
				if offered != stage.Offered || rejected != stage.Rejected || completed != stage.Completed || failed != stage.Failed || unknown != stage.Indeterminate || timedout != stage.TimedOut || late != stage.Late || within != stage.WithinWindow || mutations != stage.RequestedMutations {
					return errors.New("crawler pooled count mismatch")
				}
				if !reflect.DeepEqual(stage.ScheduledToVisible, latency.summary()) || !reflect.DeepEqual(stage.DispatchToVisible, service.summary()) {
					return errors.New("crawler pooled latency mismatch")
				}
				rate := float64(completed) / (float64(stage.ElapsedNS) / 1e9)
				if stage.CompletedProductsPerSecond != rate {
					return errors.New("crawler completed throughput mismatch")
				}
				raw, _ := json.Marshal(expected)
				sum := sha256.Sum256(raw)
				if stage.ExpectedDocumentsSHA256 != hex.EncodeToString(sum[:]) || stage.VerifiedDocuments != len(expected) {
					return errors.New("crawler full-field expected hash is not reproducible from acknowledged pages")
				}
				if err := auditCrawlerHTTPObservation(stage, options.HTTPObserver); err != nil {
					return err
				}
			}
		}
	}
	return nil
}
