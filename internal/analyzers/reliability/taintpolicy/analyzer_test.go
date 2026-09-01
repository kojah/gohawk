package taintpolicy

import (
	"testing"

	"github.com/kojah/gohawk/internal/analyzertest"
	"golang.org/x/tools/go/analysis/analysistest"
)

func TestAnalyzer(t *testing.T) {
	analyzertest.Run(t, analysistest.TestData(), Analyzer(), "taintpolicy")
}

func TestConfiguration(t *testing.T) {
	analyzer := Analyzer()
	for name, value := range map[string]string{"sinks": "process", "sanitizers": "taintpolicy/config.scrub"} {
		if err := analyzer.Flags.Set(name, value); err != nil {
			t.Fatalf("set %s=%s: %v", name, value, err)
		}
	}
	analyzertest.Run(t, analysistest.TestData(), analyzer, "taintpolicy/config")
}
