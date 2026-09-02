package determinism

import (
	"go/ast"
	"go/token"
	"go/types"

	"github.com/kojah/gohawk/internal/syntax"

	"golang.org/x/tools/go/analysis"
)

// Determinism contracts describe source-level operations that either expose
// sequence order or make it irrelevant. They keep API recognition and accepted
// guard patterns separate from propagation through an individual map range.

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

type singletonMapProof struct {
	proven bool
	reason string
}

func singletonMapGuard(pass *analysis.Pass, preceding []ast.Stmt, ranged *ast.RangeStmt, guardedMap ast.Expr) singletonMapProof {
	if mapMayMutateOrEscape(pass, ranged.Body, ranged.X) {
		return singletonMapProof{}
	}
	// Continuing past `len(m) != 1` proves that the selected entry is unique, so
	// returning from the range does not depend on map order. Network Doctor uses
	// this contract when unpacking its sole default-route family:
	// https://github.com/heymaikol/network-doctor/blob/336bff5c1fff3f4ed7e703e218b093a9be6dabfe/internal/simulation/hunt_generate.go#L1251-L1269
	for index, statement := range preceding {
		condition, ok := statement.(*ast.IfStmt)
		if !ok || !blockTerminates(condition.Body) {
			continue
		}
		comparison, ok := condition.Cond.(*ast.BinaryExpr)
		if !ok || comparison.Op != token.NEQ {
			continue
		}
		if (lengthOfExpression(pass, comparison.X, ranged.X) && integerOne(comparison.Y) ||
			lengthOfExpression(pass, comparison.Y, ranged.X) && integerOne(comparison.X)) &&
			!statementsMayMutateOrEscapeMap(pass, preceding[index+1:], ranged.X) {
			return singletonMapProof{proven: true, reason: "singleton-map-early-exit"}
		}
	}
	// A bare positive len guard also proves uniqueness, but only when the range
	// is the guarded block's first statement. This deliberately avoids proving
	// that intervening calls or aliases cannot mutate the map. Blnk selects the
	// only configured currency using this exact shape:
	// https://github.com/blnkfinance/blnk/blob/3356cb4c482c065e96624957a3f4be3ae9739c1a/reconciliation.go#L1156-L1161
	if len(preceding) == 0 && guardedMap != nil && sameMapExpression(pass, guardedMap, ranged.X) {
		return singletonMapProof{proven: true, reason: "singleton-map-positive-guard"}
	}
	return singletonMapProof{}
}

func statementsMayMutateOrEscapeMap(pass *analysis.Pass, statements []ast.Stmt, target ast.Expr) bool {
	for _, statement := range statements {
		if statementReassignsMap(pass, statement, target) || mapMayMutateOrEscape(pass, statement, target) {
			return true
		}
	}
	return false
}

func statementReassignsMap(pass *analysis.Pass, statement ast.Stmt, target ast.Expr) bool {
	reassigned := false
	ast.Inspect(statement, func(candidate ast.Node) bool {
		assignment, ok := candidate.(*ast.AssignStmt)
		if !ok {
			return true
		}
		for _, left := range assignment.Lhs {
			if sameMapExpression(pass, left, target) {
				reassigned = true
				return false
			}
		}
		return !reassigned
	})
	return reassigned
}

func mapMayMutateOrEscape(pass *analysis.Pass, node ast.Node, target ast.Expr) bool {
	unsafe := false
	ast.Inspect(node, func(candidate ast.Node) bool {
		if unsafe {
			return false
		}
		switch typed := candidate.(type) {
		case *ast.AssignStmt:
			for _, left := range typed.Lhs {
				if mapIndexMutation(pass, left) {
					unsafe = true
					return false
				}
			}
			for _, right := range typed.Rhs {
				if expressionCarriesMap(pass, right, target) {
					unsafe = true
					return false
				}
			}
		case *ast.IncDecStmt:
			unsafe = mapIndexMutation(pass, typed.X)
		case *ast.CallExpr:
			if mapCallMayMutateOrEscape(pass, typed, target) {
				unsafe = true
				return false
			}
		case *ast.FuncLit:
			// Capturing the exact map gives delayed code an alias whose mutation
			// cannot be excluded by this source-local guard proof.
			if nodeUsesMapExpression(pass, typed.Body, target) {
				unsafe = true
				return false
			}
			return false
		case *ast.ReturnStmt:
			for _, result := range typed.Results {
				if expressionCarriesMap(pass, result, target) {
					unsafe = true
					return false
				}
			}
		case *ast.SendStmt:
			unsafe = expressionCarriesMap(pass, typed.Value, target)
		}
		return !unsafe
	})
	return unsafe
}

func mapCallMayMutateOrEscape(pass *analysis.Pass, call *ast.CallExpr, target ast.Expr) bool {
	if syntax.IsCallTo(pass, call, syntax.Builtin("len")) {
		return false
	}
	if (syntax.IsCallTo(pass, call, syntax.Builtin("delete")) || syntax.IsCallTo(pass, call, syntax.Builtin("clear"))) &&
		len(call.Args) > 0 && isMapType(pass.TypesInfo.TypeOf(call.Args[0])) {
		// Removing entries cannot make a map that started with one entry
		// produce a second iteration.
		return false
	}
	for _, argument := range call.Args {
		if isMapType(pass.TypesInfo.TypeOf(argument)) || expressionCarriesMap(pass, argument, target) {
			return true
		}
	}
	selector, ok := syntax.Unparen(call.Fun).(*ast.SelectorExpr)
	return ok && (isMapType(pass.TypesInfo.TypeOf(selector.X)) || expressionCarriesMap(pass, selector.X, target))
}

func mapIndexMutation(pass *analysis.Pass, expression ast.Expr) bool {
	indexed, ok := syntax.Unparen(expression).(*ast.IndexExpr)
	return ok && isMapType(pass.TypesInfo.TypeOf(indexed.X))
}

func expressionCarriesMap(pass *analysis.Pass, expression, target ast.Expr) bool {
	expression = syntax.Unparen(expression)
	if sameMapExpression(pass, expression, target) {
		return true
	}
	switch typed := expression.(type) {
	case *ast.CompositeLit:
		for _, element := range typed.Elts {
			switch value := element.(type) {
			case *ast.KeyValueExpr:
				if expressionCarriesMap(pass, value.Value, target) {
					return true
				}
			case ast.Expr:
				if expressionCarriesMap(pass, value, target) {
					return true
				}
			}
		}
	case *ast.UnaryExpr:
		return expressionCarriesMap(pass, typed.X, target)
	}
	return false
}

func nodeUsesMapExpression(pass *analysis.Pass, node ast.Node, target ast.Expr) bool {
	used := false
	ast.Inspect(node, func(candidate ast.Node) bool {
		expression, ok := candidate.(ast.Expr)
		if ok && sameMapExpression(pass, expression, target) {
			used = true
			return false
		}
		return !used
	})
	return used
}

// Guard identity remains exact even though mutation checks consider every map
// value in the range body as a possible alias. This asymmetry is deliberate:
// only the guarded expression establishes cardinality, while an alias of that
// expression can still invalidate it by adding entries during iteration.
func sameMapExpression(pass *analysis.Pass, first, second ast.Expr) bool {
	return syntax.SameExpression(pass, syntax.Unparen(first), syntax.Unparen(second))
}

func positiveSingletonMap(pass *analysis.Pass, condition ast.Expr) ast.Expr {
	comparison, ok := syntax.Unparen(condition).(*ast.BinaryExpr)
	if !ok || comparison.Op != token.EQL {
		return nil
	}
	if lengthCall, ok := syntax.Unparen(comparison.X).(*ast.CallExpr); ok && integerOne(syntax.Unparen(comparison.Y)) {
		return singletonLengthArgument(pass, lengthCall)
	}
	if lengthCall, ok := syntax.Unparen(comparison.Y).(*ast.CallExpr); ok && integerOne(syntax.Unparen(comparison.X)) {
		return singletonLengthArgument(pass, lengthCall)
	}
	return nil
}

func singletonLengthArgument(pass *analysis.Pass, call *ast.CallExpr) ast.Expr {
	if len(call.Args) != 1 || !syntax.IsCallTo(pass, call, syntax.Builtin("len")) {
		return nil
	}
	return call.Args[0]
}

func lengthOfExpression(pass *analysis.Pass, expression, target ast.Expr) bool {
	call, ok := expression.(*ast.CallExpr)
	if !ok || len(call.Args) != 1 || !syntax.SameExpression(pass, call.Args[0], target) {
		return false
	}
	return syntax.IsCallTo(pass, call, syntax.Builtin("len"))
}

func integerOne(expression ast.Expr) bool {
	literal, ok := expression.(*ast.BasicLit)
	return ok && literal.Kind == token.INT && literal.Value == "1"
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
	for _, symbol := range []syntax.Symbol{
		syntax.PackageFunction("fmt", "Print"),
		syntax.PackageFunction("fmt", "Printf"),
		syntax.PackageFunction("fmt", "Println"),
		syntax.PackageFunction("fmt", "Fprint"),
		syntax.PackageFunction("fmt", "Fprintf"),
		syntax.PackageFunction("fmt", "Fprintln"),
	} {
		if syntax.IsCallTo(pass, call, symbol) {
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
