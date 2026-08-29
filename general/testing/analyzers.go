// Package testingchecks provides analyzers for Go test code.
package testingchecks

import (
	"github.com/kojah/gohawk/internal/analyzerbase"

	"golang.org/x/tools/go/analysis"
)

const (
	checkBlockingTestSend    = analyzerbase.CheckBlockingTestSend
	checkBlockingTestReceive = analyzerbase.CheckBlockingTestReceive
	checkBlockingTestSelect  = analyzerbase.CheckBlockingTestSelect
	checkTestHelperMarker    = analyzerbase.CheckTestHelperMarker
)

var reportf = analyzerbase.Reportf
var report = analyzerbase.Report

// Analyzers returns the testing analyzers in stable order.
func Analyzers() []*analysis.Analyzer {
	return []*analysis.Analyzer{
		blockingTestAnalyzer(),
		testPolicyAnalyzer(),
	}
}
