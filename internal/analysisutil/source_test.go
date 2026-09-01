package analysisutil

import (
	"go/ast"
	"go/parser"
	"go/token"
	"slices"
	"testing"

	"github.com/kojah/gohawk/internal/analysispasses/testvariant"

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

func TestIncludeProductionFilesInTestVariantsAddsMarkerPrerequisite(t *testing.T) {
	baseRequirement := &analysis.Analyzer{Name: "base", Doc: "base prerequisite", Run: func(*analysis.Pass) (any, error) { return nil, nil }}
	analyzer := &analysis.Analyzer{Name: "target", Doc: "target analyzer", Requires: []*analysis.Analyzer{baseRequirement}}

	wrapper := IncludeProductionFilesInTestVariants(analyzer)

	if wrapper == analyzer {
		t.Fatal("IncludeProductionFilesInTestVariants returned the original analyzer")
	}
	if !slices.Contains(wrapper.Requires, baseRequirement) || !slices.Contains(wrapper.Requires, testvariant.Analyzer) {
		t.Fatalf("wrapper requirements = %v, want original requirement and test-variant marker", wrapper.Requires)
	}
	if len(analyzer.Requires) != 1 || analyzer.Requires[0] != baseRequirement {
		t.Fatalf("original requirements changed: %v", analyzer.Requires)
	}
}
