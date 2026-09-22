package condsafety

import (
	"testing"

	"github.com/kojah/gohawk/internal/analyzertest"
	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/analysis/analysistest"
)

func TestAnalyzer(t *testing.T) {
	analyzer := Analyzer()
	run := analyzer.Run
	analyzer.Run = func(pass *analysis.Pass) (any, error) {
		report := pass.Report
		pass.Report = func(diagnostic analysis.Diagnostic) {
			if len(diagnostic.Related) != 2 {
				t.Errorf("related evidence = %v, want two locations", diagnostic.Related)
			}
			for _, related := range diagnostic.Related {
				if !related.Pos.IsValid() || related.Message == "" {
					t.Errorf("invalid evidence: %+v", related)
				}
			}
			report(diagnostic)
		}
		return run(pass)
	}
	analyzertest.Run(t, analysistest.TestData(), analyzer, "condsafety")
}
