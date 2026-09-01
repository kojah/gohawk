package goroutineownership

import (
	"testing"

	"github.com/kojah/gohawk/internal/analyzertest"
	"golang.org/x/tools/go/analysis/analysistest"
)

func TestAnalyzer(t *testing.T) {
	analyzertest.Run(t, analysistest.TestData(), Analyzer(), "goroutineownership")
}

func TestCausalJoins(t *testing.T) {
	analyzertest.Run(t, analysistest.TestData(), Analyzer(), "goroutineownership/causal")
}

func TestJoinMode(t *testing.T) {
	analyzer := Analyzer()
	if err := analyzer.Flags.Set("mode", "join"); err != nil {
		t.Fatalf("set mode: %v", err)
	}
	analyzertest.Run(t, analysistest.TestData(), analyzer, "goroutineownership/strict")
}

func TestLifecycleMode(t *testing.T) {
	analyzer := Analyzer()
	if err := analyzer.Flags.Set("mode", "lifecycle"); err != nil {
		t.Fatalf("set mode: %v", err)
	}
	analyzertest.Run(t, analysistest.TestData(), analyzer, "goroutineownership/lifecycle")
}
