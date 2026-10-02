package lockorder

import (
	"encoding/json"
	"flag"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kojah/gohawk/internal/analyzertest"
	analysisTrace "github.com/kojah/gohawk/internal/trace"
	"golang.org/x/tools/go/analysis/analysistest"
)

type lockTraceEvent struct {
	Phase     string            `json:"phase"`
	Reason    string            `json:"reason"`
	Outcome   string            `json:"outcome"`
	Candidate string            `json:"candidate"`
	Position  string            `json:"position"`
	Details   map[string]string `json:"details"`
}

func TestLockTraceBoundaries(t *testing.T) {
	flags := flag.NewFlagSet("cycle-trace", flag.ContinueOnError)
	analysisTrace.RegisterFlags(flags)
	set := func(name, value string) {
		t.Helper()
		if err := flags.Set(name, value); err != nil {
			t.Fatal(err)
		}
	}
	path := filepath.Join(t.TempDir(), "trace.jsonl")
	t.Cleanup(func() {
		set("gohawk-trace", "none")
		set("gohawk-trace-candidate", "")
		set("gohawk-trace-file", os.DevNull)
	})
	set("gohawk-trace", "lockorder")
	set("gohawk-trace-candidate", "")
	set("gohawk-trace-file", path)
	analyzertest.Run(t, analysistest.TestData(), Analyzer(), "lockorder")
	analyzertest.Run(t, analysistest.TestData(), Analyzer(), "ordercycles")
	analyzertest.Run(t, analysistest.TestData(), Analyzer(), "privateread")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	checkDecisionTrace(t, data, "private-write-storage", "owners.go:", "accepted")
	checkLongerCycleTrace(t, data)
	checkConstantTrace(t, data, "predecessor-constant-branch-infeasible", "computed_guard.go:")
	checkConstantTrace(t, data, "carried-constant-branch-infeasible", "carried_release_state.go:")
	checkConstantTrace(t, data, "stable-parameter-branch-infeasible", "compound_parameter_guard.go:")
	checkDecisionTrace(t, data, "imported-writer-guard-unknown", "opaque_writer.go:", "unknown")
	checkDecisionTrace(t, data, "conditional-caller-release-proven", "caller_release.go:", "accepted")
	checkDecisionTrace(t, data, "lock-state-budget-exhausted", "state_budget.go:", "unknown")
	checkDecisionTrace(t, data, "fresh-field-identity-unknown", "escaped_fresh_field.go:", "unknown")
	checkDecisionTrace(t, data, "cross-owner-class-unknown", "cross_owner_orders.go:", "unknown")
	checkDecisionTrace(t, data, "loaded-acquisition-guard-unknown", "loaded_getter_guards.go:", "unknown")
	checkDecisionTrace(t, data, "held-for-caller-proven", "return_handoff.go:", "accepted")
	checkDecisionTrace(t, data, "unreleased-return", "private_mutex.go:", "rejected")
	checkDecisionTrace(t, data, "private-mutex-only", "private_mutex.go:", "accepted")
	checkDecisionTrace(t, data, "release-ownership-unknown", "lockorder.go:", "unknown")
	checkDecisionTrace(t, data, "read-lock-write", "read_locks.go:", "rejected")
	checkDecisionTrace(t, data, "exclusive-writer-guard-unknown", "read_locks.go:", "unknown")
	checkHelperReleaseTrace(t, data)
	checkInstructionEvidence(t, data, "deferred-release-proven", "captured_owner_handoff.go:", "accepted")
	checkMutexActionTrace(t, data)
	checkInstructionEvidence(t, data, "release-identity-unknown", "uncertain_unlock_identity.go:", "unknown")
	found := false
	foundUnknown := false
	for line := range strings.SplitSeq(strings.TrimSpace(string(data)), "\n") {
		var event lockTraceEvent
		if err := json.Unmarshal([]byte(line), &event); err != nil {
			t.Fatal(err)
		}
		if event.Reason == "loaded-acquisition-guard-unknown" && strings.Contains(event.Candidate, "optional_owners.go:") {
			foundUnknown = true
			if event.Phase != "decision" || event.Outcome != "unknown" || event.Position != event.Candidate {
				t.Errorf("invalid optional mutex uncertainty: %+v", event)
			}
			continue
		}
		if event.Reason != "opposite-order-recorded" || !strings.Contains(event.Candidate, "lock_classes.go:") {
			continue
		}
		found = true
		if event.Phase != "evidence" || event.Outcome != "rejected" ||
			event.Position == "" || event.Position == event.Candidate ||
			event.Details["held"] == "" || event.Details["acquired"] == "" {
			t.Errorf("invalid cycle evidence: %+v", event)
		}
	}
	if !found {
		t.Error("missing opposite-order evidence")
	}
	if !foundUnknown {
		t.Error("missing optional mutex uncertainty")
	}
}

func checkMutexActionTrace(t *testing.T, data []byte) {
	t.Helper()
	operations := make(map[string]bool)
	for line := range strings.SplitSeq(strings.TrimSpace(string(data)), "\n") {
		var event lockTraceEvent
		if err := json.Unmarshal([]byte(line), &event); err != nil {
			t.Fatal(err)
		}
		if event.Reason != "mutex-action-observed" || !strings.Contains(event.Candidate, "captured_owner_handoff.go:") {
			continue
		}
		if event.Phase != "evidence" || event.Outcome != "observed" || event.Position == "" ||
			event.Details["lock"] == "" || event.Details["instruction"] == "" || event.Details["deferred"] != "false" {
			t.Fatalf("invalid mutex action evidence: %+v", event)
		}
		operation := event.Details["operation"]
		if operation != "acquire" && operation != "release" || operations[operation] {
			t.Fatalf("invalid or repeated mutex action: %+v", event)
		}
		operations[operation] = true
	}
	if !operations["acquire"] || !operations["release"] {
		t.Fatalf("missing mutex actions on reported return: %v", operations)
	}
}

func checkInstructionEvidence(t *testing.T, data []byte, reason, file, outcome string) {
	t.Helper()
	for line := range strings.SplitSeq(strings.TrimSpace(string(data)), "\n") {
		var event lockTraceEvent
		if err := json.Unmarshal([]byte(line), &event); err != nil {
			t.Fatal(err)
		}
		if event.Reason != reason || !strings.Contains(event.Candidate, file) {
			continue
		}
		if event.Phase != "evidence" || event.Outcome != outcome || event.Position == "" || event.Position == event.Candidate {
			t.Fatalf("invalid instruction evidence: %+v", event)
		}
		return
	}
	t.Fatalf("missing instruction evidence: %s", reason)
}

func checkDecisionTrace(t *testing.T, data []byte, reason, file, outcome string) {
	t.Helper()
	found := false
	for line := range strings.SplitSeq(strings.TrimSpace(string(data)), "\n") {
		var event lockTraceEvent
		if err := json.Unmarshal([]byte(line), &event); err != nil {
			t.Fatal(err)
		}
		if event.Reason != reason || !strings.Contains(event.Candidate, file) {
			continue
		}
		found = true
		if event.Phase != "decision" || event.Outcome != outcome || event.Position != event.Candidate {
			t.Errorf("invalid decision evidence: %+v", event)
		}
	}
	if !found {
		t.Errorf("missing decision evidence: %s", reason)
	}
}

func checkConstantTrace(t *testing.T, data []byte, reason, file string) {
	t.Helper()
	found := false
	for line := range strings.SplitSeq(strings.TrimSpace(string(data)), "\n") {
		var event lockTraceEvent
		if err := json.Unmarshal([]byte(line), &event); err != nil {
			t.Fatal(err)
		}
		if event.Reason != reason || !strings.Contains(event.Candidate, file) {
			continue
		}
		found = true
		if event.Phase != "evidence" || event.Outcome != "accepted" || event.Position != event.Candidate {
			t.Errorf("invalid constant feasibility evidence: %+v", event)
		}
	}
	if !found {
		t.Errorf("missing constant feasibility evidence: %s", reason)
	}
}

func checkLongerCycleTrace(t *testing.T, data []byte) {
	t.Helper()
	found := false
	for line := range strings.SplitSeq(strings.TrimSpace(string(data)), "\n") {
		var event lockTraceEvent
		if err := json.Unmarshal([]byte(line), &event); err != nil {
			t.Fatal(err)
		}
		if event.Reason != "cycle-order-recorded" || !strings.Contains(event.Candidate, "ordercycles/cycles.go:") {
			continue
		}
		found = true
		if event.Phase != "evidence" || event.Outcome != "rejected" || event.Position == "" ||
			event.Details["held-mode"] == "" || event.Details["acquired-mode"] == "" {
			t.Errorf("invalid longer-cycle evidence: %+v", event)
		}
	}
	if !found {
		t.Error("missing longer-cycle evidence")
	}
}

// A reported missing release replays the helper that may have released the
// lock, bound to the reported return.
func checkHelperReleaseTrace(t *testing.T, data []byte) {
	t.Helper()
	for line := range strings.SplitSeq(strings.TrimSpace(string(data)), "\n") {
		var event lockTraceEvent
		if err := json.Unmarshal([]byte(line), &event); err != nil {
			t.Fatal(err)
		}
		if event.Reason != "helper-release-unproven" || !strings.Contains(event.Candidate, "captured_receiver_release.go:") {
			continue
		}
		if event.Phase != "evidence" || event.Outcome != "rejected" || !strings.Contains(event.Details["instruction"], "maybeUnsubscribeLocked") ||
			event.Details["source"] == "" || event.Details["completion"] == "" {
			t.Fatalf("unexpected helper-release evidence: %+v", event)
		}
		return
	}
	t.Fatal("missing helper-release-unproven evidence for captured_receiver_release.go")
}
