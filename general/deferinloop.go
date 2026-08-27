package general

import (
	"go/ast"
	"strings"

	"github.com/kojah/gohawk/analysisutil"

	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/analysis/passes/inspect"
	"golang.org/x/tools/go/ast/inspector"
)

func deferInLoopAnalyzer() *analysis.Analyzer {
	return &analysis.Analyzer{
		Name:     "deferinloop",
		Doc:      "checks cleanup defers whose lifetime extends across loop iterations",
		Requires: []*analysis.Analyzer{inspect.Analyzer},
		Run:      runDeferInLoop,
	}
}

func runDeferInLoop(pass *analysis.Pass) (any, error) {
	in := pass.ResultOf[inspect.Analyzer].(*inspector.Inspector)
	in.Preorder([]ast.Node{(*ast.ForStmt)(nil), (*ast.RangeStmt)(nil)}, func(node ast.Node) {
		var body *ast.BlockStmt
		switch loop := node.(type) {
		case *ast.ForStmt:
			body = loop.Body
		case *ast.RangeStmt:
			body = loop.Body
		}
		ast.Inspect(body, func(candidate ast.Node) bool {
			switch typed := candidate.(type) {
			case *ast.FuncLit, *ast.ForStmt, *ast.RangeStmt:
				return false
			case *ast.DeferStmt:
				if cleanupDefer(pass, body, typed.Call) {
					analysisutil.Reportf(pass, typed.Pos(), "deferred cleanup runs after the loop instead of after this iteration")
				}
				return false
			default:
				return true
			}
		})
	})
	return nil, nil
}

func cleanupDefer(pass *analysis.Pass, body *ast.BlockStmt, call *ast.CallExpr) bool {
	if call == nil {
		return false
	}
	var name string
	var target ast.Expr
	switch function := call.Fun.(type) {
	case *ast.Ident:
		name = function.Name
		if name == "close" {
			return false
		}
		target = function
	case *ast.SelectorExpr:
		name = function.Sel.Name
		target = function.X
	}
	name = strings.ToLower(name)
	cleanup := false
	for _, fragment := range []string{"cancel", "cleanup", "close", "commit", "release", "rollback", "stop", "unlock"} {
		if strings.Contains(name, fragment) {
			cleanup = true
			break
		}
	}
	if !cleanup || target == nil {
		return false
	}
	if strings.Contains(name, "unlock") {
		return loopAcquiresTarget(pass, body, target)
	}
	root := expressionRoot(target)
	object := pass.TypesInfo.ObjectOf(root)
	return object != nil && object.Pos() >= body.Pos() && object.Pos() < body.End()
}

func loopAcquiresTarget(pass *analysis.Pass, body *ast.BlockStmt, target ast.Expr) bool {
	found := false
	ast.Inspect(body, func(node ast.Node) bool {
		if found {
			return false
		}
		call, ok := node.(*ast.CallExpr)
		if !ok {
			return true
		}
		selector, ok := call.Fun.(*ast.SelectorExpr)
		if ok && (selector.Sel.Name == "Lock" || selector.Sel.Name == "RLock") && sameExpression(pass, selector.X, target) {
			found = true
			return false
		}
		return true
	})
	return found
}

func expressionRoot(expression ast.Expr) *ast.Ident {
	switch candidate := expression.(type) {
	case *ast.Ident:
		return candidate
	case *ast.SelectorExpr:
		return expressionRoot(candidate.X)
	case *ast.IndexExpr:
		return expressionRoot(candidate.X)
	case *ast.ParenExpr:
		return expressionRoot(candidate.X)
	case *ast.StarExpr:
		return expressionRoot(candidate.X)
	default:
		return nil
	}
}

func sameExpression(pass *analysis.Pass, first, second ast.Expr) bool {
	switch left := first.(type) {
	case *ast.Ident:
		right, ok := second.(*ast.Ident)
		return ok && pass.TypesInfo.ObjectOf(left) == pass.TypesInfo.ObjectOf(right)
	case *ast.SelectorExpr:
		right, ok := second.(*ast.SelectorExpr)
		return ok && left.Sel.Name == right.Sel.Name && sameExpression(pass, left.X, right.X)
	case *ast.ParenExpr:
		right, ok := second.(*ast.ParenExpr)
		return ok && sameExpression(pass, left.X, right.X)
	case *ast.StarExpr:
		right, ok := second.(*ast.StarExpr)
		return ok && sameExpression(pass, left.X, right.X)
	case *ast.BasicLit:
		right, ok := second.(*ast.BasicLit)
		return ok && left.Kind == right.Kind && left.Value == right.Value
	default:
		return false
	}
}
