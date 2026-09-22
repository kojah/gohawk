package resourcelifetime

import (
	"encoding/json"
	"strings"
	"testing"
)

func assertFollowupBoundaryTrace(t *testing.T, data []byte) {
	t.Helper()
	want := map[string]string{
		"stored-by-callee":                    "private_retention.go:",
		"returned-logger-retains-writer":      "returned_loggers.go:",
		"prior-defer-may-clean-captured-cell": "prior_captured_cleanup.go:",
		"paired-error-helper-cleanup":         "paired_error_cleanup.go:",
		"rows-transaction-finished":           "sql_row_parents.go:",
		"captured-body-guarded-cleanup":       "http_guarded_capture.go:",
	}
	proofFiles := map[string]string{
		"exact-error-equals-non-nil-filesystem-sentinel": "error_guards.go:",
		"visible-error-predicate-false-for-nil":          "error_predicates.go:",
	}
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
		if file, ok := proofFiles[event.Details["proof"]]; ok && event.Reason == "acquisition-error-proven" {
			if event.Phase != "evidence" || event.Outcome != "accepted" || !strings.Contains(event.Candidate, file) {
				t.Errorf("unexpected acquisition-error evidence: %+v", event)
			}
			delete(proofFiles, event.Details["proof"])
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
	if len(proofFiles) != 0 || len(want) != 0 {
		t.Errorf("missing followup evidence: proofs=%v missing=%v", proofFiles, want)
	}
}
