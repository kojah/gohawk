package goroutineownership

import (
	"encoding/json"
	"os"
	"strings"
	"testing"
)

func assertFollowupBoundaryTrace(t *testing.T, path string) {
	t.Helper()
	want := map[string][2]string{
		"joinedBeforeDeferredCompletion":                 {"join-proven", "accepted"},
		"joinedBeforeDeferredClose":                      {"join-proven", "accepted"},
		"deferredWorkStillNeedsCompletion":               {"unowned-return", "rejected"},
		"consumesFactoryCompanion":                       {"opaque-ownership-transfer", "unknown"},
		"helperSettlesStoredSignal":                      {"opaque-ownership-transfer", "unknown"},
		"aggregateInspectionDoesNotSettle":               {"unowned-return", "rejected"},
		"canceledContextPassedToOpaqueWorker":            {"locally-canceled-context", "unknown"},
		"opaqueWorkerWithConditionalCancellation":        {"unowned-return", "rejected"},
		"assertedOwnerHandoff":                           {"opaque-ownership-transfer", "unknown"},
		"assertedOwnerNotHandedOff":                      {"unowned-return", "rejected"},
		"bufferedResultDoesNotReplaceDeferredCompletion": {"unowned-return", "rejected"},
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	for line := range strings.SplitSeq(strings.TrimSpace(string(data)), "\n") {
		var event struct {
			Function  string `json:"function"`
			Reason    string `json:"reason"`
			Phase     string `json:"phase"`
			Outcome   string `json:"outcome"`
			Candidate string `json:"candidate"`
			Position  string `json:"position"`
		}
		if err := json.Unmarshal([]byte(line), &event); err != nil {
			t.Fatal(err)
		}
		name := strings.TrimPrefix(event.Function, "goroutineownership.")
		expected, ok := want[name]
		if !ok || event.Phase != "decision" || event.Reason != expected[0] {
			continue
		}
		if event.Outcome != expected[1] || event.Candidate == "" || event.Candidate != event.Position {
			t.Errorf("invalid follow-up boundary event: %+v", event)
		}
		delete(want, name)
	}
	if len(want) > 0 {
		t.Errorf("missing follow-up boundary traces: %v", want)
	}
}
