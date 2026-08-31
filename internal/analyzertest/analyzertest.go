// Package analyzertest provides the common behavioral harness for gohawk analyzers.
package analyzertest

import (
	"strings"
	"testing"

	"github.com/kojah/gohawk/analysisutil"

	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/analysis/analysistest"
)

// Run checks diagnostics and their source ranges against analysistest fixtures.
func Run(t *testing.T, testdata string, analyzer *analysis.Analyzer, patterns ...string) {
	t.Helper()
	analysistest.Run(t, testdata, requireDiagnosticContract(t, analyzer), patterns...)
}

// RunWithSuggestedFixes checks diagnostics and golden suggested fixes.
func RunWithSuggestedFixes(t *testing.T, testdata string, analyzer *analysis.Analyzer, patterns ...string) {
	t.Helper()
	analysistest.RunWithSuggestedFixes(t, testdata, requireDiagnosticContract(t, analyzer), patterns...)
}

func requireDiagnosticContract(t *testing.T, analyzer *analysis.Analyzer) *analysis.Analyzer {
	t.Helper()
	run := analyzer.Run
	analyzer.Run = func(pass *analysis.Pass) (any, error) {
		report := pass.Report
		pass.Report = func(diagnostic analysis.Diagnostic) {
			if diagnostic.End <= diagnostic.Pos {
				t.Errorf("%s diagnostic %q has no precise range", analyzer.Name, diagnostic.Message)
			}
			if !strings.HasPrefix(diagnostic.Category, analyzer.Name+"/") {
				t.Errorf("%s diagnostic %q has invalid check identity %q", analyzer.Name, diagnostic.Message, diagnostic.Category)
			}
			if !analysisutil.DiagnosticSuppressed(pass, diagnostic.Pos, analyzer.Name) {
				report(diagnostic)
			}
		}
		defer func() { pass.Report = report }()
		return run(pass)
	}
	return analyzer
}
