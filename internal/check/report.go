package check

import (
	"fmt"
	"go/token"
	"strings"

	"github.com/kojah/gohawk/internal/analysisutil"
	"github.com/kojah/gohawk/internal/trace"

	"golang.org/x/tools/go/analysis"
)

// Reportf reports a diagnostic with a precise source range.
func Reportf(pass *analysis.Pass, id ID, position token.Pos, format string, args ...any) {
	source := analysisutil.SourceRange(pass, position)
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
	trace.EmitDiagnostic(pass, analyzer, "candidate", "diagnostic-candidate", trace.OutcomeObserved, diagnostic)
	if len(diagnostic.SuggestedFixes) > 0 {
		trace.EmitDiagnostic(pass, analyzer, "fix", "suggested-fix-available", trace.OutcomeAccepted, diagnostic)
	}
	pass.Report(diagnostic)
}
