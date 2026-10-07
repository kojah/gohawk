package goroutineownership

import (
	"bytes"
	"encoding/json"
	"os"
	"strings"
	"testing"

	"github.com/kojah/gohawk/internal/engine/ssaflow"
	"github.com/kojah/gohawk/internal/testsupport/analyzertest"
	"golang.org/x/tools/go/analysis/analysistest"
	"golang.org/x/tools/go/ssa"
)

// assertSpawnProofs checks the authoritative proof as well as the fixture's
// diagnostics. Each case names the first launch: later waiter launches are
// separate obligations, not joins by the launching goroutine itself.
func assertSpawnProofs(t *testing.T, want map[string]GoroutineOutcome, patterns ...string) {
	t.Helper()
	for _, result := range analyzertest.Run(t, analysistest.TestData(), Analyzer(), patterns...) {
		functions, err := ssaflow.SourceSSAFunctions(result.Pass)
		if err != nil {
			t.Fatal(err)
		}
		for _, function := range functions {
			expected, ok := want[function.Name()]
			if !ok {
				continue
			}
			for _, spawn := range ssaflow.InstructionsOf[*ssa.Go](function) {
				analysis := newSpawnAnalysis(result.Pass, function, spawn)
				if proof := analysis.prove(); proof.Outcome != expected {
					t.Errorf("%s: got %+v, want %v", function.Name(), proof, expected)
				}
				delete(want, function.Name())
				break
			}
		}
	}
	if len(want) != 0 {
		t.Errorf("missing spawn proof cases: %v", want)
	}
}

// assertClassifierLabels checks a normal analyzer run, where each selected
// instruction must be labelled once with its reason and proof strength.
func assertClassifierLabels(t *testing.T, path string, want map[string][3]string) {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	counts := map[string]int{}
	for line := range strings.SplitSeq(strings.TrimSpace(string(data)), "\n") {
		var event followupTraceEvent
		if err := json.Unmarshal([]byte(line), &event); err != nil {
			t.Fatal(err)
		}
		expected, ok := want[event.Function]
		if !ok || event.Phase != "label" {
			continue
		}
		counts[event.Function]++
		if event.Reason != expected[0] || event.Outcome != expected[1] || event.Details["label"] != expected[2] ||
			event.Candidate == "" || event.Position == "" {
			t.Errorf("invalid classifier label: %+v", event)
		}
	}
	for name := range want {
		if counts[name] != 1 {
			t.Errorf("%s: got %d labels, want one", name, counts[name])
		}
	}
}

func TestAsyncObserverStrength(t *testing.T) {
	assertSpawnProofs(t, map[string]GoroutineOutcome{
		"directAsyncWait":    GoroutineUnknown,
		"helperAsyncWait":    GoroutineUnknown,
		"directWait":         GoroutineLifecycleHonored,
		"deferredWait":       GoroutineLifecycleHonored,
		"helperDeferredWait": GoroutineLifecycleHonored,
		"launchedHelper":     GoroutineUnknown,
		"unrelatedObserver":  GoroutineLifecycleViolated,
	}, "asyncobservers")
}

func TestAsyncObserverTrace(t *testing.T) {
	path := enableSummaryJoinTrace(t)
	analyzertest.Run(t, analysistest.TestData(), Analyzer(), "asyncobservers")
	want := map[string][3]string{
		"asyncobservers.directAsyncWait": {"launched-helper", "unknown", "opaque-use"},
		"asyncobservers.helperAsyncWait": {"helper-use", "unknown", "opaque-use"},
		"asyncobservers.launchedHelper":  {"launched-helper", "unknown", "opaque-use"},
		"asyncobservers.directWait":      {"direct-join", "accepted", "join"},
		"asyncobservers.deferredWait":    {"direct-join", "accepted", "join"},
	}
	assertClassifierLabels(t, path, want)
}

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

func TestCompletionBindingStrength(t *testing.T) {
	assertSpawnProofs(t, map[string]GoroutineOutcome{
		"mixedSignal":          GoroutineUnknown,
		"mixedGroup":           GoroutineUnknown,
		"replacedCapture":      GoroutineUnknown,
		"captureSnapshot":      GoroutineLifecycleHonored,
		"exactSignalMissing":   GoroutineLifecycleViolated,
		"exactSignalJoined":    GoroutineLifecycleHonored,
		"exactCaptureMissing":  GoroutineLifecycleViolated,
		"nestedCaptureMissing": GoroutineLifecycleViolated,
		"exactGroupMissing":    GoroutineLifecycleViolated,
	}, "completionbindings")
}

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
	analyzertest.Run(t, analysistest.TestData(), Analyzer(), "joinbindings", "ownerparticipation")
	want := map[string][3]string{
		"joinbindings.mixedWait":           {"possible-join-receiver", "unknown", "opaque-use"},
		"joinbindings.mixedOwner":          {"owner-lifecycle-participation", "unknown", "opaque-use"},
		"joinbindings.exactWait":           {"direct-join", "accepted", "join"},
		"ownerparticipation.directClose":   {"owner-lifecycle-participation", "unknown", "opaque-use"},
		"ownerparticipation.deferredClose": {"owner-lifecycle-participation", "unknown", "opaque-use"},
		"ownerparticipation.helperClose":   {"helper-use", "unknown", "opaque-use"},
	}
	assertClassifierLabels(t, path, want)
}

func TestNotificationPromiseStrength(t *testing.T) {
	assertSpawnProofs(t, map[string]GoroutineOutcome{
		"optionalClose":        GoroutineUnknown,
		"optionalSend":         GoroutineUnknown,
		"optionalRegistration": GoroutineUnknown,
		"nestedProgress":       GoroutineUnknown,
		"outerProgress":        GoroutineUnknown,
		"detachedClose":        GoroutineUnknown,
		"alternateMissing":     GoroutineLifecycleViolated,
		"deferredMissing":      GoroutineLifecycleViolated,
		"synchronousMissing":   GoroutineLifecycleViolated,
		"deferredJoined":       GoroutineLifecycleHonored,
		"synchronousJoined":    GoroutineLifecycleHonored,
	}, "notificationpromises")
}

func TestOwnerParticipationStrength(t *testing.T) {
	want := map[string]GoroutineOutcome{
		"directClose":    GoroutineUnknown,
		"deferredClose":  GoroutineUnknown,
		"helperClose":    GoroutineUnknown,
		"directStop":     GoroutineUnknown,
		"directShutdown": GoroutineUnknown,
		"directWait":     GoroutineUnknown,
		"directKill":     GoroutineUnknown,
		"closeAndJoin":   GoroutineLifecycleHonored,
		"closeOther":     GoroutineLifecycleViolated,
	}
	assertSpawnProofs(t, want, "ownerparticipation")
}

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

func TestRecursiveHelperStrength(t *testing.T) {
	assertSpawnProofs(t, map[string]GoroutineOutcome{
		"recursiveObserved":   GoroutineUnknown,
		"mutualObserved":      GoroutineUnknown,
		"exactAfterRecursive": GoroutineLifecycleHonored,
		"unrelatedRecursive":  GoroutineLifecycleViolated,
	}, "recursivehelpers")
}

func TestRecursiveHelperTrace(t *testing.T) {
	path := enableSummaryJoinTrace(t)
	analyzertest.Run(t, analysistest.TestData(), Analyzer(), "recursivehelpers")
	assertClassifierLabels(t, path, map[string][3]string{
		"recursivehelpers.recursiveObserved":   {"helper-use", "unknown", "opaque-use"},
		"recursivehelpers.mutualObserved":      {"helper-use", "unknown", "opaque-use"},
		"recursivehelpers.exactAfterRecursive": {"helper-use", "accepted", "join"},
	})
}

func TestRelayBindingStrength(t *testing.T) {
	assertSpawnProofs(t, map[string]GoroutineOutcome{
		"reassignedGroup":     GoroutineLifecycleHonored,
		"exactRelay":          GoroutineLifecycleHonored,
		"unrelatedWait":       GoroutineLifecycleViolated,
		"relayWithCallerWork": GoroutineUnknown,
		"relayWithWork":       GoroutineLifecycleViolated,
	}, "relaybindings")
}

// These intentionally remain accepted by the precision-first reporter. The
// proof must still distinguish a possible handoff from exact ownership.
func TestTransferProofStrength(t *testing.T) {
	want := map[string]GoroutineOutcome{
		"mixedReturn":               GoroutineUnknown,
		"overwrittenAggregate":      GoroutineUnknown,
		"discardedWrapper":          GoroutineUnknown,
		"exactStore":                GoroutineLifecycleHonored,
		"mixedStore":                GoroutineUnknown,
		"overwrittenAggregateStore": GoroutineUnknown,
		"exactBesideOpaque":         GoroutineLifecycleHonored,
		"configuredMockResult":      GoroutineUnknown,
	}
	assertSpawnProofs(t, want, "transferlabels")
}
