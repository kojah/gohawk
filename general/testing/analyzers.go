// Package testingchecks provides analyzers for Go test code.
package testingchecks

import (
	"github.com/kojah/gohawk/internal/analyzerbase"
)

const (
	checkBlockingTestSend    = analyzerbase.CheckBlockingTestSend
	checkBlockingTestReceive = analyzerbase.CheckBlockingTestReceive
	checkBlockingTestSelect  = analyzerbase.CheckBlockingTestSelect
	checkTestHelperMarker    = analyzerbase.CheckTestHelperMarker
)

var reportf = analyzerbase.Reportf
var report = analyzerbase.Report

// Specs returns the testing analyzer declarations in stable order.
func Specs() []analyzerbase.AnalyzerSpec {
	return []analyzerbase.AnalyzerSpec{
		{
			Analyzer: blockingTestAnalyzer(), Profile: analyzerbase.AnalyzerProfileOptIn,
			Checks: []analyzerbase.CheckInfo{
				{ID: checkBlockingTestSend, Doc: "Reports unguarded channel sends in context-aware test code.", Tags: []analyzerbase.Tag{analyzerbase.TagReliability}},
				{ID: checkBlockingTestReceive, Doc: "Reports blocking channel receives in tests without a cancellation escape.", Tags: []analyzerbase.Tag{analyzerbase.TagReliability}},
				{ID: checkBlockingTestSelect, Doc: "Reports blocking selects in tests without a cancellation escape.", Tags: []analyzerbase.Tag{analyzerbase.TagReliability}},
			},
		},
		{
			Analyzer: testPolicyAnalyzer(), Profile: analyzerbase.AnalyzerProfileOptIn, SuggestedFix: true,
			Checks: []analyzerbase.CheckInfo{
				{ID: checkTestHelperMarker, Doc: "Reports test helpers that do not call Helper on every return path.", Tags: []analyzerbase.Tag{analyzerbase.TagPolicy}},
			},
		},
	}
}
