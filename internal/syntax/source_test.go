package syntax

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

func TestVetToolInvocation(t *testing.T) {
	for _, test := range []struct {
		name      string
		arguments []string
		want      bool
	}{
		{name: "unitchecker config", arguments: []string{"gohawk", "-json", "/tmp/vet.cfg"}, want: true},
		{name: "standalone packages", arguments: []string{"gohawk", "-json", "./..."}},
	} {
		t.Run(test.name, func(t *testing.T) {
			if got := vetToolInvocation(test.arguments); got != test.want {
				t.Fatalf("vetToolInvocation(%v) = %t, want %t", test.arguments, got, test.want)
			}
		})
	}
}

func TestCanonicalTestVariantResult(t *testing.T) {
	marker := &analysis.Analyzer{Name: "marker", Doc: "test marker", Run: func(*analysis.Pass) (any, error) { return nil, nil }}
	pass := &analysis.Pass{ResultOf: map[*analysis.Analyzer]any{marker: CanonicalTestVariant{}}}
	if !canonicalTestVariant(pass) {
		t.Fatal("canonicalTestVariant did not recognize the typed source-selection result")
	}
	pass.ResultOf[marker] = true
	if canonicalTestVariant(pass) {
		t.Fatal("canonicalTestVariant accepted an unrelated prerequisite result")
	}
}

func TestAnalyzeFileSkipsTestFilesUnlessIncluded(t *testing.T) {
	files := token.NewFileSet()
	production, err := parser.ParseFile(files, "p.go", "package p\n", 0)
	if err != nil {
		t.Fatal(err)
	}
	test, err := parser.ParseFile(files, "p_test.go", "package p\n", 0)
	if err != nil {
		t.Fatal(err)
	}
	marker := &analysis.Analyzer{Name: "marker", Doc: "test marker", Run: func(*analysis.Pass) (any, error) { return nil, nil }}
	// The augmented test variant is the only pass this driver runs.
	pass := &analysis.Pass{
		Fset: files, Files: []*ast.File{production, test},
		ResultOf: map[*analysis.Analyzer]any{marker: CanonicalTestVariant{}},
	}
	if !AnalyzeFile(pass, production) || AnalyzeFile(pass, test) {
		t.Fatal("default analysis must keep production files and skip test files")
	}
	includeTestFiles = true
	t.Cleanup(func() { includeTestFiles = false })
	if !AnalyzeFile(pass, production) || !AnalyzeFile(pass, test) {
		t.Fatal("the test-file option must analyze both files")
	}
}
