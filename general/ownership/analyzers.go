// Package ownership provides ownership and lifecycle analyzers.
package ownership

import (
	"github.com/kojah/gohawk/internal/analyzerbase"

	"golang.org/x/tools/go/analysis"
)

const (
	checkCancellationRelease      = analyzerbase.CheckCancellationRelease
	checkChannelCapacityRationale = analyzerbase.CheckChannelCapacityRationale
	checkChannelCallerClose       = analyzerbase.CheckChannelCallerClose
	checkChannelSendAfterClose    = analyzerbase.CheckChannelSendAfterClose
	checkDeferCleanupInLoop       = analyzerbase.CheckDeferCleanupInLoop
	checkExitSkipsDefer           = analyzerbase.CheckExitSkipsDefer
	checkGoroutineJoin            = analyzerbase.CheckGoroutineJoin
	checkGoroutineDetached        = analyzerbase.CheckGoroutineDetached
	checkGoroutineProducerSend    = analyzerbase.CheckGoroutineProducerSend
	checkProcessWait              = analyzerbase.CheckProcessWait
	checkResourceRelease          = analyzerbase.CheckResourceRelease
)

var reportf = analyzerbase.Reportf
var report = analyzerbase.Report
var newChoiceValue = analyzerbase.NewChoiceValue
var newCommaSeparatedChoice = analyzerbase.NewCommaSeparatedChoice
var commaSeparatedSet = analyzerbase.CommaSeparatedSet

// Analyzers returns the ownership and lifecycle analyzers in stable order.
func Analyzers() []*analysis.Analyzer {
	return []*analysis.Analyzer{
		cancellationOwnershipAnalyzer(),
		channelPolicyAnalyzer(),
		deferInLoopAnalyzer(),
		exitPolicyAnalyzer(),
		goroutineOwnershipAnalyzer(),
		processOwnershipAnalyzer(),
		resourceLifetimeAnalyzer(),
	}
}
