package ownership

import (
	"go/ast"
	"strings"

	"github.com/kojah/gohawk/analysisutil"

	"golang.org/x/tools/go/analysis"
)

func deferInLoopAnalyzer() *analysis.Analyzer {
	return &analysis.Analyzer{
		Name: "deferinloop",
		Doc:  "checks cleanup defers whose lifetime extends across loop iterations",
		Run:  runDeferInLoop,
	}
}

func runDeferInLoop(pass *analysis.Pass) (any, error) {
	for _, file := range pass.Files {
		if !analysisutil.AnalyzeFile(pass, file) {
			continue
		}
		ast.Inspect(file, func(node ast.Node) bool {
			var body *ast.BlockStmt
			switch loop := node.(type) {
			case *ast.ForStmt:
				body = loop.Body
			case *ast.RangeStmt:
				body = loop.Body
			default:
				return true
			}
			ast.Inspect(body, func(candidate ast.Node) bool {
				switch typed := candidate.(type) {
				case *ast.FuncLit, *ast.ForStmt, *ast.RangeStmt:
					return false
				case *ast.DeferStmt:
					if cleanupDefer(pass, body, typed.Call) {
						reportf(pass, checkDeferCleanupInLoop, typed.Pos(), "deferred cleanup runs after the loop instead of after this iteration")
					}
					return false
				default:
					return true
				}
			})
			return true
		})
	}
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
		if ok && (selector.Sel.Name == "Lock" || selector.Sel.Name == "RLock") && analysisutil.SameExpression(pass, selector.X, target) {
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
