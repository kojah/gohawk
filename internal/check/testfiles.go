package check

import (
	"flag"
	"go/token"
	"strings"

	"github.com/kojah/gohawk/internal/syntax"
	"golang.org/x/tools/go/analysis"
)

// Test code is outside gohawk's subject: it is neither analyzed nor reported
// unless -gohawk-include-tests is set. syntax.AnalyzeFile owns the decision;
// the registry repeats it only as a backstop for a diagnostic whose position
// lands in a test file.

// RegisterFlags adds the test-file option to the analysis driver's flag set.
func RegisterFlags(flags *flag.FlagSet) {
	syntax.RegisterTestFlag(flags)
}

// IncludeTests reports whether test files are analyzed and reported.
func IncludeTests() bool {
	return syntax.IncludeTestFiles()
}

// TestFilePosition reports whether position lies in a _test.go file.
func TestFilePosition(pass *analysis.Pass, position token.Pos) bool {
	return strings.HasSuffix(pass.Fset.Position(position).Filename, "_test.go")
}
