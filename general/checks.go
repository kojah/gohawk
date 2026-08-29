package general

import (
	"fmt"
	"go/token"
	"slices"

	"github.com/kojah/gohawk/analysisutil"

	"golang.org/x/tools/go/analysis"
)

// AnalyzerCheck identifies one independently taggable diagnostic rule.
type AnalyzerCheck string

// AnalyzerCheckInfo describes why a specific diagnostic rule matters.
type AnalyzerCheckInfo struct {
	ID   AnalyzerCheck
	Doc  string
	Tags []AnalyzerTag
}

const (
	checkAPIParameterCount        AnalyzerCheck = "apishape/parameter-count"
	checkAPIMixedReceivers        AnalyzerCheck = "apishape/mixed-receivers"
	checkAPIAdjacentSameType      AnalyzerCheck = "apishape/adjacent-same-type"
	checkAPIAdjacentOptional      AnalyzerCheck = "apishape/adjacent-optional-scalars"
	checkContextFirst             AnalyzerCheck = "contextpolicy/context-first"
	checkContextStorage           AnalyzerCheck = "contextpolicy/context-storage"
	checkContextTestOwnership     AnalyzerCheck = "contextpolicy/test-context"
	checkContextNilArgument       AnalyzerCheck = "contextpolicy/nil-context"
	checkClosedStringDomain       AnalyzerCheck = "closedomain/closed-string-domain"
	checkWireKeyedLiteral         AnalyzerCheck = "wirepolicy/keyed-literal"
	checkWireSerializationTag     AnalyzerCheck = "wirepolicy/serialization-tag"
	checkCancellationRelease      AnalyzerCheck = "cancellationownership/release"
	checkChannelCapacityRationale AnalyzerCheck = "channelpolicy/capacity-rationale"
	checkChannelCallerClose       AnalyzerCheck = "channelpolicy/caller-close"
	checkChannelSendAfterClose    AnalyzerCheck = "channelpolicy/send-after-close"
	checkDeferCleanupInLoop       AnalyzerCheck = "deferinloop/cleanup-lifetime"
	checkExitSkipsDefer           AnalyzerCheck = "exitpolicy/skipped-defer"
	checkGoroutineJoin            AnalyzerCheck = "goroutineownership/unjoined"
	checkGoroutineProducerSend    AnalyzerCheck = "goroutineownership/abandoned-send"
	checkProcessWait              AnalyzerCheck = "processownership/missing-wait"
	checkResourceRelease          AnalyzerCheck = "resourcelifetime/missing-release"
	checkConcurrentCapture        AnalyzerCheck = "concurrentcapture/shared-capture"
	checkDeterministicMapOutput   AnalyzerCheck = "determinism/map-output-order"
	checkErrorLogAndReturn        AnalyzerCheck = "errorownership/log-and-return"
	checkErrorTextClassification  AnalyzerCheck = "errorownership/text-classification"
	checkErrorMismatchedInline    AnalyzerCheck = "errorownership/mismatched-inline-error"
	checkEvaluationOrder          AnalyzerCheck = "evalorder/operand-mutation"
	checkMutableGlobalState       AnalyzerCheck = "globalstate/mutable-package-state"
	checkLockMissingRelease       AnalyzerCheck = "lockorder/missing-release"
	checkLockRecursiveAcquire     AnalyzerCheck = "lockorder/recursive-acquire"
	checkLockContradictoryOrder   AnalyzerCheck = "lockorder/contradictory-order"
	checkOnceDiscardedWrapper     AnalyzerCheck = "oncepolicy/discarded-wrapper"
	checkSyncMapNonAtomicClaim    AnalyzerCheck = "syncmapatomicity/non-atomic-claim"
	checkTaintUntrustedSink       AnalyzerCheck = "taintpolicy/untrusted-sink"
	checkBlockingTestSend         AnalyzerCheck = "blockingtest/send"
	checkBlockingTestReceive      AnalyzerCheck = "blockingtest/receive"
	checkBlockingTestSelect       AnalyzerCheck = "blockingtest/select"
	checkTestHelperMarker         AnalyzerCheck = "testpolicy/helper-marker"
)

var analyzerChecks = map[string][]AnalyzerCheckInfo{
	"apishape": {
		{ID: checkAPIParameterCount, Doc: "Reports exported APIs with more than the configured maximum number of parameters.", Tags: []AnalyzerTag{AnalyzerTagPolicy}},
		{ID: checkAPIMixedReceivers, Doc: "Reports types that mix pointer and value receiver methods.", Tags: []AnalyzerTag{AnalyzerTagPolicy}},
		{ID: checkAPIAdjacentSameType, Doc: "Reports long adjacent runs of parameters that share one type.", Tags: []AnalyzerTag{AnalyzerTagPolicy}},
		{ID: checkAPIAdjacentOptional, Doc: "Reports adjacent optional scalar parameters that are easy to swap.", Tags: []AnalyzerTag{AnalyzerTagPolicy}},
	},
	"contextpolicy": {
		{ID: checkContextFirst, Doc: "Reports context.Context parameters that are not first.", Tags: []AnalyzerTag{AnalyzerTagReliability, AnalyzerTagPolicy}},
		{ID: checkContextStorage, Doc: "Reports context.Context values stored in structs.", Tags: []AnalyzerTag{AnalyzerTagReliability, AnalyzerTagPolicy}},
		{ID: checkContextTestOwnership, Doc: "Reports tests that use context.Background instead of the testing handle's context.", Tags: []AnalyzerTag{AnalyzerTagPolicy}},
		{ID: checkContextNilArgument, Doc: "Reports definitely nil context.Context arguments.", Tags: []AnalyzerTag{AnalyzerTagCorrectness}},
	},
	"closedomain": {
		{ID: checkClosedStringDomain, Doc: "Reports exported string fields used as small closed sets of values.", Tags: []AnalyzerTag{AnalyzerTagReliability, AnalyzerTagPolicy}},
	},
	"wirepolicy": {
		{ID: checkWireKeyedLiteral, Doc: "Reports positional composite literals for persisted or wire structs.", Tags: []AnalyzerTag{AnalyzerTagReliability, AnalyzerTagPolicy}},
		{ID: checkWireSerializationTag, Doc: "Reports exported wire fields without explicit JSON or TOML tags.", Tags: []AnalyzerTag{AnalyzerTagReliability, AnalyzerTagPolicy}},
	},
	"cancellationownership": {
		{ID: checkCancellationRelease, Doc: "Reports derived cancel functions that are neither called nor transferred on every return path.", Tags: []AnalyzerTag{AnalyzerTagCorrectness}},
	},
	"channelpolicy": {
		{ID: checkChannelCapacityRationale, Doc: "Reports large constant channel capacities without a nearby bounded rationale.", Tags: []AnalyzerTag{AnalyzerTagPolicy}},
		{ID: checkChannelCallerClose, Doc: "Reports functions that close channels received from their callers.", Tags: []AnalyzerTag{AnalyzerTagReliability, AnalyzerTagPolicy}},
		{ID: checkChannelSendAfterClose, Doc: "Reports sends reachable after a channel has been closed.", Tags: []AnalyzerTag{AnalyzerTagCorrectness}},
	},
	"deferinloop": {
		{ID: checkDeferCleanupInLoop, Doc: "Reports cleanup defers whose lifetime extends across loop iterations.", Tags: []AnalyzerTag{AnalyzerTagReliability}},
	},
	"exitpolicy": {
		{ID: checkExitSkipsDefer, Doc: "Reports immediate process termination that bypasses an earlier defer.", Tags: []AnalyzerTag{AnalyzerTagCorrectness}},
	},
	"goroutineownership": {
		{ID: checkGoroutineJoin, Doc: "Reports goroutines without a recognizable join handle or lifecycle owner.", Tags: []AnalyzerTag{AnalyzerTagReliability}},
		{ID: checkGoroutineProducerSend, Doc: "Reports producer goroutines that can block after their receiver stops waiting.", Tags: []AnalyzerTag{AnalyzerTagReliability}},
	},
	"processownership": {
		{ID: checkProcessWait, Doc: "Reports successfully started commands that are neither waited on nor transferred.", Tags: []AnalyzerTag{AnalyzerTagCorrectness}},
	},
	"resourcelifetime": {
		{ID: checkResourceRelease, Doc: "Reports owned resources that are not released on every return path.", Tags: []AnalyzerTag{AnalyzerTagCorrectness}},
	},
	"concurrentcapture": {
		{ID: checkConcurrentCapture, Doc: "Reports repeatedly launched goroutines that mutate the same captured local.", Tags: []AnalyzerTag{AnalyzerTagCorrectness}},
	},
	"determinism": {
		{ID: checkDeterministicMapOutput, Doc: "Reports map iteration that reaches ordered output without explicit sorting.", Tags: []AnalyzerTag{AnalyzerTagReliability}},
	},
	"errorownership": {
		{ID: checkErrorLogAndReturn, Doc: "Reports functions that both log and return the same error.", Tags: []AnalyzerTag{AnalyzerTagReliability, AnalyzerTagPolicy}},
		{ID: checkErrorTextClassification, Doc: "Reports production code that classifies errors by matching their text.", Tags: []AnalyzerTag{AnalyzerTagReliability}},
		{ID: checkErrorMismatchedInline, Doc: "Reports inline error declarations whose condition checks a different error.", Tags: []AnalyzerTag{AnalyzerTagCorrectness}},
	},
	"evalorder": {
		{ID: checkEvaluationOrder, Doc: "Reports expressions whose later operand mutates a value read by an earlier operand.", Tags: []AnalyzerTag{AnalyzerTagCorrectness}},
	},
	"globalstate": {
		{ID: checkMutableGlobalState, Doc: "Reports mutable package-level state without an explicit owner.", Tags: []AnalyzerTag{AnalyzerTagReliability, AnalyzerTagPolicy}},
	},
	"lockorder": {
		{ID: checkLockMissingRelease, Doc: "Reports return paths that leave an owned lock held.", Tags: []AnalyzerTag{AnalyzerTagCorrectness}},
		{ID: checkLockRecursiveAcquire, Doc: "Reports attempts to acquire a lock that is already held.", Tags: []AnalyzerTag{AnalyzerTagCorrectness}},
		{ID: checkLockContradictoryOrder, Doc: "Reports inconsistent acquisition order for the same pair of locks.", Tags: []AnalyzerTag{AnalyzerTagCorrectness}},
	},
	"oncepolicy": {
		{ID: checkOnceDiscardedWrapper, Doc: "Reports sync.Once function wrappers that are called and immediately discarded.", Tags: []AnalyzerTag{AnalyzerTagCorrectness}},
	},
	"syncmapatomicity": {
		{ID: checkSyncMapNonAtomicClaim, Doc: "Reports separate sync.Map Load and Delete operations used to claim one value.", Tags: []AnalyzerTag{AnalyzerTagCorrectness}},
	},
	"taintpolicy": {
		{ID: checkTaintUntrustedSink, Doc: "Reports untrusted input that reaches a configured sensitive sink without validation.", Tags: []AnalyzerTag{AnalyzerTagCorrectness, AnalyzerTagReliability}},
	},
	"blockingtest": {
		{ID: checkBlockingTestSend, Doc: "Reports unguarded channel sends in context-aware test code.", Tags: []AnalyzerTag{AnalyzerTagReliability}},
		{ID: checkBlockingTestReceive, Doc: "Reports blocking channel receives in tests without a cancellation escape.", Tags: []AnalyzerTag{AnalyzerTagReliability}},
		{ID: checkBlockingTestSelect, Doc: "Reports blocking selects in tests without a cancellation escape.", Tags: []AnalyzerTag{AnalyzerTagReliability}},
	},
	"testpolicy": {
		{ID: checkTestHelperMarker, Doc: "Reports test helpers that do not call Helper on every return path.", Tags: []AnalyzerTag{AnalyzerTagPolicy}},
	},
}

func cloneChecks(checks []AnalyzerCheckInfo) []AnalyzerCheckInfo {
	cloned := make([]AnalyzerCheckInfo, len(checks))
	for index, check := range checks {
		cloned[index] = AnalyzerCheckInfo{ID: check.ID, Doc: check.Doc, Tags: slices.Clone(check.Tags)}
	}
	return cloned
}

func reportf(pass *analysis.Pass, check AnalyzerCheck, position token.Pos, format string, args ...any) {
	source := analysisutil.SourceRange(pass, position)
	report(pass, check, analysis.Diagnostic{
		Pos:     source.Pos(),
		End:     source.End(),
		Message: fmt.Sprintf(format, args...),
	})
}

func report(pass *analysis.Pass, check AnalyzerCheck, diagnostic analysis.Diagnostic) {
	diagnostic.Category = string(check)
	pass.Report(diagnostic)
}
