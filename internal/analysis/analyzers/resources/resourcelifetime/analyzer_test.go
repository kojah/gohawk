package resourcelifetime

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/kojah/gohawk/internal/engine/ssaflow"
	"github.com/kojah/gohawk/internal/engine/ssaflow/ssaflowtest"
	analysisTrace "github.com/kojah/gohawk/internal/reporting/trace"
	"github.com/kojah/gohawk/internal/testsupport/analyzertest"
	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/analysis/analysistest"
	"golang.org/x/tools/go/ssa"
)

func TestAnalyzer(t *testing.T) {
	flags := flag.NewFlagSet("cleanup-trace", flag.ContinueOnError)
	analysisTrace.RegisterFlags(flags)
	path := filepath.Join(t.TempDir(), "trace.jsonl")
	// Trace flags are process-global and do not expose a reset API. This test
	// owns their configuration; clear the candidate, select no analyzer, and
	// close the temporary destination before its directory is removed.
	t.Cleanup(func() {
		for name, value := range map[string]string{
			"gohawk-trace": "none", "gohawk-trace-candidate": "", "gohawk-trace-file": os.DevNull,
		} {
			if err := flags.Set(name, value); err != nil {
				t.Error(err)
			}
		}
	})
	for name, value := range map[string]string{
		"gohawk-trace": "resourcelifetime", "gohawk-trace-candidate": "", "gohawk-trace-file": path,
	} {
		if err := flags.Set(name, value); err != nil {
			t.Fatal(err)
		}
	}
	// Reuse this run for diagnostic context and trace checks: repeating the
	// fixtures also repeats dependency loading and fact serialization checks.
	results := analyzertest.Run(t, analysistest.TestData(), Analyzer(),
		"resourcelifetime", "processexit", "processexitlib", "processexitrecursive", "privateentry")
	assertMissingReleaseEvidence(t, results)
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	foundReturnPath := false
	for line := range strings.SplitSeq(strings.TrimSpace(string(data)), "\n") {
		var event struct {
			Reason    string `json:"reason"`
			Phase     string `json:"phase"`
			Outcome   string `json:"outcome"`
			Candidate string `json:"candidate"`
			Function  string `json:"function"`
		}
		if err := json.Unmarshal([]byte(line), &event); err != nil {
			t.Fatal(err)
		}
		if event.Reason == "resource-return-path" && strings.Contains(event.Candidate, "sql_boundaries.go:") {
			foundReturnPath = true
			if event.Phase != "evidence" || event.Outcome != "observed" {
				t.Errorf("unexpected return-path evidence: %+v", event)
			}
		}
		if event.Reason != "captured-by-prior-cleanup" {
			continue
		}
		found = true
		if event.Phase != "label" || event.Outcome != "unknown" ||
			!strings.Contains(event.Candidate, "reassigned_cleanup.go:21:") || event.Function != "resourcelifetime.reassignedCleanup" {
			t.Errorf("unexpected prior-cleanup evidence: %+v", event)
		}
	}
	if !found {
		t.Error("missing prior-cleanup trace evidence")
	}
	if !foundReturnPath {
		t.Error("missing resource return-path evidence")
	}
	assertSQLBoundaryTrace(t, data)
	assertLabelTrace(t, data)
	assertFollowupBoundaryTrace(t, data)
}

func assertSQLBoundaryTrace(t *testing.T, data []byte) {
	t.Helper()
	// A closed parent statement makes the rows unknown, a classifier label;
	// a canceled context is the proof's decision.
	want := map[string][2]string{
		"statement-parent-closed":             {"label", "unknown"},
		"context-canceled-before-acquisition": {"decision", "accepted"},
	}
	for line := range strings.SplitSeq(strings.TrimSpace(string(data)), "\n") {
		var event struct {
			Reason    string `json:"reason"`
			Phase     string `json:"phase"`
			Outcome   string `json:"outcome"`
			Candidate string `json:"candidate"`
		}
		if err := json.Unmarshal([]byte(line), &event); err != nil {
			t.Fatal(err)
		}
		expected, ok := want[event.Reason]
		if !ok || !strings.Contains(event.Candidate, "sql_boundaries.go:") {
			continue
		}
		if event.Phase != expected[0] || event.Outcome != expected[1] {
			t.Errorf("unexpected SQL boundary trace: %+v", event)
		}
		delete(want, event.Reason)
	}
	if len(want) != 0 {
		t.Errorf("missing SQL boundary traces: %v", want)
	}
}

// boundedSearchDeadline bounds the analyzer action, not package loading or
// prerequisite fact inference. Those costs vary independently of the release
// search, especially under race instrumentation. The formerly unbounded search
// did not finish in sixty seconds; retain a much smaller action deadline.
const boundedSearchDeadline = 15 * time.Second

// TestRecursiveReleaseSearchStaysBounded fails if the release search is once
// again unbounded on mutually recursive callees. It is a cost test, so it
// asserts elapsed time rather than diagnostics; the fixture expects none.
func TestRecursiveReleaseSearchStaysBounded(t *testing.T) {
	start := time.Now()
	results := analyzertest.Run(t, analysistest.TestData(), Analyzer(), "recursivecleanup")
	if len(results) != 1 || results[0].Action == nil || results[0].Action.Err != nil {
		t.Fatal("expected one successful analyzer action for the recursive fixture")
	}
	// checker.Action.Duration starts after prerequisite actions finish. Timing
	// the whole harness falsely attributes dependency work to recursive search.
	elapsed := results[0].Action.Duration
	t.Logf("recursive fixture: analyzer=%s, complete harness=%s", elapsed, time.Since(start))
	if elapsed > boundedSearchDeadline {
		t.Errorf("analyzing mutually recursive callees took %s, want under %s; the release search is not bounded",
			elapsed, boundedSearchDeadline)
	}
}

// assertLabelTrace checks that a proven release is traced as a settled label
// on the instruction that settles it, and that the shared lifecycle evidence
// names the question each answer served.
func assertLabelTrace(t *testing.T, data []byte) {
	t.Helper()
	settled, questioned := false, false
	for line := range strings.SplitSeq(strings.TrimSpace(string(data)), "\n") {
		var event struct {
			Phase    string            `json:"phase"`
			Outcome  string            `json:"outcome"`
			Position string            `json:"position"`
			Details  map[string]string `json:"details"`
		}
		if err := json.Unmarshal([]byte(line), &event); err != nil {
			t.Fatal(err)
		}
		if event.Phase == "label" && strings.HasSuffix(event.Position, "argument_cases.go:107:2") {
			settled = event.Outcome == "accepted" && event.Details["label"] == "settled"
		}
		if event.Phase == "evidence" && event.Details["question"] == "release+summary" {
			questioned = true
		}
	}
	if !settled {
		t.Error("missing settled label for the deferred constant-argument release")
	}
	if !questioned {
		t.Error("lifecycle evidence does not name the question it answered")
	}
}

// resourceReasonSpellings is every reason's code as a trace prints it.
var resourceReasonSpellings = map[resourceLifetimeReason]string{
	resourceReasonCallResultMayTransfer:                    "call-result-may-transfer",
	resourceReasonDirectMayCarry:                           "direct-value-may-carry-resource",
	resourceReasonAggregateMayCarry:                        "aggregate-may-carry-resource",
	resourceReasonWrapperMayCarry:                          "wrapper-may-carry-resource",
	resourceReasonMemoryWriter:                             "memory-writer-no-external-resource",
	resourceReasonAcquisitionLocationUnknown:               "acquisition-location-unknown",
	resourceReasonPriorDeferMayRelease:                     "prior-defer-may-release",
	resourceReasonNone:                                     "",
	resourceReasonAcquisitionErrorProven:                   "acquisition-error-proven",
	resourceReasonAggregateOwnerMayEscape:                  "aggregate-owner-may-escape",
	resourceReasonAmbiguousCleanupValue:                    "ambiguous-cleanup-value",
	resourceReasonAmbiguousHelperCleanupValue:              "ambiguous-helper-cleanup-value",
	resourceReasonAppended:                                 "appended",
	resourceReasonAppendedToLocalCollection:                "appended-to-local-collection",
	resourceReasonCollectionReturned:                       "collection-returned",
	resourceReasonCollectionReleasedByHelper:               "collection-released-by-helper",
	resourceReasonCollectionReleased:                       "collection-released",
	resourceReasonCollectionUseUnknown:                     "collection-use-unknown",
	resourceReasonRowsExhaustedEdgeUnknown:                 "rows-exhausted-edge-unknown",
	resourceReasonResultGuardedDefer:                       "result-guarded-defer",
	resourceReasonResultGuardedRelease:                     "result-guarded-release",
	resourceReasonResultGuardedUnknown:                     "result-guarded-unknown",
	resourceReasonBudgetExhausted:                          "budget-exhausted",
	resourceReasonCallEffectsAsynchronousExposure:          "call-effects-asynchronous-exposure",
	resourceReasonParentCleanup:                            "caller-owned-database-cleanup",
	resourceReasonCapturedAggregateOwner:                   "captured-aggregate-owner",
	resourceReasonCapturedBodyGuardedCleanup:               "captured-body-guarded-cleanup",
	resourceReasonCapturedByLiteralCallingUnreadableCallee: "captured-by-literal-calling-unreadable-callee",
	resourceReasonCapturedByPossiblyRetainedCallback:       "captured-by-possibly-retained-callback",
	resourceReasonCapturedByPriorCleanup:                   "captured-by-prior-cleanup",
	resourceReasonCapturedByRetainingLiteral:               "captured-by-retaining-literal",
	resourceReasonCapturedByStartedLiteral:                 "captured-by-started-literal",
	resourceReasonCapturedCellMayCleanup:                   "captured-cell-may-cleanup",
	resourceReasonCompressionOutputMayBeAbandoned:          "compression-output-may-be-abandoned",
	resourceReasonCanceledAcquisition:                      "context-canceled-before-acquisition",
	resourceReasonDynamicCallee:                            "dynamic-callee",
	resourceReasonErrorPredicateFalseForNil:                "error-predicate-false-for-nil",
	resourceReasonErrorTypeAssertionSucceeded:              "error-type-assertion-succeeded",
	resourceReasonErrorsAsExactAcquisitionError:            "errors-as-exact-acquisition-error",
	resourceReasonErrorsIsNonNilContextSentinel:            "errors-is-non-nil-context-sentinel",
	resourceReasonErrorsIsNonNilFilesystemSentinel:         "errors-is-non-nil-filesystem-sentinel",
	resourceReasonEvidenceNotFound:                         "evidence-not-found",
	resourceReasonEvidenceUnavailable:                      "evidence-unavailable",
	resourceReasonExactErrorEqualsNonNilFilesystemSentinel: "exact-error-equals-non-nil-filesystem-sentinel",
	resourceReasonHeadAcquisition:                          "head-body-acquisition-uncertain",
	resourceReasonHeadClientNotUnconfigured:                "head-client-not-unconfigured",
	resourceReasonHeadRequestModified:                      "head-request-modified",
	resourceReasonHelperCleanupInLoop:                      "helper-cleanup-in-loop",
	resourceReasonImportedHelperCleanupInLoop:              "imported-helper-cleanup-in-loop",
	resourceReasonInterfaceMethod:                          "interface-method",
	resourceReasonHeaderOnlyAcquisition:                    "local-header-only-body-uncertain",
	resourceReasonLocalServerClientOverrideUnresolved:      "local-server-client-override-unresolved",
	resourceReasonLocalServerHandlerUnavailable:            "local-server-handler-unavailable",
	resourceReasonLocalServerHeaderOnlyEffects:             "local-server-header-only-effects",
	resourceReasonLocalServerIdentityUnavailable:           "local-server-identity-unavailable",
	resourceReasonLocalServerWriterEffectsUnavailable:      "local-server-writer-effects-unavailable",
	resourceReasonNestedInTransferredArgument:              "nested-in-transferred-argument",
	resourceReasonUntouched:                                "none",
	resourceReasonOpaqueConsumption:                        "opaque-consumption",
	optionalAcquisitionSuccessPhi:                          "optional-acquisition-success-phi",
	resourceReasonPairedErrorHelperCleanup:                 "paired-error-helper-cleanup",
	resourceReasonPriorDeferMayCleanCapturedCell:           "prior-defer-may-clean-captured-cell",
	resourceReasonReleaseProven:                            "release-proven",
	resourceReasonRepeatedGuardEdgeUnknown:                 "repeated-guard-edge-unknown",
	resourceReasonResourceReturnPath:                       "resource-return-path",
	resourceReasonReturnedCleanupProjection:                "returned-cleanup-projection",
	resourceReasonReturnedMayTransfer:                      "returned-may-transfer",
	resourceReasonReturnedWrapperRetains:                   "returned-wrapper-retains-resource",
	resourceReasonReturnedProjectionLacksCleanup:           "returned-projection-lacks-cleanup",
	resourceReasonReturnedViewCannotRelease:                "returned-view-cannot-release",
	resourceReasonRowsTransactionFinished:                  "rows-transaction-finished",
	resourceReasonTransactionContextCanceled:               "transaction-context-canceled",
	resourceReasonResponseBodyAggregateHandoff:             "response-body-aggregate-handoff",
	resourceReasonSentToChannel:                            "sent-to-channel",
	resourceReasonSettled:                                  "settled",
	resourceReasonStatementParentClosed:                    "statement-parent-closed",
	resourceReasonStoredInMap:                              "stored-in-map",
	resourceReasonIndirectDestinationUnknown:               "indirect-destination-unknown",
	resourceReasonStoredOnCollectionOwner:                  "stored-on-collection-owner",
	resourceReasonWrapperStoredOnForeignOwner:              "wrapper-stored-on-foreign-owner",
	resourceReasonAcquisitionUnreachable:                   "acquisition-unreachable",
	resourceReasonTestifyNoErrorGuard:                      "testify-no-error-guard",
	resourceReasonUnownedReturn:                            "unowned-return",
	resourceReasonUnsummarizedCallee:                       "unsummarized-callee",
	resourceReasonOSIsNotExist:                             "os-isnotexist",
	resourceReasonOSIsExist:                                "os-isexist",
	resourceReasonOSIsPermission:                           "os-ispermission",
	resourceReasonOSIsTimeout:                              "os-istimeout",
	resourceReasonProcessExitReclaims:                      "process-exit-reclaims",
	resourceReasonReturnedRetainingWrapper:                 "returned-retaining-wrapper",
}

func TestResourceLifetimeReasonCodes(t *testing.T) {
	want := resourceReasonSpellings
	if len(want) != int(resourceReasonCount) {
		t.Fatal("every reason needs a boundary spelling assertion")
	}
	for reason := range resourceReasonCount {
		code, ok := want[reason]
		if !ok || reason.String() != code {
			t.Errorf("reason %d: got %q, want %q", reason, reason.String(), code)
		}
	}
	for _, reason := range []resourceLifetimeReason{resourceReasonCount, 255} {
		if reason.String() != "invalid-resource-reason" {
			t.Errorf("invalid reason %d: %q", reason, reason.String())
		}
	}
}

func assertMissingReleaseEvidence(t *testing.T, results []*analysistest.Result) {
	t.Helper()
	want := map[int][]string{
		13: {"17:when this is true", "18:returns here without releasing `config`"},
		24: {"25:reaches the end of the function without releasing the resource"},
		30: {"35:returns here without releasing `config`"},
	}
	for _, result := range results {
		if result.Pass == nil {
			continue
		}
		for _, diagnostic := range result.Diagnostics {
			position := result.Pass.Fset.Position(diagnostic.Pos)
			expected, ok := want[position.Line]
			if filepath.Base(position.Filename) != "missing_release_evidence.go" || !ok {
				continue
			}
			delete(want, position.Line)
			var got []string
			for _, related := range diagnostic.Related {
				got = append(got, fmt.Sprintf("%d:%s", result.Pass.Fset.Position(related.Pos).Line, related.Message))
			}
			if !slices.Equal(got, expected) {
				t.Errorf("line %d: evidence = %q, want %q", position.Line, got, expected)
			}
		}
	}
	if len(want) != 0 {
		t.Errorf("missing evidence diagnostics at lines %v", want)
	}
}

type followupTraceEvent struct {
	Reason    string            `json:"reason"`
	Phase     string            `json:"phase"`
	Outcome   string            `json:"outcome"`
	Candidate string            `json:"candidate"`
	Function  string            `json:"function"`
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
	assertResourceDecisions(t, events)
	assertHTTPBoundaryTrace(t, events)
	assertCleanupBoundaryTrace(t, events)
	assertOwnershipBoundaryTrace(t, events, "indirect-destination-unknown", "indirect_destinations.go:")
	assertOwnershipBoundaryTrace(t, events, "response-body-aggregate-handoff", "body_handoffs.go:")
	assertOwnershipBoundaryTrace(t, events, "call-effects-asynchronous-exposure", "imported_async.go:")
	assertContextErrorGuardTrace(t, events)
	assertUncertainEdgeTrace(t, events, "repeated-guard-edge-unknown", "guard_facts.go:")
	assertUncertainEdgeTrace(t, events, "rows-exhausted-edge-unknown", "sql_")
}

func assertContextErrorGuardTrace(t *testing.T, events []followupTraceEvent) {
	t.Helper()
	for _, event := range events {
		if event.Details["proof"] != "errors-is-non-nil-context-sentinel" {
			continue
		}
		if event.Reason != "acquisition-error-proven" || event.Phase != "evidence" || event.Outcome != "accepted" ||
			!strings.Contains(event.Candidate, "context_error_guards.go:") {
			t.Errorf("unexpected context error guard trace: %+v", event)
		}
		return
	}
	t.Error("missing context sentinel acquisition-error proof")
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
		"captured-cell-may-cleanup":           {"deferred_reassigned_response.go:", "label", "unknown"},
		"paired-error-helper-cleanup":         {"paired_error_cleanup.go:", "label", "unknown"},
		"rows-transaction-finished":           {"sql_row_parents.go:", "label", "unknown"},
		"transaction-context-canceled":        {"transaction_cancellation.go:", "label", "unknown"},
		"captured-body-guarded-cleanup":       {"http_guarded_capture.go:", "label", "unknown"},
		"wrapper-stored-on-foreign-owner":     {"published_wrapper.go:", "label", "unknown"},
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

func assertOwnershipBoundaryTrace(t *testing.T, events []followupTraceEvent, reason, file string) {
	t.Helper()
	for _, event := range events {
		if event.Reason != reason {
			continue
		}
		if event.Phase != "label" || event.Outcome != "unknown" || !strings.Contains(event.Candidate, file) {
			t.Errorf("unexpected %s trace: %+v", reason, event)
		}
		return
	}
	t.Errorf("missing %s ownership boundary", reason)
}

// Final decisions consume the same proof as reporting. An opaque handoff or a
// possible deferred release must not claim cleanup, while a memory-only writer
// is excluded by policy even if its data was not finalized.
func assertResourceDecisions(t *testing.T, events []followupTraceEvent) {
	t.Helper()
	cases := []struct{ function, reason, outcome, file string }{
		{"resourcelifetime.importedAsyncWriter", "opaque-consumption", "unknown", "imported_async.go:10:"},
		{"resourcelifetime.importedSynchronousWriter", "unowned-return", "rejected", "imported_async.go:19:"},
		{"resourcelifetime.importedAsyncWriterBypassed", "unowned-return", "rejected", "imported_async.go:37:"},
		{"resourcelifetime.deferredRequiredClose", "release-proven", "accepted", "required_cleanup.go:11:"},
		{"resourcelifetime.gzipWriterOverLocalBuffer", "memory-writer-no-external-resource", "accepted", "compression_resources.go:103:"},
		{"resourcelifetime.gzipWriterOverConstructedStringBuffer", "memory-writer-no-external-resource", "accepted", "compression_resources.go:126:"},
		{"resourcelifetime.customBufferFactory", "unowned-return", "rejected", "compression_resources.go:135:"},
		{"resourcelifetime.mixedMemoryAndExternalWriter", "unowned-return", "rejected", "compression_resources.go:144:"},
		{"resourcelifetime.filesAppendedToDeferredCloserSlice", "prior-defer-may-release", "unknown", "aggregate_ownership.go:252:"},
	}
	for _, expected := range cases {
		count := 0
		for _, event := range events {
			if event.Function != expected.function || event.Phase != "decision" || event.Reason == "diagnostic-reported" {
				continue
			}
			count++
			if event.Reason != expected.reason || event.Outcome != expected.outcome || !strings.Contains(event.Candidate, expected.file) {
				t.Errorf("unexpected final resource decision: %+v; want %+v", event, expected)
			}
		}
		if count != 1 {
			t.Errorf("final resource decisions for %s = %d, want 1", expected.function, count)
		}
	}
}

func TestResourceMetadataTraceDisabledAllocations(t *testing.T) {
	pass, branch, phi := resourceMetadataTraceFixture(t)
	proof := optionalAcquisitionProof{proof: resourceProof{Reason: optionalAcquisitionSuccessPhi}, resourcePhi: phi}
	restore := analysisTrace.Capture(nil, "", nil)
	defer restore()
	for _, test := range []struct {
		name string
		emit func()
	}{
		{"acquisition error", func() {
			traceAcquisitionErrorProof(pass, branch, resourceReasonTestifyNoErrorGuard, branch.Parent().Pos())
		}},
		{"optional acquisition", func() { traceOptionalAcquisition(pass, proof, phi.Parent().Pos()) }},
	} {
		t.Run(test.name, func(t *testing.T) {
			if allocations := testing.AllocsPerRun(100, test.emit); allocations != 0 {
				t.Fatalf("disabled tracing allocated %g times, want zero", allocations)
			}
		})
	}
}

func TestResourceMetadataTracePreservesEnabledEvidence(t *testing.T) {
	pass, branch, phi := resourceMetadataTraceFixture(t)
	var records []analysisTrace.Record
	restore := analysisTrace.Capture([]string{"resourcelifetime"}, "", func(record analysisTrace.Record) { records = append(records, record) })
	defer restore()
	candidate := branch.Parent().Pos()
	traceAcquisitionErrorProof(pass, branch, resourceReasonTestifyNoErrorGuard, candidate)
	traceOptionalAcquisition(pass, optionalAcquisitionProof{proof: resourceProof{Reason: optionalAcquisitionSuccessPhi}, resourcePhi: phi}, candidate)
	if len(records) != 2 {
		t.Fatalf("got %d events, want two evidence events", len(records))
	}
	for index, reason := range []string{"acquisition-error-proven", "optional-acquisition-success-phi"} {
		record := records[index]
		if record.Phase != "evidence" || record.Reason != reason || record.Outcome != analysisTrace.OutcomeAccepted ||
			record.Function != "tracealloc.Work" || record.Candidate != pass.Fset.Position(candidate).String() {
			t.Fatalf("unexpected evidence event: %+v", record)
		}
	}
}

func resourceMetadataTraceFixture(tb testing.TB) (*analysis.Pass, *ssa.If, *ssa.Phi) {
	tb.Helper()
	pkg := ssaflowtest.BuildPackage(tb, "tracealloc", `package tracealloc
func Work(flag bool) int { value := 0; if flag { value = 1 }; return value }`)
	function := pkg.Func("Work")
	branches := ssaflow.InstructionsOf[*ssa.If](function)
	phis := ssaflow.InstructionsOf[*ssa.Phi](function)
	if len(branches) != 1 || len(phis) != 1 {
		tb.Fatal("fixture must contain one branch and one merged value")
	}
	return &analysis.Pass{Fset: pkg.Prog.Fset, Pkg: pkg.Pkg}, branches[0], phis[0]
}
