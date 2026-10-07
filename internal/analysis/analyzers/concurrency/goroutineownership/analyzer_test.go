package goroutineownership

import (
	"encoding/json"
	"os"
	"strings"
	"testing"

	"github.com/kojah/gohawk/internal/testsupport/analyzertest"
	"golang.org/x/tools/go/analysis/analysistest"
)

func TestAnalyzer(t *testing.T) {
	tracePath := enableSummaryJoinTrace(t)
	analyzertest.Run(t, analysistest.TestData(), Analyzer(), "goroutineownership", "summaryjoins", "processexit")
	assertFollowupBoundaryTrace(t, tracePath)
	assertLabelTrace(t, tracePath)
}

func TestGoroutineOwnershipReasonCodes(t *testing.T) {
	want := map[goroutineOwnershipReason]string{
		reasonSummaryJoinBudgetExhausted:     "summary-join-budget-exhausted",
		reasonFactoryOriginBudgetExhausted:   "factory-origin-budget-exhausted",
		reasonHelperCallBudgetExhausted:      "helper-call-budget-exhausted",
		reasonPreSpawnCensusCutoff:           "pre-spawn-census-budget-exhausted",
		reasonRelayDependencyBudgetExhausted: "relay-dependency-budget-exhausted",
		reasonRetainedOwnerBudgetExhausted:   "retained-owner-budget-exhausted",
		reasonDiscoveryBudgetExhausted:       "completion-discovery-budget-exhausted",
		reasonReceiveBudgetExhausted:         "worker-receive-budget-exhausted",
		reasonNone:                           "",
		reasonJoinProven:                     "join-proven",
		reasonDeferredJoinBeforeSpawn:        "deferred-join-before-spawn",
		reasonGuardedLocalJoin:               "guarded-local-join",
		reasonStopLifecycle:                  "stop-lifecycle",
		reasonContextLifecycle:               "context-lifecycle",
		reasonLocallyCanceledContext:         "locally-canceled-context",
		reasonReceiverContext:                "receiver-context-lifecycle",
		reasonRelayDependency:                "relay-dependency-lifecycle",
		reasonSynctestBubbleOwner:            "synctest-bubble-owner",
		reasonCallerOrExternalOwner:          "caller-or-external-owner",
		reasonOwnershipTransfer:              "ownership-transfer",
		reasonOpaqueTransfer:                 "opaque-ownership-transfer",
		reasonLoopJoinUnproven:               "loop-join-unproven",
		reasonWorkerConsumesSignal:           "signal-consumed-by-worker",
		reasonFlagGuardedJoin:                "flag-guarded-join",
		reasonBufferedSignal:                 "buffered-completion-signal",
		reasonSignalCensusUnavailable:        "signal-census-unavailable",
		reasonUnobservedSignal:               "signal-never-observed",
		reasonSharedStorageSignal:            "shared-storage-signal",
		reasonNoObligation:                   "no-completion-obligation",
		reasonUnownedReturn:                  "unowned-return",
		reasonDoneBeforeCompletion:           "waitgroup-done-before-completion",
		reasonSelectedReceiveEdge:            "selected-receive-edge",
		reasonSelectedContextEdge:            "selected-context-edge",
		reasonCountedDrainEdge:               "counted-drain-edge",
		reasonProcessExitStopsWorker:         "process-exit-stops-worker",
		reasonLabelSignalReceived:            "signal-received",
		reasonLabelDirectJoin:                "direct-join",
		reasonLabelSummaryJoin:               "summary-join",
		reasonLabelHelper:                    "helper-use",
		reasonLabelPipePeer:                  "pipe-peer",
		reasonLabelTestingCleanup:            "testing-cleanup",
		reasonLabelStoredOutside:             "stored-outside-function",
		reasonLabelGoMockReturn:              "gomock-return",
		reasonLabelStoredTestingReceiver:     "stored-testing-receiver",
		reasonLabelSignalAggregateReceive:    "receive-from-signal-aggregate",
		reasonLabelSelectSends:               "select-sends-tracked",
		reasonLabelSent:                      "sent-to-channel",
		reasonLabelStoredInMap:               "stored-in-map",
		reasonLabelAppended:                  "appended",
		reasonLabelClosesRetainedOwner:       "closes-retained-owner",
		reasonLabelDynamicCallee:             "dynamic-callee",
		reasonLabelCalleeWithoutBody:         "callee-without-body",
		reasonLabelLaunchedHelper:            "launched-helper",
		reasonLabelReturnedTracked:           "returned-tracked-value",
		reasonLabelReturnedProjection:        "returned-signal-projection",
		reasonLabelReturnedContainment:       "returned-possible-owner",
		reasonLabelPossibleJoin:              "possible-join-receiver",
		reasonLabelOwnerLifecycle:            "owner-lifecycle-participation",
		reasonLabelPossibleSignalReceive:     "possible-signal-receive",
		reasonSelectedPossibleReceiveEdge:    "selected-possible-receive-edge",
		reasonCountedPossibleDrainEdge:       "counted-possible-drain-edge",
	}
	if len(want) != int(goroutineOwnershipReasonCount) {
		t.Fatal("every reason needs a boundary spelling assertion")
	}
	for reason := range goroutineOwnershipReasonCount {
		code, ok := want[reason]
		if !ok || reason.String() != code {
			t.Errorf("reason %d: got %q, want %q", reason, reason.String(), code)
		}
	}
	for _, reason := range []goroutineOwnershipReason{goroutineOwnershipReasonCount, 255} {
		if reason.String() != "invalid-goroutine-ownership-reason" {
			t.Errorf("invalid reason %d: %q", reason, reason.String())
		}
	}
}

func TestSummaryJoinReasonCodes(t *testing.T) {
	want := map[summaryJoinReason]string{
		summaryJoinNone: "",
		summaryJoinConcurrencyJoinBudgetExhausted: "concurrency-join-budget-exhausted",
		summaryJoinConcurrencyJoinNotApplicable:   "concurrency-join-not-applicable",
		summaryJoinConcurrencyJoinNotSynchronous:  "concurrency-join-not-synchronous",
		summaryJoinConcurrencySummaryJoin:         "concurrency-summary-join",
		summaryJoinConcurrencySummaryNoExactJoin:  "concurrency-summary-no-exact-join",
	}
	if len(want) != int(summaryJoinReasonCount) {
		t.Fatal("every reason needs a boundary spelling assertion")
	}
	for reason := range summaryJoinReasonCount {
		code, ok := want[reason]
		if !ok || reason.String() != code {
			t.Errorf("reason %d: got %q, want %q", reason, reason.String(), code)
		}
	}
	for _, reason := range []summaryJoinReason{summaryJoinReasonCount, 255} {
		if reason.String() != "invalid-goroutineownership-reason" {
			t.Errorf("invalid reason %d: %q", reason, reason.String())
		}
	}
}

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
		"embeddedRegistryWorker":                                                {"opaque-ownership-transfer", "unknown"},
		"nestedRegistryWorker":                                                  {"opaque-ownership-transfer", "unknown"},
		"mixedRegistryWorker":                                                   {"opaque-ownership-transfer", "unknown"},
		"freshEmbeddedWorker":                                                   {"unowned-return", "rejected"},
		"visibleFreshEmbeddedWorker":                                            {"unowned-return", "rejected"},
		"unrelatedRegistryOwner":                                                {"unowned-return", "rejected"},
		"replacedRegistryGroup":                                                 {"unowned-return", "rejected"},
		"receiverBoundThroughSecondBinding":                                     {"receiver-context-lifecycle", "unknown"},
		"cleanupOpaqueWorkerField":                                              {"opaque-ownership-transfer", "unknown"},
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

// assertLabelTrace checks that the classifier's labels are traced as label
// steps named by the rule that decided them: a join as accepted, an opaque
// use as unknown with the boundary it met.
func assertLabelTrace(t *testing.T, path string) {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	want := map[string][2]string{
		"opaque_worker_fields.go:19:20": {"closes-retained-owner", "unknown"},
		"completion_tails.go:16:13":     {"direct-join", "accepted"},
		"cleanup_results.go:21:2":       {"closes-retained-owner", "unknown"},
		"factory_signals.go:19:2":       {"returned-signal-projection", "unknown"},
	}
	for line := range strings.SplitSeq(strings.TrimSpace(string(data)), "\n") {
		var event followupTraceEvent
		if err := json.Unmarshal([]byte(line), &event); err != nil {
			t.Fatal(err)
		}
		if event.Phase != "label" {
			continue
		}
		for position, expected := range want {
			if strings.HasSuffix(event.Position, position) && event.Reason == expected[0] && event.Outcome == expected[1] {
				if event.Candidate == "" {
					t.Errorf("label missing candidate association: %+v", event)
				}
				delete(want, position)
			}
		}
	}
	if len(want) != 0 {
		t.Errorf("missing label steps: %v", want)
	}
}

func TestReturnedOwnershipTrace(t *testing.T) {
	path := enableSummaryJoinTrace(t)
	analyzertest.Run(t, analysistest.TestData(), Analyzer(), "returnlabels")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	labels := 0
	decisions := map[string]int{}
	var labelCandidate, decisionCandidate string
	for line := range strings.SplitSeq(strings.TrimSpace(string(data)), "\n") {
		var event followupTraceEvent
		if err := json.Unmarshal([]byte(line), &event); err != nil {
			t.Fatal(err)
		}
		name := strings.TrimPrefix(event.Function, "returnlabels.")
		if event.Phase == "label" && strings.HasPrefix(event.Details["instruction"], "return ") {
			labels++
			labelCandidate = event.Candidate
			assertReturnedTransferLabel(t, event)
		}
		if event.Phase == "decision" {
			decisions[name]++
			wantReason, wantOutcome := "unowned-return", "rejected"
			if name == "mergedReturn" {
				wantReason, wantOutcome = "join-proven", "accepted"
				decisionCandidate = event.Candidate
			}
			if event.Reason != wantReason || event.Outcome != wantOutcome || event.Candidate != event.Position {
				t.Errorf("invalid returned ownership decision: %+v", event)
			}
		}
	}
	if labels != 1 || decisions["mergedReturn"] != 1 || decisions["unrelatedReturn"] != 1 {
		t.Errorf("got %d return labels and decisions %v, want one transfer label and one decision per candidate", labels, decisions)
	}
	if labelCandidate == "" || labelCandidate != decisionCandidate {
		t.Errorf("return label candidate %q differs from final decision candidate %q", labelCandidate, decisionCandidate)
	}
}

func assertReturnedTransferLabel(t *testing.T, event followupTraceEvent) {
	t.Helper()
	if event.Function != "returnlabels.mergedReturn" || event.Reason != "returned-tracked-value" || event.Outcome != "accepted" ||
		event.Details["label"] != "transfer" || event.Candidate == "" {
		t.Errorf("invalid returned ownership label: %+v", event)
	}
}
