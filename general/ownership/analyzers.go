// Package ownership provides ownership and lifecycle analyzers.
package ownership

import (
	"github.com/kojah/gohawk/internal/analyzerbase"
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

// Specs returns the ownership and lifecycle analyzer declarations in stable order.
func Specs() []analyzerbase.AnalyzerSpec {
	return []analyzerbase.AnalyzerSpec{
		{
			Analyzer: cancellationOwnershipAnalyzer(), SuggestedFix: true,
			Checks: []analyzerbase.CheckInfo{
				{ID: checkCancellationRelease, Doc: "Reports derived cancel functions that are neither called nor transferred on every return path.", Tags: []analyzerbase.Tag{analyzerbase.TagCorrectness}},
			},
		},
		{
			Analyzer: channelPolicyAnalyzer(),
			Checks: []analyzerbase.CheckInfo{
				{ID: checkChannelCapacityRationale, Doc: "Reports large constant channel capacities without a nearby bounded rationale.", Profile: analyzerbase.CheckProfileOptIn, Tags: []analyzerbase.Tag{analyzerbase.TagPolicy}},
				{ID: checkChannelCallerClose, Doc: "Reports functions that close channels received from their callers.", Tags: []analyzerbase.Tag{analyzerbase.TagReliability, analyzerbase.TagPolicy}},
				{ID: checkChannelSendAfterClose, Doc: "Reports sends reachable after a channel has been closed.", Tags: []analyzerbase.Tag{analyzerbase.TagCorrectness}},
			},
		},
		{
			Analyzer: deferInLoopAnalyzer(),
			Checks: []analyzerbase.CheckInfo{
				{ID: checkDeferCleanupInLoop, Doc: "Reports cleanup defers whose lifetime extends across loop iterations.", Tags: []analyzerbase.Tag{analyzerbase.TagReliability}},
			},
		},
		{
			Analyzer: exitPolicyAnalyzer(),
			Checks: []analyzerbase.CheckInfo{
				{ID: checkExitSkipsDefer, Doc: "Reports immediate process termination that bypasses an earlier defer.", Tags: []analyzerbase.Tag{analyzerbase.TagCorrectness}},
			},
		},
		{
			Analyzer: goroutineOwnershipAnalyzer(),
			Checks: []analyzerbase.CheckInfo{
				{ID: checkGoroutineJoin, Doc: "Reports goroutines with a recognizable join or lifecycle mechanism that is not honored on every return path.", Tags: []analyzerbase.Tag{analyzerbase.TagReliability}},
				{ID: checkGoroutineDetached, Doc: "Reports goroutines without a recognizable join handle or lifecycle owner.", Profile: analyzerbase.CheckProfileOptIn, Tags: []analyzerbase.Tag{analyzerbase.TagReliability}},
				{ID: checkGoroutineProducerSend, Doc: "Reports producer goroutines that can block after their receiver stops waiting.", Tags: []analyzerbase.Tag{analyzerbase.TagReliability}},
			},
		},
		{
			Analyzer: processOwnershipAnalyzer(),
			Checks: []analyzerbase.CheckInfo{
				{ID: checkProcessWait, Doc: "Reports successfully started commands that are neither waited on nor transferred.", Tags: []analyzerbase.Tag{analyzerbase.TagCorrectness}},
			},
		},
		{
			Analyzer: resourceLifetimeAnalyzer(),
			Checks: []analyzerbase.CheckInfo{
				{ID: checkResourceRelease, Doc: "Reports owned resources that are not released on every return path.", Tags: []analyzerbase.Tag{analyzerbase.TagCorrectness}},
			},
		},
	}
}
