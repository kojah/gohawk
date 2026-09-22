// Package oncepolicy implements the oncepolicy gohawk analyzer.
package oncepolicy

import (
	"go/ast"
	"go/types"
	"slices"

	"github.com/kojah/gohawk/internal/check"
	"github.com/kojah/gohawk/internal/syntax"
	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/analysis/passes/inspect"
	"golang.org/x/tools/go/ast/inspector"
)

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
		// A direct package initializer already runs once. Discarding its wrapper
		// is redundant, but cannot repeat the initialization. Do not extend this
		// to function literals stored by an initializer: callers may repeat them.
		// https://github.com/Control-D-Inc/ctrld/blob/37c33315591632c5f08df8062d1c77e07b3a465f/doh.go#L64-L67
		if slices.ContainsFunc(pass.TypesInfo.InitOrder, func(initializer *types.Initializer) bool {
			return syntax.Unparen(initializer.Rhs) == outer
		}) {
			return
		}
		if !syntax.IsCallToAny(
			pass,
			inner,
			syntax.PackageFunction("sync", "OnceFunc"),
			syntax.PackageFunction("sync", "OnceValue"),
			syntax.PackageFunction("sync", "OnceValues"),
		) {
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
