// Package concurrentcapture implements the concurrentcapture gohawk analyzer.
package concurrentcapture

import (
	"go/ast"
	"go/token"
	"go/types"

	"github.com/kojah/gohawk/internal/analyzerbase"
	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/analysis/passes/inspect"
	"golang.org/x/tools/go/ast/inspector"
)

// Analyzer returns this package's configured Go analysis pass.
func Analyzer() *analysis.Analyzer {
	return &analysis.Analyzer{
		Name:     "concurrentcapture",
		Doc:      "checks locals mutated by goroutines launched repeatedly",
		Requires: []*analysis.Analyzer{inspect.Analyzer},
		Run:      runConcurrentCapture,
	}
}

func runConcurrentCapture(pass *analysis.Pass) (any, error) {
	in := pass.ResultOf[inspect.Analyzer].(*inspector.Inspector)
	in.Preorder([]ast.Node{(*ast.ForStmt)(nil), (*ast.RangeStmt)(nil)}, func(node ast.Node) {
		var body *ast.BlockStmt
		switch loop := node.(type) {
		case *ast.ForStmt:
			body = loop.Body
		case *ast.RangeStmt:
			body = loop.Body
		}
		if body == nil || loopJoinsEachIteration(body) {
			return
		}
		inspectRepeatedLaunches(pass, body)
	})
	return nil, nil
}

func inspectRepeatedLaunches(pass *analysis.Pass, body *ast.BlockStmt) {
	ast.Inspect(body, func(node ast.Node) bool {
		switch candidate := node.(type) {
		case *ast.FuncLit, *ast.ForStmt, *ast.RangeStmt:
			return false
		case *ast.GoStmt:
			if closure := calledClosure(candidate.Call); closure != nil {
				reportCapturedMutations(pass, closure)
			}
			return false
		case *ast.CallExpr:
			selector, ok := candidate.Fun.(*ast.SelectorExpr)
			if !ok || selector.Sel.Name != "Go" || len(candidate.Args) == 0 {
				return true
			}
			if closure, ok := candidate.Args[0].(*ast.FuncLit); ok {
				reportCapturedMutations(pass, closure)
			}
			return false
		default:
			return true
		}
	})
}

func calledClosure(call *ast.CallExpr) *ast.FuncLit {
	if call == nil {
		return nil
	}
	closure, _ := call.Fun.(*ast.FuncLit)
	return closure
}

func reportCapturedMutations(pass *analysis.Pass, closure *ast.FuncLit) {
	if closureUsesLock(closure) {
		return
	}
	reported := map[types.Object]bool{}
	ast.Inspect(closure.Body, func(node ast.Node) bool {
		if nested, ok := node.(*ast.FuncLit); ok && nested != closure {
			return false
		}
		var expressions []ast.Expr
		switch candidate := node.(type) {
		case *ast.AssignStmt:
			expressions = candidate.Lhs
		case *ast.IncDecStmt:
			expressions = []ast.Expr{candidate.X}
		default:
			return true
		}
		for _, expression := range expressions {
			identifier := mutatedRoot(pass, expression)
			if identifier == nil {
				continue
			}
			object, ok := pass.TypesInfo.ObjectOf(identifier).(*types.Var)
			if !ok || object.Parent() == pass.Pkg.Scope() || object.Pos() >= closure.Pos() || reported[object] {
				continue
			}
			reported[object] = true
			analyzerbase.Reportf(pass, analyzerbase.CheckConcurrentCapture, identifier.Pos(), "captured local %s is mutated by goroutines launched repeatedly", identifier.Name)
		}
		return true
	})
}

func mutatedRoot(pass *analysis.Pass, expression ast.Expr) *ast.Ident {
	switch candidate := expression.(type) {
	case *ast.Ident:
		return candidate
	case *ast.IndexExpr:
		typeOf := pass.TypesInfo.TypeOf(candidate.X)
		if typeOf != nil {
			_, ok := typeOf.Underlying().(*types.Map)
			if !ok {
				return nil
			}
			return mutatedRoot(pass, candidate.X)
		}
		return nil
	case *ast.ParenExpr:
		return mutatedRoot(pass, candidate.X)
	default:
		return nil
	}
}

func closureUsesLock(closure *ast.FuncLit) bool {
	usesLock := false
	ast.Inspect(closure.Body, func(node ast.Node) bool {
		if nested, ok := node.(*ast.FuncLit); ok && nested != closure {
			return false
		}
		call, ok := node.(*ast.CallExpr)
		if !ok {
			return true
		}
		selector, ok := call.Fun.(*ast.SelectorExpr)
		if ok && (selector.Sel.Name == "Lock" || selector.Sel.Name == "RLock") {
			usesLock = true
			return false
		}
		return true
	})
	return usesLock
}

func loopJoinsEachIteration(body *ast.BlockStmt) bool {
	joined := false
	ast.Inspect(body, func(node ast.Node) bool {
		switch node.(type) {
		case *ast.FuncLit, *ast.ForStmt, *ast.RangeStmt:
			return false
		}
		switch candidate := node.(type) {
		case *ast.UnaryExpr:
			joined = joined || candidate.Op == token.ARROW
		case *ast.CallExpr:
			selector, ok := candidate.Fun.(*ast.SelectorExpr)
			joined = joined || ok && (selector.Sel.Name == "Wait" || selector.Sel.Name == "Join")
		}
		return !joined
	})
	return joined
}
