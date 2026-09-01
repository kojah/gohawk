package cancellationownership

import (
	"testing"

	"github.com/kojah/gohawk/internal/analyzertest"
	"golang.org/x/tools/go/analysis/analysistest"
)

func TestAnalyzer(t *testing.T) {
	analyzertest.Run(t, analysistest.TestData(), Analyzer(), "cancellationownership")
}

func TestSuggestedFixes(t *testing.T) {
	analyzertest.RunWithSuggestedFixes(t, analysistest.TestData(), Analyzer(), "cancellationownership/fix")
}
