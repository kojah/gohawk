package goroutineownership

import (
	"encoding/json"
	"os"
	"strings"
	"testing"
)

type followupTraceEvent struct {
	Function  string            `json:"function"`
	Reason    string            `json:"reason"`
	Phase     string            `json:"phase"`
	Outcome   string            `json:"outcome"`
	Candidate string            `json:"candidate"`
	Position  string            `json:"position"`
	Details   map[string]string `json:"details"`
}

func assertEdgeEvent(t *testing.T, event followupTraceEvent, outcome string) {
	t.Helper()
	if event.Phase != "evidence" || event.Outcome != outcome || event.Candidate == "" ||
		event.Details["from_block"] == "" || event.Details["to_block"] == "" {
		t.Errorf("invalid selected edge: %+v", event)
	}
}

func assertFollowupBoundaryTrace(t *testing.T, path string) {
	t.Helper()
	want := map[string][2]string{
		"opaqueOutputNeedsJoin":                                                 {"unowned-return", "rejected"},
		"receiveOnlyOpaqueInput":                                                {"opaque-ownership-transfer", "unknown"},
		"relayQueueParticipant":                                                 {"relay-dependency-lifecycle", "unknown"},
		"relayCanceledParticipant":                                              {"relay-dependency-lifecycle", "unknown"},
		"relayExtraWorkIsNotCovered":                                            {"unowned-return", "rejected"},
		"observedRetainedContext":                                               {"opaque-ownership-transfer", "unknown"},
		"ignoredContextCannotSettle":                                            {"unowned-return", "rejected"},
		"emptyReceiveArmSharedReturn":                                           {"join-proven", "accepted"},
		"selectTimeoutDoesNotJoin":                                              {"unowned-return", "rejected"},
		"helperSelectTimeoutDoesNotJoin":                                        {"unowned-return", "rejected"},
		"returnedCleanupBoundsReader":                                           {"opaque-ownership-transfer", "unknown"},
		"unrelatedReturnedCleanupDoesNotSettle":                                 {"unowned-return", "rejected"},
		"joinedBeforeDeferredCompletion":                                        {"join-proven", "accepted"},
		"joinedBeforeDeferredClose":                                             {"join-proven", "accepted"},
		"deferredWorkStillNeedsCompletion":                                      {"unowned-return", "rejected"},
		"consumesFactoryCompanion":                                              {"opaque-ownership-transfer", "unknown"},
		"helperSettlesStoredSignal":                                             {"opaque-ownership-transfer", "unknown"},
		"aggregateInspectionDoesNotSettle":                                      {"unowned-return", "rejected"},
		"canceledContextPassedToOpaqueWorker":                                   {"locally-canceled-context", "unknown"},
		"opaqueWorkerWithConditionalCancellation":                               {"unowned-return", "rejected"},
		"assertedOwnerHandoff":                                                  {"opaque-ownership-transfer", "unknown"},
		"assertedOwnerNotHandedOff":                                             {"unowned-return", "rejected"},
		"bufferedResultDoesNotReplaceDeferredCompletion":                        {"unowned-return", "rejected"},
		"bufferedReaderClosedByCaller":                                          {"opaque-ownership-transfer", "unknown"},
		"nestedCapturedConnectionCleanup$1":                                     {"opaque-ownership-transfer", "unknown"},
		"ignoredWrapperArgumentDoesNotSettle":                                   {"unowned-return", "rejected"},
		"transportClosedBeforeLaunch":                                           {"unowned-return", "rejected"},
		"transportReleaseDoesNotSettleErrorSend":                                {"unowned-return", "rejected"},
		"pipePeerConsumedAfterLaunch":                                           {"opaque-ownership-transfer", "unknown"},
		"ignoredPeerDoesNotSettle":                                              {"unowned-return", "rejected"},
		"(*goroutineownership.segmentDownloader).startBoundedByReceiverContext": {"receiver-context-lifecycle", "unknown"},
		"(*goroutineownership.segmentDownloader).startWithContextInstalledHere": {"unowned-return", "rejected"},
		"pipePeerDoesNotSettleErrorSend":                                        {"unowned-return", "rejected"},
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	selectedEdge := false
	contextEdge := false
	for line := range strings.SplitSeq(strings.TrimSpace(string(data)), "\n") {
		var event followupTraceEvent
		if err := json.Unmarshal([]byte(line), &event); err != nil {
			t.Fatal(err)
		}
		name := strings.TrimPrefix(event.Function, "goroutineownership.")
		if name == "observedRetainedContext" && event.Reason == "selected-context-edge" {
			contextEdge = true
			assertEdgeEvent(t, event, "unknown")
		}
		if name == "emptyReceiveArmSharedReturn" && event.Reason == "selected-receive-edge" {
			selectedEdge = true
			assertEdgeEvent(t, event, "accepted")
		}
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
	if !selectedEdge {
		t.Error("missing selected receive edge evidence")
	}
	if !contextEdge {
		t.Error("missing selected context edge evidence")
	}
}
