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
			for index, right := range typed.Rhs {
				if index >= len(typed.Lhs) || !expressionUsesAnyObject(pass, right, variables) {
					continue
				}
				left, ok := typed.Lhs[index].(*ast.Ident)
				if ok && orderedAccumulatorType(pass.TypesInfo.TypeOf(left)) {
					result[pass.TypesInfo.ObjectOf(left)] = true
				}
			}
			if len(typed.Lhs) == 1 && len(typed.Rhs) == 1 {
				left, leftOK := typed.Lhs[0].(*ast.Ident)
				appendCall, appendOK := typed.Rhs[0].(*ast.CallExpr)
				if leftOK && appendOK &&
					appendedRangeValue(pass, appendCall, variables) &&
					len(appendCall.Args) > 0 &&
					analysisutil.SameExpression(pass, left, appendCall.Args[0]) {
					result[pass.TypesInfo.ObjectOf(left)] = true
				}
			}
		case *ast.CallExpr:
			selector, ok := typed.Fun.(*ast.SelectorExpr)
			if !ok {
				return true
			}
			receiver, receiverOK := selectorExpressionObject(pass, selector)
			if receiverOK && orderedAccumulatorType(receiver.Type()) && writeMethod(selector.Sel.Name) {
				for _, argument := range typed.Args {
					if expressionUsesAnyObject(pass, argument, variables) {
						result[receiver] = true
					}
				}
			}
		}
		return true
	})
	delete(result, nil)
	return result
}

func appendedRangeValue(pass *analysis.Pass, call *ast.CallExpr, variables map[types.Object]bool) bool {
	function, ok := call.Fun.(*ast.Ident)
	builtin, builtinOK := pass.TypesInfo.Uses[function].(*types.Builtin)
	if !ok || !builtinOK || builtin.Name() != "append" || len(call.Args) < 2 {
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
