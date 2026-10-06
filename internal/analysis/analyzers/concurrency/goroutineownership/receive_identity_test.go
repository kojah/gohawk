package goroutineownership

import (
	"encoding/json"
	"os"
	"strings"
	"testing"

	"github.com/kojah/gohawk/internal/testsupport/analyzertest"
	"golang.org/x/tools/go/analysis/analysistest"
)

func TestReceiveIdentityStrength(t *testing.T) {
	assertSpawnProofs(t, map[string]GoroutineOutcome{
		"directMixed":    GoroutineUnknown,
		"helperMixed":    GoroutineUnknown,
		"nestedMixed":    GoroutineUnknown,
		"siblingField":   GoroutineUnknown,
		"helperReplaced": GoroutineUnknown,
		"groupMixed":     GoroutineUnknown,
		"selectedMixed":  GoroutineUnknown,
		"selectedEdge":   GoroutineUnknown,
		"mixedDefault":   GoroutineLifecycleViolated,
		"exact":          GoroutineLifecycleHonored,
		"exactHelper":    GoroutineLifecycleHonored,
		"unrelated":      GoroutineLifecycleViolated,
	}, "receiveidentity")
}

func TestReceiveIdentityTrace(t *testing.T) {
	path := enableSummaryJoinTrace(t)
	analyzertest.Run(t, analysistest.TestData(), Analyzer(), "receiveidentity")
	assertReceiveIdentityTrace(t, path)
}

func assertReceiveIdentityTrace(t *testing.T, path string) {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	want := map[string][2]string{
		"receiveidentity.directMixed": {"possible-signal-receive", "opaque-use"},
		"receiveidentity.helperMixed": {"helper-use", "opaque-use"},
		"receiveidentity.groupMixed":  {"helper-use", "opaque-use"},
		"receiveidentity.exact":       {"signal-received", "join"},
	}
	counts := map[string]int{}
	edge := false
	for line := range strings.SplitSeq(strings.TrimSpace(string(data)), "\n") {
		var event followupTraceEvent
		if err := json.Unmarshal([]byte(line), &event); err != nil {
			t.Fatal(err)
		}
		if event.Function == "receiveidentity.selectedEdge" && event.Reason == "selected-possible-receive-edge" {
			edge = true
			assertEdgeEvent(t, event, "unknown")
		}
		if expected, ok := want[event.Function]; ok && event.Phase == "label" {
			counts[event.Function]++
			if event.Reason != expected[0] || event.Details["label"] != expected[1] || event.Candidate == "" {
				t.Errorf("invalid receive identity label: %+v", event)
			}
		}
	}
	for name := range want {
		if counts[name] != 1 {
			t.Errorf("%s: got %d labels, want one", name, counts[name])
		}
	}
	if !edge {
		t.Error("missing unknown selected receive edge")
	}
}
