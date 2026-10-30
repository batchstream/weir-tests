// Package soak runs a long write and read workload through Weir and checks
// every reported outcome against the databases directly.
package soak

import (
	"slices"
	"sync"
	"time"
)

// keyState is what one worker knows about a key it alone writes. Writes to a
// key are sequential and carry increasing sequence numbers, so the database
// may hold only the latest APPLIED sequence or a sequence whose outcome was
// UNKNOWN. A refused write gets a fresh sequence when retried, so a refusal
// that took effect stays visible.
type keyState struct {
	id      string
	issued  int64
	applied int64
	unknown []unknownWrite
}

type unknownWrite struct {
	seq    int64
	issued time.Time
}

// allows reports whether the database may hold seq for this key; seq 0 means
// the key is absent.
func (k *keyState) allows(seq int64) bool {
	if seq == 0 {
		return k.applied == 0
	}
	return seq == k.applied || slices.ContainsFunc(k.unknown, func(write unknownWrite) bool { return write.seq == seq })
}

// settle records that the write of seq issued at issued was APPLIED. An
// unknown write lands within lateWindow of being issued if at all, so one
// issued earlier than that before seq was replaced by it.
func (k *keyState) settle(seq int64, issued time.Time, lateWindow time.Duration) {
	k.applied = seq
	horizon := issued.Add(-lateWindow)
	k.unknown = slices.DeleteFunc(k.unknown, func(write unknownWrite) bool { return write.issued.Before(horizon) })
}

func (k *keyState) unknownSeqs() []int64 {
	seqs := make([]int64, len(k.unknown))
	for index, write := range k.unknown {
		seqs[index] = write.seq
	}
	return seqs
}

// Violation is an observation that contradicts the reported outcomes.
type Violation struct {
	At       string  `json:"at_utc"`
	Backend  string  `json:"backend"`
	Kind     string  `json:"kind"`
	Key      string  `json:"key"`
	Observed int64   `json:"observed_seq"`
	Applied  int64   `json:"applied_seq"`
	Unknown  []int64 `json:"unknown_seqs,omitempty"`
	Detail   string  `json:"detail,omitempty"`
}

// violationLog keeps the first violations in full and counts the rest.
type violationLog struct {
	mu    sync.Mutex
	count int
	kept  []Violation
}

const keptViolations = 1000

func (l *violationLog) add(backend, kind string, key *keyState, observed int64) {
	violation := Violation{
		At:       time.Now().UTC().Format(time.RFC3339Nano),
		Backend:  backend,
		Kind:     kind,
		Key:      key.id,
		Observed: observed,
		Applied:  key.applied,
		Unknown:  key.unknownSeqs(),
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	l.count++
	if len(l.kept) < keptViolations {
		l.kept = append(l.kept, violation)
	}
}

func (l *violationLog) snapshot() (int, []Violation) {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.count, slices.Clone(l.kept)
}
