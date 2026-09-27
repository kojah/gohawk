package check

import (
	"go/token"
	"strings"

	"golang.org/x/tools/go/analysis"
)

// Test code is outside gohawk's subject: it is neither analyzed nor reported.
// syntax.AnalyzeFile owns the decision; the registry repeats it only as a
// backstop for a diagnostic whose position lands in a test file.

// TestFilePosition reports whether position lies in a _test.go file.
func TestFilePosition(pass *analysis.Pass, position token.Pos) bool {
	return strings.HasSuffix(pass.Fset.Position(position).Filename, "_test.go")
}
