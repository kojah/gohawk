package syntax

import (
	"flag"
	"go/ast"
	"go/token"
	"os"
	"strings"

	"golang.org/x/tools/go/analysis"
)

// CanonicalTestVariant marks an analysis result from a driver whose augmented
// test package is the canonical pass for both production and test files.
// Source-selection prerequisite passes produce this fact without making
// syntax depend on the execution-infrastructure package that owns the pass.
type CanonicalTestVariant struct{}

type sourceRange struct {
	start token.Pos
	end   token.Pos
}

func (source sourceRange) Pos() token.Pos { return source.start }
func (source sourceRange) End() token.Pos { return source.end }

// SourceRange returns the smallest useful syntax range that starts at or
// contains position. SSA instructions often retain an operator position but
// no full range, so analyzers built on SSA use this helper to recover the
// corresponding source expression or statement.
func SourceRange(pass *analysis.Pass, position token.Pos) analysis.Range {
	var exact ast.Node
	var containing ast.Node
	for _, file := range pass.Files {
		if position < file.Pos() || position > file.End() {
			continue
		}
		ast.Inspect(file, func(node ast.Node) bool {
			if node == nil || position < node.Pos() || position >= node.End() {
				return false
			}
			if node.Pos() == position {
				if exact == nil || node.End() > exact.End() {
					exact = node
				}
			} else if containing == nil || node.End()-node.Pos() < containing.End()-containing.Pos() {
				containing = node
			}
			return true
		})
		break
	}
	if exact != nil {
		return sourceRange{start: exact.Pos(), end: exact.End()}
	}
	if containing != nil {
		return sourceRange{start: containing.Pos(), end: containing.End()}
	}
	return sourceRange{start: position, end: position}
}

// GeneratedFile reports whether file carries Go's generated-file marker.
func GeneratedFile(file *ast.File) bool {
	return ast.IsGenerated(file)
}

// includeTestFiles is the -gohawk-include-tests option. gohawk checks
// production code, so test files are skipped entirely by default: their
// functions are not analyzed, summarized as roots, or counted in a package's
// closed-world inventories, and their diagnostics are not reported.
var includeTestFiles bool

// RegisterTestFlag adds the test-file option to the driver's flag set.
func RegisterTestFlag(flags *flag.FlagSet) {
	flags.BoolVar(&includeTestFiles, "gohawk-include-tests", false, "analyze and report _test.go files")
}

// IncludeTestFiles reports whether test files are analyzed.
func IncludeTestFiles() bool {
	return includeTestFiles
}

// AnalyzeFile reports whether file is the canonical copy to analyze. Package-
// loading drivers commonly analyze production files once normally and again in
// a test variant. Other drivers expose the augmented test variant as their only
// pass, so every file in that pass is canonical. Test files are analyzed only
// when the test-file option is set.
func AnalyzeFile(pass *analysis.Pass, file *ast.File) bool {
	if GeneratedFile(file) {
		return false
	}
	if ExcludedTestFile(pass, file) {
		return false
	}
	isTest := testFile(pass, file)
	driverUsesTestVariant := canonicalTestVariant(pass)
	if !testVariant(pass) || vetToolInvocation(os.Args) || driverUsesTestVariant {
		return true
	}
	return isTest
}

// ExcludedTestFile reports whether file is a test file the option leaves out.
// Whole-package inventories, which read every file rather than only the
// canonical copy, use it to skip test code.
func ExcludedTestFile(pass *analysis.Pass, file *ast.File) bool {
	return !includeTestFiles && testFile(pass, file)
}

func testFile(pass *analysis.Pass, file *ast.File) bool {
	return strings.HasSuffix(pass.Fset.Position(file.Pos()).Filename, "_test.go")
}

func canonicalTestVariant(pass *analysis.Pass) bool {
	for _, result := range pass.ResultOf {
		if _, ok := result.(CanonicalTestVariant); ok {
			return true
		}
	}
	return false
}

func testVariant(pass *analysis.Pass) bool {
	for _, file := range pass.Files {
		if testFile(pass, file) {
			return true
		}
	}
	return false
}

func vetToolInvocation(arguments []string) bool {
	for _, argument := range arguments[1:] {
		if strings.HasSuffix(argument, ".cfg") {
			return true
		}
	}
	return false
}
