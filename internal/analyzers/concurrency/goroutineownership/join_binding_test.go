package goroutineownership

import (
	"encoding/json"
	"os"
	"strings"
	"testing"

	"github.com/kojah/gohawk/internal/analyzertest"
	"golang.org/x/tools/go/analysis/analysistest"
)

func TestJoinBindingStrength(t *testing.T) {
	want := map[string]GoroutineOutcome{
		"exactHelper":         GoroutineLifecycleHonored,
		"mixedHelper":         GoroutineUnknown,
		"replacedHelperField": GoroutineUnknown,
		"exactWait":           GoroutineLifecycleHonored,
		"mixedWait":           GoroutineUnknown,
		"unrelatedHelper":     GoroutineLifecycleViolated,
		"mixedOwner":          GoroutineUnknown,
	}
	assertSpawnProofs(t, want, "joinbindings")
}

func TestJoinReceiverTrace(t *testing.T) {
	path := enableSummaryJoinTrace(t)
	analyzertest.Run(t, analysistest.TestData(), Analyzer(), "joinbindings")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	want := map[string][3]string{
		"mixedWait":  {"possible-join-receiver", "unknown", "opaque-use"},
		"mixedOwner": {"possible-join-receiver", "unknown", "opaque-use"},
		"exactWait":  {"direct-join", "accepted", "join"},
	}
	counts := map[string]int{}
	for line := range strings.SplitSeq(strings.TrimSpace(string(data)), "\n") {
		var event followupTraceEvent
		if err := json.Unmarshal([]byte(line), &event); err != nil {
			t.Fatal(err)
		}
		name := strings.TrimPrefix(event.Function, "joinbindings.")
		expected, ok := want[name]
		if !ok || event.Phase != "label" {
			continue
		}
		counts[name]++
		if event.Reason != expected[0] || event.Outcome != expected[1] || event.Details["label"] != expected[2] ||
			event.Candidate == "" || event.Position == "" {
			t.Errorf("invalid join receiver label: %+v", event)
		}
	}
	for name := range want {
		if counts[name] != 1 {
			t.Errorf("%s: got %d labels, want one per receiver", name, counts[name])
		}
	}
}
