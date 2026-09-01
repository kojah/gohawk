package determinism

import (
	"go/ast"
	"go/types"

	"github.com/kojah/gohawk/internal/analysisutil"

	"golang.org/x/tools/go/analysis"
)

func mapRangeReachesOrderedOutput(pass *analysis.Pass, function *ast.FuncDecl, block *ast.BlockStmt, index int, ranged *ast.RangeStmt) bool {
	variables := rangeObjects(pass, ranged)
	// Tie the range variables to an ordered return or sink before reporting.
	// Independent file creation, table-driven subtests, set construction, and
	// commutative reductions do not expose iteration order merely because the
	// surrounding function also returns or writes ordered data. Network Doctor's
	// site fixture is a representative independent-per-key loop:
	// https://github.com/heymaikol/network-doctor/blob/336bff5c1fff3f4ed7e703e218b093a9be6dabfe/cmd/docsite/verify_test.go#L12-L28
	if directRangeOutput(pass, function, ranged.Body, variables) && !singletonMapGuard(pass, block.List[:index], ranged.X) {
		return true
	}
	for accumulator := range orderedRangeAccumulators(pass, ranged.Body, variables) {
		if accumulatorObservedWithoutSort(pass, block.List[index+1:], accumulator) {
			return true
		}
	}
	return false
}

func rangeObjects(pass *analysis.Pass, ranged *ast.RangeStmt) map[types.Object]bool {
	result := make(map[types.Object]bool)
	for _, expression := range []ast.Expr{ranged.Key, ranged.Value} {
		identifier, ok := expression.(*ast.Ident)
		if !ok || identifier.Name == "_" {
			continue
		}
		if object := pass.TypesInfo.ObjectOf(identifier); object != nil {
			result[object] = true
		}
	}
	return result
}

func directRangeOutput(pass *analysis.Pass, function *ast.FuncDecl, body *ast.BlockStmt, variables map[types.Object]bool) bool {
	found := false
	ast.Inspect(body, func(node ast.Node) bool {
		if found {
			return false
		}
		switch typed := node.(type) {
		case *ast.FuncLit:
			return false
		case *ast.ReturnStmt:
			if !orderedFunctionResult(pass, function) {
				return true
			}
			for _, expression := range typed.Results {
				if expressionUsesAnyObject(pass, expression, variables) {
					found = true
					return false
				}
			}
		case *ast.CallExpr:
			if orderedSinkCall(pass, typed) && expressionUsesAnyObject(pass, typed, variables) {
				found = true
				return false
			}
		}
		return true
	})
	return found
}

func orderedRangeAccumulators(pass *analysis.Pass, body *ast.BlockStmt, variables map[types.Object]bool) map[types.Object]bool {
	result := make(map[types.Object]bool)
	ast.Inspect(body, func(node ast.Node) bool {
		switch typed := node.(type) {
		case *ast.FuncLit:
			return false
		case *ast.AssignStmt:
			recordOrderedAssignment(pass, typed, variables, result)
		case *ast.CallExpr:
			recordOrderedCall(pass, typed, variables, result)
		}
		return true
	})
	delete(result, nil)
	return result
}

func recordOrderedAssignment(
	pass *analysis.Pass,
	assignment *ast.AssignStmt,
	variables map[types.Object]bool,
	result map[types.Object]bool,
) {
	for index, right := range assignment.Rhs {
		if index >= len(assignment.Lhs) || !expressionUsesAnyObject(pass, right, variables) {
			continue
		}
		left, ok := assignment.Lhs[index].(*ast.Ident)
		if ok && orderedAccumulatorType(pass.TypesInfo.TypeOf(left)) {
			result[pass.TypesInfo.ObjectOf(left)] = true
		}
	}
	if len(assignment.Lhs) != 1 || len(assignment.Rhs) != 1 {
		return
	}
	left, leftOK := assignment.Lhs[0].(*ast.Ident)
	appendCall, appendOK := assignment.Rhs[0].(*ast.CallExpr)
	if leftOK && appendOK && appendedRangeValue(pass, appendCall, variables) &&
		len(appendCall.Args) > 0 && analysisutil.SameExpression(pass, left, appendCall.Args[0]) {
		result[pass.TypesInfo.ObjectOf(left)] = true
	}
}

func recordOrderedCall(
	pass *analysis.Pass,
	call *ast.CallExpr,
	variables map[types.Object]bool,
	result map[types.Object]bool,
) {
	selector, ok := call.Fun.(*ast.SelectorExpr)
	if !ok {
		return
	}
	receiver, ok := selectorExpressionObject(pass, selector)
	if !ok || !orderedAccumulatorType(receiver.Type()) || !writeMethod(selector.Sel.Name) {
		return
	}
	for _, argument := range call.Args {
		if expressionUsesAnyObject(pass, argument, variables) {
			result[receiver] = true
		}
	}
}

func appendedRangeValue(pass *analysis.Pass, call *ast.CallExpr, variables map[types.Object]bool) bool {
	if !analysisutil.IsCallTo(pass, call, analysisutil.Builtin("append")) || len(call.Args) < 2 {
		return false
	}
	for _, argument := range call.Args[1:] {
		if expressionUsesAnyObject(pass, argument, variables) {
			return true
		}
	}
	return false
}

func orderedAccumulatorType(value types.Type) bool {
	if value == nil {
		return false
	}
	switch underlying := value.Underlying().(type) {
	case *types.Array, *types.Slice:
		return true
	case *types.Basic:
		return underlying.Kind() == types.String
	}
	return analysisutil.NamedType(value, "strings", "Builder") || analysisutil.NamedType(value, "bytes", "Buffer")
}
