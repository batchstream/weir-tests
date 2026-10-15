package benchmark

import (
	"errors"
	"sync"
)

type crawlerApplicationCounts struct {
	Requests          uint64         `json:"requests"`
	Records           uint64         `json:"records"`
	MinimumRecords    int            `json:"minimum_records"`
	MaximumRecords    int            `json:"maximum_records"`
	RecordsPerRequest map[int]uint64 `json:"records_per_request"`
}
type crawlerApplicationLedger struct {
	mu     sync.Mutex
	counts crawlerApplicationCounts
}

func (l *crawlerApplicationLedger) record(records int) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.counts.RecordsPerRequest == nil {
		l.counts.RecordsPerRequest = map[int]uint64{}
	}
	if l.counts.Requests == 0 || records < l.counts.MinimumRecords {
		l.counts.MinimumRecords = records
	}
	l.counts.Requests++
	l.counts.Records += uint64(records)
	l.counts.MaximumRecords = max(l.counts.MaximumRecords, records)
	l.counts.RecordsPerRequest[records]++
}
func (l *crawlerApplicationLedger) snapshot() crawlerApplicationCounts {
	l.mu.Lock()
	defer l.mu.Unlock()
	result := l.counts
	result.RecordsPerRequest = make(map[int]uint64, len(l.counts.RecordsPerRequest))
	for records, count := range l.counts.RecordsPerRequest {
		result.RecordsPerRequest[records] = count
	}
	return result
}
func mergeCrawlerApplicationCounts(total *crawlerApplicationCounts, part crawlerApplicationCounts) {
	if total.RecordsPerRequest == nil {
		total.RecordsPerRequest = map[int]uint64{}
	}
	if part.Requests > 0 && (total.Requests == 0 || part.MinimumRecords < total.MinimumRecords) {
		total.MinimumRecords = part.MinimumRecords
	}
	total.Requests += part.Requests
	total.Records += part.Records
	total.MaximumRecords = max(total.MaximumRecords, part.MaximumRecords)
	for records, count := range part.RecordsPerRequest {
		total.RecordsPerRequest[records] += count
	}
}
func auditCrawlerApplicationCounts(counts crawlerApplicationCounts, shape string) error {
	var requests, records uint64
	var minimum, maximum int
	for width, count := range counts.RecordsPerRequest {
		if width < 1 || count < 1 {
			return errors.New("invalid application request histogram")
		}
		if minimum == 0 || width < minimum {
			minimum = width
		}
		maximum = max(maximum, width)
		requests += count
		records += uint64(width) * count
	}
	if requests != counts.Requests || records != counts.Records || minimum != counts.MinimumRecords || maximum != counts.MaximumRecords {
		return errors.New("application request histogram/count mismatch")
	}
	if shape == "single" && requests > 0 && (minimum != 1 || maximum != 1 || counts.RecordsPerRequest[1] != requests) {
		return errors.New("single-record workload contains a client batch")
	}
	return nil
}
