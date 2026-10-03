package benchmark

import "testing"

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
