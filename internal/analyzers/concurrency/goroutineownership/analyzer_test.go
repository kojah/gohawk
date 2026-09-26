package goroutineownership

import (
	"testing"

	"github.com/kojah/gohawk/internal/analyzertest"
	"golang.org/x/tools/go/analysis/analysistest"
)

func TestAnalyzer(t *testing.T) {
	tracePath := enableSummaryJoinTrace(t)
	analyzertest.Run(t, analysistest.TestData(), Analyzer(), "goroutineownership", "summaryjoins", "processexit")
	assertFollowupBoundaryTrace(t, tracePath)
	assertLabelTrace(t, tracePath)
}
