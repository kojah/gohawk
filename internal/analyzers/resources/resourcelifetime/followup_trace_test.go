package resourcelifetime

import (
	"encoding/json"
	"strings"
	"testing"
)

func assertFollowupBoundaryTrace(t *testing.T, data []byte) {
	t.Helper()
	want := map[string]string{
		"paired-error-helper-cleanup": "paired_error_cleanup.go:",
		"rows-transaction-finished":   "sql_row_parents.go:",
	}
	foundSentinel := false
	for line := range strings.SplitSeq(strings.TrimSpace(string(data)), "\n") {
		var event struct {
			Reason    string            `json:"reason"`
			Phase     string            `json:"phase"`
			Outcome   string            `json:"outcome"`
			Candidate string            `json:"candidate"`
			Details   map[string]string `json:"details"`
		}
		if err := json.Unmarshal([]byte(line), &event); err != nil {
			t.Fatal(err)
		}
		if event.Reason == "acquisition-error-proven" &&
			event.Details["proof"] == "exact-error-equals-non-nil-filesystem-sentinel" {
			foundSentinel = true
			if event.Phase != "evidence" || event.Outcome != "accepted" || !strings.Contains(event.Candidate, "error_guards.go:") {
				t.Errorf("unexpected sentinel evidence: %+v", event)
			}
		}
		file, ok := want[event.Reason]
		if !ok || !strings.Contains(event.Candidate, file) {
			continue
		}
		if event.Phase != "evidence" || event.Outcome != "accepted" {
			t.Errorf("unexpected cleanup boundary evidence: %+v", event)
		}
		delete(want, event.Reason)
	}
	if !foundSentinel || len(want) != 0 {
		t.Errorf("missing followup evidence: sentinel=%t missing=%v", foundSentinel, want)
	}
}
