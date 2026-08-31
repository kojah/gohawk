package testpolicy

import (
	"testing"

	"github.com/kojah/gohawk/internal/analyzertest"
	"golang.org/x/tools/go/analysis/analysistest"
)

func TestAnalyzer(t *testing.T) {
	analyzertest.Run(t, analysistest.TestData(), Analyzer(), "testpolicy")
}

func TestSuggestedFixes(t *testing.T) {
	analyzertest.RunWithSuggestedFixes(t, analysistest.TestData(), Analyzer(), "testpolicy/fix")
}
