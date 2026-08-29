package general

import (
	"go/ast"
	"go/token"
	"go/types"

	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/analysis/passes/inspect"
	"golang.org/x/tools/go/ast/inspector"
)

type syncMapLoad struct {
	receiver ast.Expr
	key      ast.Expr
	value    types.Object
	ok       types.Object
}

func syncMapAtomicityAnalyzer() *analysis.Analyzer {
	return &analysis.Analyzer{
		Name:     "syncmapatomicity",
		Doc:      "checks non-atomic sync.Map load-and-delete claims",
		Requires: []*analysis.Analyzer{inspect.Analyzer},
		Run:      runSyncMapAtomicity,
	}
}

func runSyncMapAtomicity(pass *analysis.Pass) (any, error) {
	in := pass.ResultOf[inspect.Analyzer].(*inspector.Inspector)
	in.Preorder([]ast.Node{(*ast.FuncDecl)(nil)}, func(node ast.Node) {
		function := node.(*ast.FuncDecl)
		if function.Body == nil || bodyUsesMutex(function.Body) {
			return
		}
		ast.Inspect(function.Body, func(candidate ast.Node) bool {
			if _, nested := candidate.(*ast.FuncLit); nested {
				return false
			}
			block, ok := candidate.(*ast.BlockStmt)
			if ok {
				reportSyncMapClaims(pass, block)
			}
			return true
		})
	})
	return nil, nil
}

func reportSyncMapClaims(pass *analysis.Pass, block *ast.BlockStmt) {
	for index, statement := range block.List {
		if conditional, ok := statement.(*ast.IfStmt); ok {
			if load, ok := syncMapLoadAssignment(pass, conditional.Init); ok {
				reportConditionalSyncMapClaim(pass, conditional, load)
			}
		}
		if index+1 >= len(block.List) {
			continue
		}
		load, ok := syncMapLoadAssignment(pass, statement)
		if !ok {
			continue
		}
		conditional, ok := block.List[index+1].(*ast.IfStmt)
		if ok {
			reportConditionalSyncMapClaim(pass, conditional, load)
		}
	}
}

func syncMapLoadAssignment(pass *analysis.Pass, statement ast.Stmt) (syncMapLoad, bool) {
	assignment, ok := statement.(*ast.AssignStmt)
	if !ok || len(assignment.Lhs) != 2 || len(assignment.Rhs) != 1 {
		return syncMapLoad{}, false
	}
	call, ok := assignment.Rhs[0].(*ast.CallExpr)
	if !ok || len(call.Args) != 1 || !syncMapMethod(pass, call, "Load") {
		return syncMapLoad{}, false
	}
	selector := call.Fun.(*ast.SelectorExpr)
	value, valueOK := assignment.Lhs[0].(*ast.Ident)
	okIdentifier, okOK := assignment.Lhs[1].(*ast.Ident)
	if !valueOK || !okOK {
		return syncMapLoad{}, false
	}
	return syncMapLoad{
		receiver: selector.X,
		key:      call.Args[0],
		value:    pass.TypesInfo.ObjectOf(value),
		ok:       pass.TypesInfo.ObjectOf(okIdentifier),
	}, true
}

func reportConditionalSyncMapClaim(pass *analysis.Pass, conditional *ast.IfStmt, load syncMapLoad) {
	if load.value == nil || load.ok == nil || !conditionRequiresTrue(pass, conditional.Cond, load.ok) || len(conditional.Body.List) == 0 {
		return
	}
	deleteCall, ok := expressionStatementCall(conditional.Body.List[0])
	if !ok || len(deleteCall.Args) != 1 || !syncMapMethod(pass, deleteCall, "Delete") {
		return
	}
	selector := deleteCall.Fun.(*ast.SelectorExpr)
	if !sameExpression(pass, selector.X, load.receiver) || !sameExpression(pass, deleteCall.Args[0], load.key) {
		return
	}
	usesValue := false
	for _, statement := range conditional.Body.List[1:] {
		usesValue = usesValue || expressionUsesObject(pass, statement, load.value)
	}
	if usesValue {
		reportf(pass, checkSyncMapNonAtomicClaim, deleteCall.Pos(), "sync.Map Load and Delete do not atomically claim the value")
	}
}

func conditionRequiresTrue(pass *analysis.Pass, expression ast.Expr, object types.Object) bool {
	switch candidate := expression.(type) {
	case *ast.Ident:
		return pass.TypesInfo.ObjectOf(candidate) == object
	case *ast.ParenExpr:
		return conditionRequiresTrue(pass, candidate.X, object)
	case *ast.BinaryExpr:
		if candidate.Op != token.EQL {
			return false
		}
		identifier, ok := candidate.X.(*ast.Ident)
		truth, truthOK := candidate.Y.(*ast.Ident)
		return ok && truthOK && pass.TypesInfo.ObjectOf(identifier) == object && truth.Name == "true"
	default:
		return false
	}
}

func expressionStatementCall(statement ast.Stmt) (*ast.CallExpr, bool) {
	expression, ok := statement.(*ast.ExprStmt)
	if !ok {
		return nil, false
	}
	call, ok := expression.X.(*ast.CallExpr)
	return call, ok
}

func syncMapMethod(pass *analysis.Pass, call *ast.CallExpr, name string) bool {
	selector, ok := call.Fun.(*ast.SelectorExpr)
	if !ok || selector.Sel.Name != name {
		return false
	}
	function, ok := pass.TypesInfo.ObjectOf(selector.Sel).(*types.Func)
	return ok && function.Pkg() != nil && function.Pkg().Path() == "sync"
}

func bodyUsesMutex(body *ast.BlockStmt) bool {
	usesMutex := false
	ast.Inspect(body, func(node ast.Node) bool {
		if usesMutex {
			return false
		}
		call, ok := node.(*ast.CallExpr)
		if !ok {
			return true
		}
		selector, ok := call.Fun.(*ast.SelectorExpr)
		if ok && (selector.Sel.Name == "Lock" || selector.Sel.Name == "RLock") {
			usesMutex = true
			return false
		}
		return true
	})
	return usesMutex
}
