package general

import (
	"go/ast"
	"go/types"

	"github.com/kojah/gohawk/analysisutil"

	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/analysis/passes/inspect"
	"golang.org/x/tools/go/ast/inspector"
)

func oncePolicyAnalyzer() *analysis.Analyzer {
	return &analysis.Analyzer{
		Name:     "oncepolicy",
		Doc:      "checks sync.Once function wrappers that are immediately discarded",
		Requires: []*analysis.Analyzer{inspect.Analyzer},
		Run:      runOncePolicy,
	}
}

func runOncePolicy(pass *analysis.Pass) (any, error) {
	in := pass.ResultOf[inspect.Analyzer].(*inspector.Inspector)
	in.Preorder([]ast.Node{(*ast.CallExpr)(nil)}, func(node ast.Node) {
		outer := node.(*ast.CallExpr)
		inner, ok := outer.Fun.(*ast.CallExpr)
		if !ok || len(outer.Args) != 0 {
			return
		}
		selector, ok := inner.Fun.(*ast.SelectorExpr)
		if !ok {
			return
		}
		function, ok := pass.TypesInfo.ObjectOf(selector.Sel).(*types.Func)
		if !ok || function.Pkg() == nil || function.Pkg().Path() != "sync" {
			return
		}
		switch function.Name() {
		case "OnceFunc", "OnceValue", "OnceValues":
			analysisutil.Reportf(pass, outer.Pos(), "sync.%s wrapper is discarded after one call", function.Name())
		}
	})
	return nil, nil
}
