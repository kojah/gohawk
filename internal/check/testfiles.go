package check

import (
	"flag"
	"go/token"
	"strings"

	"golang.org/x/tools/go/analysis"
)

// Test files are excluded from every diagnostic by default. Fixture files,
// table-driven tests, and intentionally orphaned helper processes produce a
// steady stream of reports that are true by the letter of a policy and rarely
// worth acting on, and they dominated review effort in the repository audit.
// Analyzers whose subject is the test itself opt back in through the registry;
// everything else can be re-enabled with -gohawk-include-tests.

var includeTests bool

// RegisterFlags adds the test-file option to the analysis driver's flag set.
func RegisterFlags(flags *flag.FlagSet) {
	flags.BoolVar(&includeTests, "gohawk-include-tests", false, "report diagnostics in _test.go files for analyzers that do not target tests")
}

// IncludeTests reports whether diagnostics in test files are wanted from
// analyzers that do not target tests.
func IncludeTests() bool {
	return includeTests
}

// TestFilePosition reports whether position lies in a _test.go file.
func TestFilePosition(pass *analysis.Pass, position token.Pos) bool {
	return strings.HasSuffix(pass.Fset.Position(position).Filename, "_test.go")
}
