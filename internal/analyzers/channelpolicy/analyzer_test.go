package channelpolicy

import (
	"testing"

	"github.com/kojah/gohawk/internal/analyzertest"
	"golang.org/x/tools/go/analysis/analysistest"
)

func TestAnalyzer(t *testing.T) {
	analyzertest.Run(t, analysistest.TestData(), Analyzer(), "channelpolicy", "channelpolicy/testfiles")
}

func TestConfiguration(t *testing.T) {
	analyzer := Analyzer()
	if err := analyzer.Flags.Set("max-unexplained-capacity", "10"); err != nil {
		t.Fatalf("set max-unexplained-capacity: %v", err)
	}
	analyzertest.Run(t, analysistest.TestData(), analyzer, "channelpolicy/config")
}
