package goroutineownership

import (
	"bytes"
	"encoding/json"
	"os"
	"testing"

	"github.com/kojah/gohawk/internal/analyzertest"
	"golang.org/x/tools/go/analysis/analysistest"
)

func TestCallerBoundStrength(t *testing.T) {
	want := map[string]GoroutineOutcome{
		"callerStop":          GoroutineUnknown,
		"callerContext":       GoroutineUnknown,
		"callerStopHelper":    GoroutineUnknown,
		"capturedStopHelper":  GoroutineLifecycleViolated,
		"callerContextHelper": GoroutineUnknown,
		"callerCompletion":    GoroutineTransferred,
		"callerGroup":         GoroutineTransferred,
		"exactJoin":           GoroutineLifecycleHonored,
		"ignoredContext":      GoroutineLifecycleViolated,
		"localStop":           GoroutineLifecycleViolated,
	}
	assertSpawnProofs(t, want, "callerbounds")
}

func TestCallerBoundTrace(t *testing.T) {
	path := enableSummaryJoinTrace(t)
	analyzertest.Run(t, analysistest.TestData(), Analyzer(), "callerbounds")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	want := map[string][2]string{
		"callerbounds.callerStop":          {"stop-lifecycle", "unknown"},
		"callerbounds.callerContext":       {"context-lifecycle", "unknown"},
		"callerbounds.callerStopHelper":    {"stop-lifecycle", "unknown"},
		"callerbounds.callerContextHelper": {"context-lifecycle", "unknown"},
		"callerbounds.callerCompletion":    {"caller-or-external-owner", "accepted"},
		"callerbounds.callerGroup":         {"caller-or-external-owner", "accepted"},
		"callerbounds.exactJoin":           {"join-proven", "accepted"},
		"callerbounds.capturedStopHelper":  {"unowned-return", "rejected"},
	}
	counts := map[string]int{}
	for line := range bytes.SplitSeq(bytes.TrimSpace(data), []byte("\n")) {
		var event followupTraceEvent
		if err := json.Unmarshal(line, &event); err != nil {
			t.Fatal(err)
		}
		expected, ok := want[event.Function]
		if !ok || event.Phase != "decision" {
			continue
		}
		counts[event.Function]++
		if event.Reason != expected[0] || event.Outcome != expected[1] || event.Candidate == "" || event.Candidate != event.Position {
			t.Errorf("invalid caller-bound decision: %+v", event)
		}
	}
	for name := range want {
		if counts[name] != 1 {
			t.Errorf("%s: got %d decisions, want one", name, counts[name])
		}
	}
}
