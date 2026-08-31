// Package testingchecks provides analyzers for Go test code.
package testingchecks

import (
	"github.com/kojah/gohawk/internal/analyzerbase"
)

const checkTestHelperMarker = analyzerbase.CheckTestHelperMarker

var report = analyzerbase.Report

// Specs returns the testing analyzer declarations in stable order.
func Specs() []analyzerbase.AnalyzerSpec {
	return []analyzerbase.AnalyzerSpec{
		{
			Analyzer: testPolicyAnalyzer(), OptIn: true, SuggestedFix: true,
			Checks: []analyzerbase.CheckInfo{
				{ID: checkTestHelperMarker, Doc: "Reports test helpers that do not call Helper on every return path."},
			},
		},
	}
}
