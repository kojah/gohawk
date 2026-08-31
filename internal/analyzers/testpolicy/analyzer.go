// Package testpolicy implements the testpolicy gohawk analyzer.
package testpolicy

import (
	"go/ast"
	"go/token"
	"go/types"
	"strings"

	"github.com/kojah/gohawk/analysisutil"
	ssautil "github.com/kojah/gohawk/analysisutil/ssa"
	"github.com/kojah/gohawk/internal/analyzerbase"

	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/analysis/passes/buildssa"
	"golang.org/x/tools/go/ssa"
)

// Analyzer returns this package's configured Go analysis pass.
func Analyzer() *analysis.Analyzer {
	return &analysis.Analyzer{
		Name:     "testpolicy",
		Doc:      "checks lifecycle ownership in test helpers",
		Requires: []*analysis.Analyzer{buildssa.Analyzer},
		Run:      runTestPolicy,
	}
}

func runTestPolicy(pass *analysis.Pass) (any, error) {
	functions, err := ssautil.SourceSSAFunctions(pass)
	if err != nil {
		return nil, err
	}
	for _, function := range functions {
		file := ssautil.FunctionFile(pass, function)
		_, declaration := function.Syntax().(*ast.FuncDecl)
		// Function literals that accept *testing.T are callbacks, not helpers:
		// t.Run bodies and table-driven builders should retain the caller's
		// location rather than marking themselves as reusable helper boundaries.
		if !declaration || file == nil || !strings.HasSuffix(pass.Fset.Position(file.Pos()).Filename, "_test.go") || testEntryPoint(function.Name()) {
			continue
		}
		handle := testingSSAParameter(function)
		if handle == nil {
			continue
		}
		if ssautil.UnownedReturnFromEntry(function, func(instruction ssa.Instruction) bool {
			common := ssautil.InstructionCall(instruction)
			return ssautil.CallName(common) == "Helper" && ssautil.AliasesValue(ssautil.CallReceiver(common), handle)
		}) {
			source := analysisutil.SourceRange(pass, function.Pos())
			analyzerbase.Report(pass, analyzerbase.CheckTestHelperMarker, analysis.Diagnostic{
				Pos:            source.Pos(),
				End:            source.End(),
				Message:        "test helper accepting " + handle.Name() + " must call " + handle.Name() + ".Helper() on every return path",
				SuggestedFixes: testHelperFix(pass, function, handle),
			})
		}
	}
	return nil, nil
}

func testHelperFix(pass *analysis.Pass, function *ssa.Function, handle *ssa.Parameter) []analysis.SuggestedFix {
	name := handle.Name()
	if name == "" || name == "_" || !token.IsIdentifier(name) || hasHelperCall(function, handle) {
		return nil
	}
	var body *ast.BlockStmt
	switch syntax := function.Syntax().(type) {
	case *ast.FuncDecl:
		body = syntax.Body
	case *ast.FuncLit:
		body = syntax.Body
	}
	if body == nil {
		return nil
	}
	position, newText := body.Rbrace, []byte("\n\t"+name+".Helper()\n")
	if file := pass.Fset.File(body.Lbrace); file != nil {
		braceLine := file.Line(body.Lbrace)
		if file.Line(body.Rbrace) > braceLine && braceLine < file.LineCount() {
			position = file.LineStart(braceLine + 1)
			newText = []byte("\t" + name + ".Helper()\n")
		}
	}
	return []analysis.SuggestedFix{{
		Message: "Call " + name + ".Helper() at function entry",
		TextEdits: []analysis.TextEdit{{
			Pos:     position,
			NewText: newText,
		}},
	}}
}

func hasHelperCall(function *ssa.Function, handle *ssa.Parameter) bool {
	for _, block := range function.Blocks {
		for _, instruction := range block.Instrs {
			common := ssautil.InstructionCall(instruction)
			if ssautil.CallName(common) == "Helper" && ssautil.AliasesValue(ssautil.CallReceiver(common), handle) {
				return true
			}
		}
	}
	return false
}

func testEntryPoint(name string) bool {
	return strings.HasPrefix(name, "Test") || strings.HasPrefix(name, "Benchmark") || strings.HasPrefix(name, "Fuzz")
}

func testingSSAParameter(function *ssa.Function) *ssa.Parameter {
	for _, parameter := range function.Params {
		if testingHandle(parameter.Type()) {
			return parameter
		}
	}
	return nil
}

func testingHandle(value types.Type) bool {
	pointer, ok := value.(*types.Pointer)
	if !ok {
		return false
	}
	named, ok := pointer.Elem().(*types.Named)
	return ok && named.Obj().Pkg() != nil && named.Obj().Pkg().Path() == "testing" && (named.Obj().Name() == "T" || named.Obj().Name() == "B")
}
