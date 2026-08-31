package reliability

import (
	"go/ast"
	"go/token"
	"go/types"
	"strings"

	"github.com/kojah/gohawk/analysisutil"

	"golang.org/x/tools/go/analysis"
)

func determinismAnalyzer() *analysis.Analyzer {
	return &analysis.Analyzer{Name: "determinism", Doc: "checks map iteration reaching ordered output without explicit sorting", Run: runDeterminism}
}

func runDeterminism(pass *analysis.Pass) (any, error) {
	for _, file := range pass.Files {
		if !analysisutil.AnalyzeFile(pass, file) {
			continue
		}
		for _, declaration := range file.Decls {
			function, ok := declaration.(*ast.FuncDecl)
			if ok && function.Body != nil {
				analyzeDeterministicBlock(pass, function, function.Body)
			}
		}
	}
	return nil, nil
}

func analyzeDeterministicBlock(pass *analysis.Pass, function *ast.FuncDecl, block *ast.BlockStmt) {
	for index, statement := range block.List {
		ranged, ok := statement.(*ast.RangeStmt)
		if ok && isMapType(pass.TypesInfo.TypeOf(ranged.X)) && mapRangeReachesOrderedOutput(pass, function, block, index, ranged) {
			report(pass, checkDeterministicMapOutput, analysis.Diagnostic{Pos: ranged.Pos(), End: ranged.X.End(), Message: "map iteration reaches ordered output without sorting"})
		}
		inspectNestedDeterministicBlocks(pass, function, statement)
	}
}

func inspectNestedDeterministicBlocks(pass *analysis.Pass, function *ast.FuncDecl, statement ast.Stmt) {
	switch typed := statement.(type) {
	case *ast.BlockStmt:
		analyzeDeterministicBlock(pass, function, typed)
	case *ast.LabeledStmt:
		analyzeDeterministicBlock(pass, function, &ast.BlockStmt{List: []ast.Stmt{typed.Stmt}})
	case *ast.IfStmt:
		analyzeDeterministicBlock(pass, function, typed.Body)
		if alternate, ok := typed.Else.(*ast.BlockStmt); ok {
			analyzeDeterministicBlock(pass, function, alternate)
		} else if alternate, ok := typed.Else.(*ast.IfStmt); ok {
			inspectNestedDeterministicBlocks(pass, function, alternate)
		}
	case *ast.ForStmt:
		analyzeDeterministicBlock(pass, function, typed.Body)
	case *ast.RangeStmt:
		analyzeDeterministicBlock(pass, function, typed.Body)
	case *ast.SwitchStmt:
		for _, clause := range typed.Body.List {
			if branch, ok := clause.(*ast.CaseClause); ok {
				analyzeDeterministicBlock(pass, function, &ast.BlockStmt{List: branch.Body})
			}
		}
	case *ast.TypeSwitchStmt:
		for _, clause := range typed.Body.List {
			if branch, ok := clause.(*ast.CaseClause); ok {
				analyzeDeterministicBlock(pass, function, &ast.BlockStmt{List: branch.Body})
			}
		}
	case *ast.SelectStmt:
		for _, clause := range typed.Body.List {
			if branch, ok := clause.(*ast.CommClause); ok {
				analyzeDeterministicBlock(pass, function, &ast.BlockStmt{List: branch.Body})
			}
		}
	}
}

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
				if leftOK && appendOK && appendedRangeValue(pass, appendCall, variables) && len(appendCall.Args) > 0 && analysisutil.SameExpression(pass, left, appendCall.Args[0]) {
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

func accumulatorObservedWithoutSort(pass *analysis.Pass, statements []ast.Stmt, accumulator types.Object) bool {
	return blockObservesAccumulatorWithoutSort(pass, statements, accumulator, false)
}

func blockObservesAccumulatorWithoutSort(pass *analysis.Pass, statements []ast.Stmt, accumulator types.Object, sorted bool) bool {
	for _, statement := range statements {
		if directSortOf(pass, statement, accumulator) {
			sorted = true
			continue
		}
		if statementMutatesObject(pass, statement, accumulator) {
			sorted = false
		}
		if nestedStatementObservesWithoutSort(pass, statement, accumulator, sorted) {
			return true
		}
		observed := false
		ast.Inspect(statement, func(node ast.Node) bool {
			if observed {
				return false
			}
			switch typed := node.(type) {
			case *ast.FuncLit, *ast.BlockStmt:
				return false
			case *ast.ReturnStmt:
				for _, expression := range typed.Results {
					if orderedObjectObservation(pass, expression, accumulator) {
						observed = true
						return false
					}
				}
			case *ast.CallExpr:
				if orderedSinkCall(pass, typed) && determinismUsesObject(pass, typed, accumulator) {
					observed = true
					return false
				}
			}
			return true
		})
		if observed && !sorted {
			return true
		}
	}
	return false
}

func nestedStatementObservesWithoutSort(pass *analysis.Pass, statement ast.Stmt, accumulator types.Object, sorted bool) bool {
	switch typed := statement.(type) {
	case *ast.LabeledStmt:
		return nestedStatementObservesWithoutSort(pass, typed.Stmt, accumulator, sorted)
	case *ast.IfStmt:
		if blockObservesAccumulatorWithoutSort(pass, typed.Body.List, accumulator, sorted) {
			return true
		}
		switch alternate := typed.Else.(type) {
		case *ast.BlockStmt:
			return blockObservesAccumulatorWithoutSort(pass, alternate.List, accumulator, sorted)
		case *ast.IfStmt:
			return nestedStatementObservesWithoutSort(pass, alternate, accumulator, sorted)
		}
	case *ast.ForStmt:
		return blockObservesAccumulatorWithoutSort(pass, typed.Body.List, accumulator, sorted)
	case *ast.RangeStmt:
		return blockObservesAccumulatorWithoutSort(pass, typed.Body.List, accumulator, sorted)
	case *ast.SwitchStmt:
		for _, clause := range typed.Body.List {
			if branch, ok := clause.(*ast.CaseClause); ok && blockObservesAccumulatorWithoutSort(pass, branch.Body, accumulator, sorted) {
				return true
			}
		}
	}
	return false
}

func orderedObjectObservation(pass *analysis.Pass, expression ast.Expr, object types.Object) bool {
	call, ok := expression.(*ast.CallExpr)
	if ok && orderInsensitiveCall(pass, call) {
		return false
	}
	return determinismUsesObject(pass, expression, object)
}

func orderInsensitiveCall(pass *analysis.Pass, call *ast.CallExpr) bool {
	for _, symbol := range []analysisutil.FunctionSymbol{
		{Package: "slices", Name: "Contains"}, {Package: "slices", Name: "ContainsFunc"},
		{Package: "slices", Name: "Equal"}, {Package: "slices", Name: "EqualFunc"},
	} {
		if analysisutil.IsPackageCall(pass, call, symbol) {
			return true
		}
	}
	return false
}

func directSortOf(pass *analysis.Pass, statement ast.Stmt, object types.Object) bool {
	expression, ok := statement.(*ast.ExprStmt)
	if !ok {
		return false
	}
	call, ok := expression.X.(*ast.CallExpr)
	if !ok || len(call.Args) == 0 || !determinismUsesObject(pass, call.Args[0], object) {
		return false
	}
	selector, ok := call.Fun.(*ast.SelectorExpr)
	if !ok {
		return false
	}
	identifier, ok := selector.X.(*ast.Ident)
	if !ok {
		return false
	}
	imported, ok := pass.TypesInfo.Uses[identifier].(*types.PkgName)
	return ok && (imported.Imported().Path() == "sort" || imported.Imported().Path() == "slices" && strings.HasPrefix(selector.Sel.Name, "Sort"))
}

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
		if lengthOfExpression(pass, comparison.X, ranged) && integerLiteral(comparison.Y, "1") || lengthOfExpression(pass, comparison.Y, ranged) && integerLiteral(comparison.X, "1") {
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
		{Package: "fmt", Name: "Print"}, {Package: "fmt", Name: "Printf"}, {Package: "fmt", Name: "Println"},
		{Package: "fmt", Name: "Fprint"}, {Package: "fmt", Name: "Fprintf"}, {Package: "fmt", Name: "Fprintln"},
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
