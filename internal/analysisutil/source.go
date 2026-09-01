package analysisutil

import (
	"go/ast"
	"go/token"
	"os"
	"reflect"
	"strings"

	"golang.org/x/tools/go/analysis"
)

var analyzeProductionFilesInTestVariants = &analysis.Analyzer{
	Name:       "gohawk_test_variant_files",
	Doc:        "marks a driver whose test variant is its canonical package pass",
	ResultType: reflect.TypeFor[bool](),
	Run: func(*analysis.Pass) (any, error) {
		return true, nil
	},
}

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

// AnalyzeFile reports whether file is the canonical copy to analyze. Package-
// loading drivers commonly analyze production files once normally and again in
// a test variant. Other drivers expose the augmented test variant as their only
// pass, so every file in that pass is canonical.
func AnalyzeFile(pass *analysis.Pass, file *ast.File) bool {
	if GeneratedFile(file) {
		return false
	}
	_, driverUsesTestVariant := pass.ResultOf[analyzeProductionFilesInTestVariants]
	if !testVariant(pass) || vetToolInvocation(os.Args) || driverUsesTestVariant {
		return true
	}
	return strings.HasSuffix(pass.Fset.Position(file.Pos()).Filename, "_test.go")
}

// IncludeProductionFilesInTestVariants returns an analyzer copy configured for
// an embedding driver that does not also run the ordinary production package.
// Without this marker, production files in the driver's augmented test package
// would be mistaken for duplicate copies and skipped.
func IncludeProductionFilesInTestVariants(analyzer *analysis.Analyzer) *analysis.Analyzer {
	wrapper := *analyzer
	wrapper.Requires = append([]*analysis.Analyzer(nil), analyzer.Requires...)
	wrapper.Requires = append(wrapper.Requires, analyzeProductionFilesInTestVariants)
	return &wrapper
}

func testVariant(pass *analysis.Pass) bool {
	for _, file := range pass.Files {
		if strings.HasSuffix(pass.Fset.Position(file.Pos()).Filename, "_test.go") {
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
