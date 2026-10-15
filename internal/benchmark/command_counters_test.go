package benchmark

import (
	"strings"
	"testing"

	"github.com/batchstream/weir-tests/internal/workload"
)

func TestPhysicalCommandDeltaPreservesUnavailableAndRejectsResets(t *testing.T) {
	before := &workload.DatabaseCounters{Commands: map[string]uint64{"find": 100, "update": 30}, CommandUnavailable: "missing bulkWrite"}
	after := &workload.DatabaseCounters{Commands: map[string]uint64{"find": 115, "update": 2, "getMore": 1}, CommandUnavailable: "missing bulkWrite"}
	result := SaturationResult{DatabaseBefore: before, DatabaseAfter: after, PhysicalCommandUnavailable: "sampling warning"}
	result.calculatePhysicalCommands()
	if len(result.PhysicalCommands) != 1 || result.PhysicalCommands["find"] != 15 {
		t.Fatal("missing or reset counters counted as measured work", result.PhysicalCommands)
	}
	for _, reason := range []string{"sampling warning", "missing bulkWrite", "unavailable or reset update", "unavailable or reset getMore"} {
		if strings.Count(result.PhysicalCommandUnavailable, reason) != 1 {
			t.Fatal("counter evidence was hidden or duplicated", reason, result.PhysicalCommandUnavailable)
		}
	}
	missing := SaturationResult{DatabaseAfter: after}
	missing.calculatePhysicalCommands()
	if missing.PhysicalCommands != nil {
		t.Fatal("missing initial observation fabricated command counts")
	}
}

func TestCommandUnavailableMergesBeforeAfterReasonsWithoutDuplicates(t *testing.T) {
	before := "missing bulkWrite; missing getMore; "
	after := "missing bulkWrite; missing getMore; missing killCursors; "
	combined := mergeCommandUnavailable(before, after, "unavailable or reset find", "unavailable or reset find")
	if combined != "missing bulkWrite; missing getMore; missing killCursors; unavailable or reset find" {
		t.Fatal("before/after unavailable evidence repeated or disappeared", combined)
	}
	message := "Elasticsearch node statistics do not count physical client HTTP commands"
	if combined := mergeCommandUnavailable(message, message); combined != message {
		t.Fatal("identical non-counter observation reason repeated", combined)
	}
}
