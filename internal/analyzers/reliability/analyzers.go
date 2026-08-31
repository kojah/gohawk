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

var (
	reportf                 = analyzerbase.Reportf
	report                  = analyzerbase.Report
	newCommaSeparatedChoice = analyzerbase.NewCommaSeparatedChoice
	commaSeparatedSet       = analyzerbase.CommaSeparatedSet
)

// Specs returns the reliability and safety analyzer declarations in stable order.
func Specs() []analyzerbase.AnalyzerSpec {
	return []analyzerbase.AnalyzerSpec{
		{
			Analyzer: concurrentCaptureAnalyzer(),
			Checks: []analyzerbase.CheckInfo{
				{ID: checkConcurrentCapture, Doc: "Reports repeatedly launched goroutines that mutate the same captured local."},
			},
		},
		{
			Analyzer: determinismAnalyzer(), OptIn: true,
			Checks: []analyzerbase.CheckInfo{
				{ID: checkDeterministicMapOutput, Doc: "Reports map iteration that reaches ordered output without explicit sorting."},
			},
		},
		{
			Analyzer: errorOwnershipAnalyzer(),
			Checks: []analyzerbase.CheckInfo{
				{ID: checkErrorLogAndReturn, Doc: "Reports functions that both log and return the same error.", OptIn: true},
				{ID: checkErrorTextClassification, Doc: "Reports production code that classifies errors by matching their text."},
				{ID: checkErrorMismatchedInline, Doc: "Reports inline error declarations whose condition checks a different error."},
			},
		},
		{
			Analyzer: evalOrderAnalyzer(),
			Checks: []analyzerbase.CheckInfo{
				{ID: checkEvaluationOrder, Doc: "Reports expressions whose later operand mutates a value read by an earlier operand."},
			},
		},
		{
			Analyzer: globalStateAnalyzer(), OptIn: true,
			Checks: []analyzerbase.CheckInfo{
				{ID: checkMutableGlobalState, Doc: "Reports mutable package-level state without an explicit owner."},
			},
		},
		{
			Analyzer: lockOrderAnalyzer(),
			Checks: []analyzerbase.CheckInfo{
				{ID: checkLockMissingRelease, Doc: "Reports return paths that leave an owned lock held."},
				{ID: checkLockRecursiveAcquire, Doc: "Reports attempts to acquire a lock that is already held."},
				{ID: checkLockContradictoryOrder, Doc: "Reports inconsistent acquisition order for the same pair of locks."},
			},
		},
		{
			Analyzer: oncePolicyAnalyzer(),
			Checks: []analyzerbase.CheckInfo{
				{ID: checkOnceDiscardedWrapper, Doc: "Reports sync.Once function wrappers that are called and immediately discarded."},
			},
		},
		{
			Analyzer: syncMapAtomicityAnalyzer(),
			Checks: []analyzerbase.CheckInfo{
				{ID: checkSyncMapNonAtomicClaim, Doc: "Reports separate sync.Map Load and Delete operations used to claim one value."},
			},
		},
		{
			Analyzer: taintPolicyAnalyzer(), OptIn: true,
			Checks: []analyzerbase.CheckInfo{
				{ID: checkTaintUntrustedSink, Doc: "Reports untrusted input that reaches a configured sensitive sink without validation."},
			},
		},
	}
}
