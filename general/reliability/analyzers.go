// Package reliability provides reliability and safety analyzers.
package reliability

import (
	"github.com/kojah/gohawk/internal/analyzerbase"

	"golang.org/x/tools/go/analysis"
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

// Analyzers returns the reliability and safety analyzers in stable order.
func Analyzers() []*analysis.Analyzer {
	return []*analysis.Analyzer{
		concurrentCaptureAnalyzer(),
		determinismAnalyzer(),
		errorOwnershipAnalyzer(),
		evalOrderAnalyzer(),
		globalStateAnalyzer(),
		lockOrderAnalyzer(),
		oncePolicyAnalyzer(),
		syncMapAtomicityAnalyzer(),
		taintPolicyAnalyzer(),
	}
}
