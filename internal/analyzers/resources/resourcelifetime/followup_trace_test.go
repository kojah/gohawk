package resourcelifetime

import (
	"encoding/json"
	"strings"
	"testing"
)

func assertFollowupBoundaryTrace(t *testing.T, data []byte) {
	t.Helper()
	want := map[string]string{
		"returned-logger-retains-writer":      "returned_loggers.go:",
		"prior-defer-may-clean-captured-cell": "prior_captured_cleanup.go:",
		"paired-error-helper-cleanup":         "paired_error_cleanup.go:",
		"rows-transaction-finished":           "sql_row_parents.go:",
	}
	foundSentinel := false
	acquisitions := map[string]bool{
		"head-body-acquisition-uncertain":  false,
		"local-header-only-body-uncertain": false,
	}
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
		if _, ok := acquisitions[event.Reason]; ok {
			acquisitions[event.Reason] = true
			if event.Phase != "decision" || event.Outcome != "unknown" || !strings.Contains(event.Candidate, "http_") {
				t.Errorf("unexpected HTTP acquisition boundary: %+v", event)
			}
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
	for reason, found := range acquisitions {
		if !found {
			t.Errorf("missing acquisition boundary: %s", reason)
		}
	}
	if !foundSentinel || len(want) != 0 {
		t.Errorf("missing followup evidence: sentinel=%t missing=%v", foundSentinel, want)
	}
}
