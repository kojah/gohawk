package general

import (
	"slices"

	"github.com/kojah/gohawk/internal/analyzerbase"
)

// AnalyzerCheck identifies one independently taggable diagnostic rule.
type AnalyzerCheck string

// CheckProfile controls whether a check runs whenever its analyzer is selected.
type CheckProfile string

const (
	CheckProfileDefault CheckProfile = "default"
	CheckProfileOptIn   CheckProfile = "opt-in"
)

// AnalyzerCheckInfo describes why a specific diagnostic rule matters.
type AnalyzerCheckInfo struct {
	ID      AnalyzerCheck
	Doc     string
	Profile CheckProfile
	Tags    []AnalyzerTag
}

// EnabledByDefault reports whether the check runs when its analyzer is selected.
func (info AnalyzerCheckInfo) EnabledByDefault() bool {
	return info.Profile == CheckProfileDefault
}

const (
	checkAPIParameterCount        AnalyzerCheck = AnalyzerCheck(analyzerbase.CheckAPIParameterCount)
	checkAPIMixedReceivers        AnalyzerCheck = AnalyzerCheck(analyzerbase.CheckAPIMixedReceivers)
	checkAPIAdjacentSameType      AnalyzerCheck = AnalyzerCheck(analyzerbase.CheckAPIAdjacentSameType)
	checkAPIAdjacentOptional      AnalyzerCheck = AnalyzerCheck(analyzerbase.CheckAPIAdjacentOptional)
	checkContextFirst             AnalyzerCheck = AnalyzerCheck(analyzerbase.CheckContextFirst)
	checkContextStorage           AnalyzerCheck = AnalyzerCheck(analyzerbase.CheckContextStorage)
	checkContextTestOwnership     AnalyzerCheck = AnalyzerCheck(analyzerbase.CheckContextTestOwnership)
	checkContextNilArgument       AnalyzerCheck = AnalyzerCheck(analyzerbase.CheckContextNilArgument)
	checkClosedStringDomain       AnalyzerCheck = AnalyzerCheck(analyzerbase.CheckClosedStringDomain)
	checkWireKeyedLiteral         AnalyzerCheck = AnalyzerCheck(analyzerbase.CheckWireKeyedLiteral)
	checkWireSerializationTag     AnalyzerCheck = AnalyzerCheck(analyzerbase.CheckWireSerializationTag)
	checkCancellationRelease      AnalyzerCheck = AnalyzerCheck(analyzerbase.CheckCancellationRelease)
	checkChannelCapacityRationale AnalyzerCheck = AnalyzerCheck(analyzerbase.CheckChannelCapacityRationale)
	checkChannelCallerClose       AnalyzerCheck = AnalyzerCheck(analyzerbase.CheckChannelCallerClose)
	checkChannelSendAfterClose    AnalyzerCheck = AnalyzerCheck(analyzerbase.CheckChannelSendAfterClose)
	checkDeferCleanupInLoop       AnalyzerCheck = AnalyzerCheck(analyzerbase.CheckDeferCleanupInLoop)
	checkExitSkipsDefer           AnalyzerCheck = AnalyzerCheck(analyzerbase.CheckExitSkipsDefer)
	checkGoroutineJoin            AnalyzerCheck = AnalyzerCheck(analyzerbase.CheckGoroutineJoin)
	checkGoroutineDetached        AnalyzerCheck = AnalyzerCheck(analyzerbase.CheckGoroutineDetached)
	checkGoroutineProducerSend    AnalyzerCheck = AnalyzerCheck(analyzerbase.CheckGoroutineProducerSend)
	checkProcessWait              AnalyzerCheck = AnalyzerCheck(analyzerbase.CheckProcessWait)
	checkResourceRelease          AnalyzerCheck = AnalyzerCheck(analyzerbase.CheckResourceRelease)
	checkConcurrentCapture        AnalyzerCheck = AnalyzerCheck(analyzerbase.CheckConcurrentCapture)
	checkDeterministicMapOutput   AnalyzerCheck = AnalyzerCheck(analyzerbase.CheckDeterministicMapOutput)
	checkErrorLogAndReturn        AnalyzerCheck = AnalyzerCheck(analyzerbase.CheckErrorLogAndReturn)
	checkErrorTextClassification  AnalyzerCheck = AnalyzerCheck(analyzerbase.CheckErrorTextClassification)
	checkErrorMismatchedInline    AnalyzerCheck = AnalyzerCheck(analyzerbase.CheckErrorMismatchedInline)
	checkEvaluationOrder          AnalyzerCheck = AnalyzerCheck(analyzerbase.CheckEvaluationOrder)
	checkMutableGlobalState       AnalyzerCheck = AnalyzerCheck(analyzerbase.CheckMutableGlobalState)
	checkLockMissingRelease       AnalyzerCheck = AnalyzerCheck(analyzerbase.CheckLockMissingRelease)
	checkLockRecursiveAcquire     AnalyzerCheck = AnalyzerCheck(analyzerbase.CheckLockRecursiveAcquire)
	checkLockContradictoryOrder   AnalyzerCheck = AnalyzerCheck(analyzerbase.CheckLockContradictoryOrder)
	checkOnceDiscardedWrapper     AnalyzerCheck = AnalyzerCheck(analyzerbase.CheckOnceDiscardedWrapper)
	checkSyncMapNonAtomicClaim    AnalyzerCheck = AnalyzerCheck(analyzerbase.CheckSyncMapNonAtomicClaim)
	checkTaintUntrustedSink       AnalyzerCheck = AnalyzerCheck(analyzerbase.CheckTaintUntrustedSink)
	checkBlockingTestSend         AnalyzerCheck = AnalyzerCheck(analyzerbase.CheckBlockingTestSend)
	checkBlockingTestReceive      AnalyzerCheck = AnalyzerCheck(analyzerbase.CheckBlockingTestReceive)
	checkBlockingTestSelect       AnalyzerCheck = AnalyzerCheck(analyzerbase.CheckBlockingTestSelect)
	checkTestHelperMarker         AnalyzerCheck = AnalyzerCheck(analyzerbase.CheckTestHelperMarker)
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
		{ID: checkContextTestOwnership, Doc: "Reports tests that use context.Background instead of the testing handle's context.", Profile: CheckProfileOptIn, Tags: []AnalyzerTag{AnalyzerTagPolicy}},
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
		{ID: checkChannelCapacityRationale, Doc: "Reports large constant channel capacities without a nearby bounded rationale.", Profile: CheckProfileOptIn, Tags: []AnalyzerTag{AnalyzerTagPolicy}},
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
		{ID: checkGoroutineJoin, Doc: "Reports goroutines with a recognizable join or lifecycle mechanism that is not honored on every return path.", Tags: []AnalyzerTag{AnalyzerTagReliability}},
		{ID: checkGoroutineDetached, Doc: "Reports goroutines without a recognizable join handle or lifecycle owner.", Profile: CheckProfileOptIn, Tags: []AnalyzerTag{AnalyzerTagReliability}},
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
		{ID: checkErrorLogAndReturn, Doc: "Reports functions that both log and return the same error.", Profile: CheckProfileOptIn, Tags: []AnalyzerTag{AnalyzerTagReliability, AnalyzerTagPolicy}},
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
		profile := check.Profile
		if profile == "" {
			profile = CheckProfileDefault
		}
		cloned[index] = AnalyzerCheckInfo{ID: check.ID, Doc: check.Doc, Profile: profile, Tags: slices.Clone(check.Tags)}
	}
	return cloned
}
