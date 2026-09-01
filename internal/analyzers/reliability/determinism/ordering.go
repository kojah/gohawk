package determinism

import (
	"go/ast"
	"go/types"
	"strings"

	"github.com/kojah/gohawk/internal/analysisutil"

	"golang.org/x/tools/go/analysis"
)

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
	for _, symbol := range []analysisutil.Symbol{
		analysisutil.PackageFunction("slices", "Contains"),
		analysisutil.PackageFunction("slices", "ContainsFunc"),
		analysisutil.PackageFunction("slices", "Equal"),
		analysisutil.PackageFunction("slices", "EqualFunc"),
	} {
		if analysisutil.IsCallTo(pass, call, symbol) {
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
	if !ok {
		return false
	}
	if directSortCall(pass, call, object) {
		return true
	}
	return localSortHelperCall(pass, call, object)
}

func directSortCall(pass *analysis.Pass, call *ast.CallExpr, object types.Object) bool {
	if len(call.Args) == 0 || !determinismUsesObject(pass, call.Args[0], object) {
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

func localSortHelperCall(pass *analysis.Pass, call *ast.CallExpr, object types.Object) bool {
	identifier, ok := call.Fun.(*ast.Ident)
	if !ok {
		return false
	}
	callee := pass.TypesInfo.ObjectOf(identifier)
	for argumentIndex, argument := range call.Args {
		if !determinismUsesObject(pass, argument, object) {
			continue
		}
		for _, file := range pass.Files {
			for _, declaration := range file.Decls {
				function, functionOK := declaration.(*ast.FuncDecl)
				if !functionOK || pass.TypesInfo.Defs[function.Name] != callee || function.Body == nil || len(function.Body.List) != 1 {
					continue
				}
				parameter := analysisutil.FunctionParameterObject(pass, function, argumentIndex)
				if parameter == nil {
					continue
				}
				expression, expressionOK := function.Body.List[0].(*ast.ExprStmt)
				if !expressionOK {
					continue
				}
				sortCall, callOK := expression.X.(*ast.CallExpr)
				if callOK && directSortCall(pass, sortCall, parameter) {
					// Requiring a single direct sort statement avoids treating helpers
					// with conditional or subsequent mutation as a stable boundary.
					return true
				}
			}
		}
	}
	return false
}
