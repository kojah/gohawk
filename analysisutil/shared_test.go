package analysisutil

import (
	"go/ast"
	"go/parser"
	"go/token"
	"testing"

	"golang.org/x/tools/go/analysis"
)

func TestSourceRangeRecoversCallFromLeftParenthesis(t *testing.T) {
	files := token.NewFileSet()
	file, err := parser.ParseFile(files, "example.go", "package p\nfunc f() { target(1) }\n", 0)
	if err != nil {
		t.Fatal(err)
	}
	var call *ast.CallExpr
	ast.Inspect(file, func(node ast.Node) bool {
		if typed, ok := node.(*ast.CallExpr); ok {
			call = typed
		}
		return true
	})
	if call == nil {
		t.Fatal("call expression not found")
	}
	rangeAtCall := SourceRange(&analysis.Pass{Fset: files, Files: []*ast.File{file}}, call.Lparen)
	if rangeAtCall.Pos() != call.Pos() || rangeAtCall.End() != call.End() {
		t.Fatalf("range = %v..%v, want %v..%v", rangeAtCall.Pos(), rangeAtCall.End(), call.Pos(), call.End())
	}
}
