package general

import (
	"github.com/kojah/gohawk/analysisutil"

	"golang.org/x/tools/go/analysis"
)

// AnalyzerGroup is a related set of Go policy analyzers.
type AnalyzerGroup struct {
	Name      string
	Doc       string
	Analyzers []*analysis.Analyzer
}

// AnalyzerGroups returns framework-neutral Go policy analyzers grouped by concern.
func AnalyzerGroups() []AnalyzerGroup {
	groups := []AnalyzerGroup{
		{
			Name: "contracts",
			Doc:  "API and data contracts",
			Analyzers: []*analysis.Analyzer{
				apiShapeAnalyzer(),
				contextPolicyAnalyzer(),
				closedDomainAnalyzer(),
				wirePolicyAnalyzer(),
			},
		},
		{
			Name: "ownership",
			Doc:  "ownership and lifecycle",
			Analyzers: []*analysis.Analyzer{
				cancellationOwnershipAnalyzer(),
				channelPolicyAnalyzer(),
				goroutineOwnershipAnalyzer(),
				processOwnershipAnalyzer(),
				resourceLifetimeAnalyzer(),
			},
		},
		{
			Name: "reliability",
			Doc:  "reliability and safety",
			Analyzers: []*analysis.Analyzer{
				determinismAnalyzer(),
				errorOwnershipAnalyzer(),
				globalStateAnalyzer(),
				lockOrderAnalyzer(),
				taintPolicyAnalyzer(),
			},
		},
		{
			Name: "testing",
			Doc:  "test infrastructure",
			Analyzers: []*analysis.Analyzer{
				blockingTestAnalyzer(),
				testPolicyAnalyzer(),
			},
		},
	}
	for groupIndex := range groups {
		for analyzerIndex, analyzer := range groups[groupIndex].Analyzers {
			groups[groupIndex].Analyzers[analyzerIndex] = withSuppressions(analyzer)
		}
	}
	return groups
}

func withSuppressions(analyzer *analysis.Analyzer) *analysis.Analyzer {
	run := analyzer.Run
	analyzer.Run = func(pass *analysis.Pass) (any, error) {
		report := pass.Report
		pass.Report = func(diagnostic analysis.Diagnostic) {
			if !analysisutil.DiagnosticSuppressed(pass, diagnostic.Pos, analyzer.Name) {
				report(diagnostic)
			}
		}
		defer func() { pass.Report = report }()
		return run(pass)
	}
	return analyzer
}

// Analyzers returns all framework-neutral Go policy analyzers in stable execution order.
func Analyzers() []*analysis.Analyzer {
	groups := AnalyzerGroups()
	names := []string{
		"apishape",
		"contextpolicy",
		"globalstate",
		"wirepolicy",
		"testpolicy",
		"blockingtest",
		"goroutineownership",
		"errorownership",
		"channelpolicy",
		"processownership",
		"closedomain",
		"taintpolicy",
		"lockorder",
		"resourcelifetime",
		"determinism",
		"cancellationownership",
	}
	analyzers := make([]*analysis.Analyzer, 0, len(names))
	for _, name := range names {
	findAnalyzer:
		for _, group := range groups {
			for _, analyzer := range group.Analyzers {
				if analyzer.Name == name {
					analyzers = append(analyzers, analyzer)
					break findAnalyzer
				}
			}
		}
	}
	return analyzers
}
