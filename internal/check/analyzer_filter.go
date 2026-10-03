package check

import (
	analysisTrace "github.com/kojah/gohawk/internal/trace"
	"golang.org/x/tools/go/analysis"
)

// FilterAnalyzerReports shallow-copies analyzer and filters reports by exact
// diagnostic category. When every declared check is disabled, Run is skipped.
// Otherwise the original result/error is retained and pass.Report is restored
// on exit. Selection validation and construction of disabled remain the caller's
// responsibility; the caller must not modify disabled during a Run.
func FilterAnalyzerReports(analyzer *analysis.Analyzer, disabled map[string]bool, checkCount int) *analysis.Analyzer {
	wrapper := *analyzer
	run := analyzer.Run
	allDisabled := len(disabled) == checkCount
	wrapper.Run = func(pass *analysis.Pass) (any, error) {
		if allDisabled {
			return nil, nil
		}
		report := pass.Report
		pass.Report = func(diagnostic analysis.Diagnostic) {
			if disabled[diagnostic.Category] {
				analysisTrace.EmitDiagnostic(pass, analysisTrace.DiagnosticEvent{
					Analyzer: analyzer.Name, Phase: analysisTrace.PhaseDecision, Reason: ReportingDisabled.String(),
					Outcome: analysisTrace.OutcomeAccepted, Diagnostic: diagnostic,
				})
				return
			}
			analysisTrace.EmitDiagnostic(pass, analysisTrace.DiagnosticEvent{
				Analyzer: analyzer.Name, Phase: analysisTrace.PhaseDecision, Reason: ReportingEmitted.String(),
				Outcome: analysisTrace.OutcomeRejected, Diagnostic: diagnostic,
			})
			report(diagnostic)
		}
		defer func() { pass.Report = report }()
		return run(pass)
	}
	return &wrapper
}
