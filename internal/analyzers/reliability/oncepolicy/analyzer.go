// Package oncepolicy implements the oncepolicy gohawk analyzer.
package oncepolicy

import (
	"go/ast"

	"github.com/kojah/gohawk/internal/analysisutil"
	"github.com/kojah/gohawk/internal/check"
	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/analysis/passes/inspect"
	"golang.org/x/tools/go/ast/inspector"
)

var onceWrappers = []analysisutil.Symbol{
	analysisutil.PackageFunction("sync", "OnceFunc"),
	analysisutil.PackageFunction("sync", "OnceValue"),
	analysisutil.PackageFunction("sync", "OnceValues"),
}

// Analyzer returns this package's configured Go analysis pass.
func Analyzer() *analysis.Analyzer {
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
		if !analysisutil.IsCallToAny(pass, inner, onceWrappers...) {
			return
		}
		var name string
		switch function := inner.Fun.(type) {
		case *ast.Ident:
			name = function.Name
		case *ast.SelectorExpr:
			name = function.Sel.Name
		default:
			return
		}
		check.Reportf(pass, check.OnceDiscardedWrapper, outer.Pos(), "sync.%s wrapper is discarded after one call", name)
	})
	return nil, nil
}
