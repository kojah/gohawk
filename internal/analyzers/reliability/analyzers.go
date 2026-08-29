// Package reliability provides reliability and safety analyzers.
package reliability

import (
	"github.com/kojah/gohawk/internal/analyzerbase"
)

const (
	checkConcurrentCapture       = analyzerbase.CheckConcurrentCapture
	checkDeterministicMapOutput  = analyzerbase.CheckDeterministicMapOutput
	checkErrorLogAndReturn       = analyzerbase.CheckErrorLogAndReturn
	checkErrorTextClassification = analyzerbase.CheckErrorTextClassification
	checkErrorMismatchedInline   = analyzerbase.CheckErrorMismatchedInline
	checkEvaluationOrder         = analyzerbase.CheckEvaluationOrder
	checkMutableGlobalState      = analyzerbase.CheckMutableGlobalState
	checkLockMissingRelease      = analyzerbase.CheckLockMissingRelease
	checkLockRecursiveAcquire    = analyzerbase.CheckLockRecursiveAcquire
	checkLockContradictoryOrder  = analyzerbase.CheckLockContradictoryOrder
	checkOnceDiscardedWrapper    = analyzerbase.CheckOnceDiscardedWrapper
	checkSyncMapNonAtomicClaim   = analyzerbase.CheckSyncMapNonAtomicClaim
	checkTaintUntrustedSink      = analyzerbase.CheckTaintUntrustedSink
)

var reportf = analyzerbase.Reportf
var report = analyzerbase.Report
var newCommaSeparatedChoice = analyzerbase.NewCommaSeparatedChoice
var commaSeparatedSet = analyzerbase.CommaSeparatedSet

// Specs returns the reliability and safety analyzer declarations in stable order.
func Specs() []analyzerbase.AnalyzerSpec {
	return []analyzerbase.AnalyzerSpec{
		{
			Analyzer: concurrentCaptureAnalyzer(),
			Checks: []analyzerbase.CheckInfo{
				{ID: checkConcurrentCapture, Doc: "Reports repeatedly launched goroutines that mutate the same captured local.", Tags: []analyzerbase.Tag{analyzerbase.TagCorrectness}},
			},
		},
		{
			Analyzer: determinismAnalyzer(), Profile: analyzerbase.AnalyzerProfileOptIn,
			Checks: []analyzerbase.CheckInfo{
				{ID: checkDeterministicMapOutput, Doc: "Reports map iteration that reaches ordered output without explicit sorting.", Tags: []analyzerbase.Tag{analyzerbase.TagReliability}},
			},
		},
		{
			Analyzer: errorOwnershipAnalyzer(),
			Checks: []analyzerbase.CheckInfo{
				{ID: checkErrorLogAndReturn, Doc: "Reports functions that both log and return the same error.", Profile: analyzerbase.CheckProfileOptIn, Tags: []analyzerbase.Tag{analyzerbase.TagReliability, analyzerbase.TagPolicy}},
				{ID: checkErrorTextClassification, Doc: "Reports production code that classifies errors by matching their text.", Tags: []analyzerbase.Tag{analyzerbase.TagReliability}},
				{ID: checkErrorMismatchedInline, Doc: "Reports inline error declarations whose condition checks a different error.", Tags: []analyzerbase.Tag{analyzerbase.TagCorrectness}},
			},
		},
		{
			Analyzer: evalOrderAnalyzer(),
			Checks: []analyzerbase.CheckInfo{
				{ID: checkEvaluationOrder, Doc: "Reports expressions whose later operand mutates a value read by an earlier operand.", Tags: []analyzerbase.Tag{analyzerbase.TagCorrectness}},
			},
		},
		{
			Analyzer: globalStateAnalyzer(), Profile: analyzerbase.AnalyzerProfileOptIn,
			Checks: []analyzerbase.CheckInfo{
				{ID: checkMutableGlobalState, Doc: "Reports mutable package-level state without an explicit owner.", Tags: []analyzerbase.Tag{analyzerbase.TagReliability, analyzerbase.TagPolicy}},
			},
		},
		{
			Analyzer: lockOrderAnalyzer(),
			Checks: []analyzerbase.CheckInfo{
				{ID: checkLockMissingRelease, Doc: "Reports return paths that leave an owned lock held.", Tags: []analyzerbase.Tag{analyzerbase.TagCorrectness}},
				{ID: checkLockRecursiveAcquire, Doc: "Reports attempts to acquire a lock that is already held.", Tags: []analyzerbase.Tag{analyzerbase.TagCorrectness}},
				{ID: checkLockContradictoryOrder, Doc: "Reports inconsistent acquisition order for the same pair of locks.", Tags: []analyzerbase.Tag{analyzerbase.TagCorrectness}},
			},
		},
		{
			Analyzer: oncePolicyAnalyzer(),
			Checks: []analyzerbase.CheckInfo{
				{ID: checkOnceDiscardedWrapper, Doc: "Reports sync.Once function wrappers that are called and immediately discarded.", Tags: []analyzerbase.Tag{analyzerbase.TagCorrectness}},
			},
		},
		{
			Analyzer: syncMapAtomicityAnalyzer(),
			Checks: []analyzerbase.CheckInfo{
				{ID: checkSyncMapNonAtomicClaim, Doc: "Reports separate sync.Map Load and Delete operations used to claim one value.", Tags: []analyzerbase.Tag{analyzerbase.TagCorrectness}},
			},
		},
		{
			Analyzer: taintPolicyAnalyzer(), Profile: analyzerbase.AnalyzerProfileOptIn,
			Checks: []analyzerbase.CheckInfo{
				{ID: checkTaintUntrustedSink, Doc: "Reports untrusted input that reaches a configured sensitive sink without validation.", Tags: []analyzerbase.Tag{analyzerbase.TagCorrectness, analyzerbase.TagReliability}},
			},
		},
	}
}
