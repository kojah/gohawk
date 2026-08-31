package globalstate

import (
	"testing"

	"github.com/kojah/gohawk/internal/analyzertest"
	"golang.org/x/tools/go/analysis/analysistest"
)

func TestAnalyzer(t *testing.T) {
	analyzertest.Run(t, analysistest.TestData(), Analyzer(), "globalstate", "globalstate/testfiles")
}

func TestConfiguration(t *testing.T) {
	analyzer := Analyzer()
	for name, value := range map[string]string{"allow-names": "cache", "allow-types": "globalstate/config.Registry"} {
		if err := analyzer.Flags.Set(name, value); err != nil {
			t.Fatalf("set %s=%s: %v", name, value, err)
		}
	}
	analyzertest.Run(t, analysistest.TestData(), analyzer, "globalstate/config")
}
