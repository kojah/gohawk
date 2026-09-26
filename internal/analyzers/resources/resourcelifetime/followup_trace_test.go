package resourcelifetime

import (
	"encoding/json"
	"strings"
	"testing"
)

type followupTraceEvent struct {
	Reason    string            `json:"reason"`
	Phase     string            `json:"phase"`
	Outcome   string            `json:"outcome"`
	Candidate string            `json:"candidate"`
	Details   map[string]string `json:"details"`
}

func decodeFollowupTrace(t *testing.T, data []byte) []followupTraceEvent {
	t.Helper()
	var events []followupTraceEvent
	for line := range strings.SplitSeq(strings.TrimSpace(string(data)), "\n") {
		var event followupTraceEvent
		if err := json.Unmarshal([]byte(line), &event); err != nil {
			t.Fatal(err)
		}
		events = append(events, event)
	}
	return events
}

func assertFollowupBoundaryTrace(t *testing.T, data []byte) {
	t.Helper()
	events := decodeFollowupTrace(t, data)
	assertHTTPBoundaryTrace(t, events)
	assertCleanupBoundaryTrace(t, events)
	assertUncertainEdgeTrace(t, events, "repeated-guard-edge-unknown", "guard_facts.go:")
	assertUncertainEdgeTrace(t, events, "rows-exhausted-edge-unknown", "sql_")
}

// A path that re-tests a guard it already took the other way, or leaves a
// Rows.Next loop on its false edge, becomes unknown on that edge, and the
// trace labels the branch.
func assertUncertainEdgeTrace(t *testing.T, events []followupTraceEvent, reason, file string) {
	t.Helper()
	for _, event := range events {
		if event.Reason != reason || !strings.Contains(event.Candidate, file) {
			continue
		}
		if event.Phase != "label" || event.Outcome != "unknown" || event.Details["branch"] == "" {
			t.Errorf("unexpected %s trace: %+v", reason, event)
		}
		return
	}
	t.Errorf("missing %s label", reason)
}

// The HTTP acquisition boundaries decide as unknown on their own fixtures, and
// a HEAD boundary that declines says which input failed, as a considered step
// on the Do site, so a trace needs no source to explain the report.
func assertHTTPBoundaryTrace(t *testing.T, events []followupTraceEvent) {
	t.Helper()
	acquisitions := map[string]bool{
		"head-body-acquisition-uncertain":  false,
		"local-header-only-body-uncertain": false,
	}
	declined := map[string]bool{
		"head-client-not-unconfigured": false,
		"head-request-modified":        false,
	}
	for _, event := range events {
		if _, ok := declined[event.Reason]; ok {
			declined[event.Reason] = true
			if event.Phase != "considered" || event.Outcome != "rejected" || !strings.Contains(event.Candidate, "http_head.go:") {
				t.Errorf("unexpected HEAD boundary rejection: %+v", event)
			}
		}
		if _, ok := acquisitions[event.Reason]; ok {
			acquisitions[event.Reason] = true
			if event.Phase != "decision" || event.Outcome != "unknown" || !strings.Contains(event.Candidate, "http_") {
				t.Errorf("unexpected HTTP acquisition boundary: %+v", event)
			}
		}
	}
	for reason, found := range acquisitions {
		if !found {
			t.Errorf("missing acquisition boundary: %s", reason)
		}
	}
	for reason, found := range declined {
		if !found {
			t.Errorf("missing HEAD boundary rejection: %s", reason)
		}
	}
}

func assertCleanupBoundaryTrace(t *testing.T, events []followupTraceEvent) {
	t.Helper()
	// A callee that stores the resource is summary evidence; the others are
	// boundaries the classifier labels unknown.
	type expectation struct{ file, phase, outcome string }
	want := map[string]expectation{
		"stored-by-callee":                    {"private_retention.go:", "evidence", "accepted"},
		"returned-wrapper-retains-resource":   {"returned_loggers.go:", "label", "unknown"},
		"prior-defer-may-clean-captured-cell": {"prior_captured_cleanup.go:", "label", "unknown"},
		"paired-error-helper-cleanup":         {"paired_error_cleanup.go:", "label", "unknown"},
		"rows-transaction-finished":           {"sql_row_parents.go:", "label", "unknown"},
		"captured-body-guarded-cleanup":       {"http_guarded_capture.go:", "label", "unknown"},
	}
	proofFiles := map[string]string{
		"exact-error-equals-non-nil-filesystem-sentinel": "error_guards.go:",
		"error-predicate-false-for-nil":                  "error_predicates.go:",
	}
	for _, event := range events {
		if file, ok := proofFiles[event.Details["proof"]]; ok && event.Reason == "acquisition-error-proven" {
			if event.Phase != "evidence" || event.Outcome != "accepted" || !strings.Contains(event.Candidate, file) {
				t.Errorf("unexpected acquisition-error evidence: %+v", event)
			}
			delete(proofFiles, event.Details["proof"])
		}
		expected, ok := want[event.Reason]
		if !ok || !strings.Contains(event.Candidate, expected.file) {
			continue
		}
		if event.Phase != expected.phase || event.Outcome != expected.outcome {
			t.Errorf("unexpected cleanup boundary evidence: %+v", event)
		}
		delete(want, event.Reason)
	}
	if len(proofFiles) != 0 || len(want) != 0 {
		t.Errorf("missing followup evidence: proofs=%v missing=%v", proofFiles, want)
	}
}
