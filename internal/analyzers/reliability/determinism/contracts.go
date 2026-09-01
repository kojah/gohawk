package determinism

import (
	"go/ast"
	"go/token"
	"go/types"

	"github.com/kojah/gohawk/internal/analysisutil"

	"golang.org/x/tools/go/analysis"
)

func statementMutatesObject(pass *analysis.Pass, statement ast.Stmt, object types.Object) bool {
	mutated := false
	ast.Inspect(statement, func(node ast.Node) bool {
		assignment, ok := node.(*ast.AssignStmt)
		if !ok {
			return true
		}
		for _, left := range assignment.Lhs {
			if determinismUsesObject(pass, left, object) {
				mutated = true
				return false
			}
		}
		return true
	})
	return mutated
}

func singletonMapGuard(pass *analysis.Pass, preceding []ast.Stmt, ranged ast.Expr) bool {
	// Continuing past `len(m) != 1` proves that the selected entry is unique, so
	// returning from the range does not depend on map order. Network Doctor uses
	// this contract when unpacking its sole default-route family:
	// https://github.com/heymaikol/network-doctor/blob/336bff5c1fff3f4ed7e703e218b093a9be6dabfe/internal/simulation/hunt_generate.go#L1251-L1269
	for _, statement := range preceding {
		condition, ok := statement.(*ast.IfStmt)
		if !ok || !blockTerminates(condition.Body) {
			continue
		}
		comparison, ok := condition.Cond.(*ast.BinaryExpr)
		if !ok || comparison.Op != token.NEQ {
			continue
		}
		if lengthOfExpression(pass, comparison.X, ranged) && integerLiteral(comparison.Y, "1") ||
			lengthOfExpression(pass, comparison.Y, ranged) && integerLiteral(comparison.X, "1") {
			return true
		}
	}
	return false
}

func lengthOfExpression(pass *analysis.Pass, expression, target ast.Expr) bool {
	call, ok := expression.(*ast.CallExpr)
	if !ok || len(call.Args) != 1 || !analysisutil.SameExpression(pass, call.Args[0], target) {
		return false
	}
	function, ok := call.Fun.(*ast.Ident)
	builtin, builtinOK := pass.TypesInfo.Uses[function].(*types.Builtin)
	return ok && builtinOK && builtin.Name() == "len"
}

func integerLiteral(expression ast.Expr, value string) bool {
	literal, ok := expression.(*ast.BasicLit)
	return ok && literal.Kind == token.INT && literal.Value == value
}

func blockTerminates(block *ast.BlockStmt) bool {
	if block == nil || len(block.List) == 0 {
		return false
	}
	_, ok := block.List[len(block.List)-1].(*ast.ReturnStmt)
	return ok
}

func orderedFunctionResult(pass *analysis.Pass, function *ast.FuncDecl) bool {
	signature, ok := pass.TypesInfo.TypeOf(function.Name).(*types.Signature)
	if !ok {
		return false
	}
	for result := range signature.Results().Variables() {
		switch underlying := result.Type().Underlying().(type) {
		case *types.Array, *types.Slice:
			return true
		case *types.Basic:
			if underlying.Kind() == types.String {
				return true
			}
		}
	}
	return false
}

func orderedSinkCall(pass *analysis.Pass, call *ast.CallExpr) bool {
	for _, symbol := range []analysisutil.FunctionSymbol{
		{Package: "fmt", Name: "Print"},
		{Package: "fmt", Name: "Printf"},
		{Package: "fmt", Name: "Println"},
		{Package: "fmt", Name: "Fprint"},
		{Package: "fmt", Name: "Fprintf"},
		{Package: "fmt", Name: "Fprintln"},
	} {
		if analysisutil.IsPackageCall(pass, call, symbol) {
			return true
		}
	}
	selector, ok := call.Fun.(*ast.SelectorExpr)
	if !ok || !writeMethod(selector.Sel.Name) {
		return false
	}
	if identifier, ok := selector.X.(*ast.Ident); ok {
		if _, imported := pass.TypesInfo.Uses[identifier].(*types.PkgName); imported {
			return false
		}
	}
	return true
}

func writeMethod(name string) bool {
	return name == "Write" || name == "WriteString" || name == "WriteByte"
}

func selectorExpressionObject(pass *analysis.Pass, selector *ast.SelectorExpr) (types.Object, bool) {
	identifier, ok := selector.X.(*ast.Ident)
	if !ok {
		return nil, false
	}
	object := pass.TypesInfo.ObjectOf(identifier)
	return object, object != nil
}

func expressionUsesAnyObject(pass *analysis.Pass, node ast.Node, objects map[types.Object]bool) bool {
	used := false
	ast.Inspect(node, func(candidate ast.Node) bool {
		identifier, ok := candidate.(*ast.Ident)
		if ok && objects[pass.TypesInfo.ObjectOf(identifier)] {
			used = true
			return false
		}
		return true
	})
	return used
}

func determinismUsesObject(pass *analysis.Pass, node ast.Node, object types.Object) bool {
	return expressionUsesAnyObject(pass, node, map[types.Object]bool{object: true})
}

func isMapType(value types.Type) bool {
	if value == nil {
		return false
	}
	_, ok := value.Underlying().(*types.Map)
	return ok
}
