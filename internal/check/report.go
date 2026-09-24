package check

import (
	"fmt"
	"go/token"
	"strings"

	"github.com/kojah/gohawk/internal/syntax"
	"github.com/kojah/gohawk/internal/trace"

	"golang.org/x/tools/go/analysis"
)

// Reportf reports a diagnostic with a precise source range.
func Reportf(pass *analysis.Pass, id ID, position token.Pos, format string, args ...any) {
	source := syntax.SourceRange(pass, position)
	Report(pass, id, analysis.Diagnostic{
		Pos:     source.Pos(),
		End:     source.End(),
		Message: fmt.Sprintf(format, args...),
	})
}

// Report associates diagnostic with id before reporting it.
func Report(pass *analysis.Pass, id ID, diagnostic analysis.Diagnostic) {
	diagnostic.Category = string(id)
	analyzer, _, _ := strings.Cut(string(id), "/")
	trace.EmitDiagnostic(pass, trace.DiagnosticEvent{
		Analyzer: analyzer, Phase: "candidate", Reason: ReportingCandidate.String(), Outcome: trace.OutcomeObserved, Diagnostic: diagnostic,
	})
	pass.Report(diagnostic)
}

// BufferReports returns a shallow pass copy that collects diagnostics and a
// commit function that publishes them once. Abandoning commit discards them.
// This lets a bounded proof withhold partial results without bypassing Report's
// category and trace handling. The caller must finish reporting before commit;
// neither the buffer nor its commit function is safe for concurrent use.
func BufferReports(pass *analysis.Pass) (*analysis.Pass, func()) {
	buffered := *pass
	var diagnostics []analysis.Diagnostic
	buffered.Report = func(diagnostic analysis.Diagnostic) { diagnostics = append(diagnostics, diagnostic) }
	return &buffered, func() {
		for _, diagnostic := range diagnostics {
			pass.Report(diagnostic)
		}
		diagnostics = nil
	}
}
