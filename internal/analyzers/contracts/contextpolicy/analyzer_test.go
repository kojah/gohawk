package contextpolicy

import (
	"testing"

	"github.com/kojah/gohawk/internal/analyzertest"
	"golang.org/x/tools/go/analysis/analysistest"
)

func TestAnalyzer(t *testing.T) {
	analyzertest.Run(t, analysistest.TestData(), Analyzer(), "contextpolicy", "contextpolicy/production")
}

func TestConfiguration(t *testing.T) {
	analyzer := Analyzer()
	if err := analyzer.Flags.Set("prefer-test-context", "false"); err != nil {
		t.Fatalf("set prefer-test-context: %v", err)
	}
	analyzertest.Run(t, analysistest.TestData(), analyzer, "contextpolicy/config")
}
