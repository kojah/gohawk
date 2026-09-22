package concurrentcapture

import (
	"go/ast"
	"go/parser"
	"go/token"
	"go/types"
	"testing"

	"github.com/kojah/gohawk/internal/analyzertest"
	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/analysis/analysistest"
)

func TestAnalyzer(t *testing.T) {
	analyzertest.Run(t, analysistest.TestData(), Analyzer(), "concurrentcapture")
}

func TestRangeIterationLanguageVersion(t *testing.T) {
	const source = `package p; func f(items []int) { for _, value := range items { go func() { value++ }() } }`
	for _, test := range []struct {
		name, packageVersion, fileVersion string
		want                              bool
	}{
		{"legacy", "go1.21", "", false},
		{"modern", "go1.22", "", true},
		{"legacy-file", "go1.27", "go1.21", false},
		{"modern-file", "go1.21", "go1.22", true},
	} {
		t.Run(test.name, func(t *testing.T) {
			files := token.NewFileSet()
			file, err := parser.ParseFile(files, "loop.go", source, 0)
			if err != nil {
				t.Fatal(err)
			}
			info := &types.Info{Defs: map[*ast.Ident]types.Object{}, Uses: map[*ast.Ident]types.Object{}, FileVersions: map[*ast.File]string{}}
			config := types.Config{GoVersion: test.packageVersion}
			pkg, err := config.Check("p", files, []*ast.File{file}, info)
			if err != nil {
				t.Fatal(err)
			}
			// Exercise the effective per-file version as well as the package
			// fallback, without depending on the host's build-tag selection.
			info.FileVersions[file] = test.fileVersion
			var loop *ast.RangeStmt
			var mutation ast.Expr
			ast.Inspect(file, func(node ast.Node) bool {
				if ranged, ok := node.(*ast.RangeStmt); ok {
					loop = ranged
				}
				if increment, ok := node.(*ast.IncDecStmt); ok {
					mutation = increment.X
				}
				return true
			})
			pass := &analysis.Pass{Pkg: pkg, Files: []*ast.File{file}, TypesInfo: info}
			if got := rangeIterationLocal(pass, loop, mutation); got != test.want {
				t.Errorf("rangeIterationLocal() = %v, want %v", got, test.want)
			}
		})
	}
}
