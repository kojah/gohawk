package general

import (
	"go/ast"
	"go/token"
	"go/types"

	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/analysis/passes/inspect"
	"golang.org/x/tools/go/ast/inspector"
)

func evalOrderAnalyzer() *analysis.Analyzer {
	return &analysis.Analyzer{
		Name:     "evalorder",
		Doc:      "checks later operands that mutate values evaluated earlier",
		Requires: []*analysis.Analyzer{inspect.Analyzer},
		Run:      runEvalOrder,
	}
}

func runEvalOrder(pass *analysis.Pass) (any, error) {
	in := pass.ResultOf[inspect.Analyzer].(*inspector.Inspector)
	in.Preorder([]ast.Node{(*ast.ReturnStmt)(nil), (*ast.CallExpr)(nil)}, func(node ast.Node) {
		switch candidate := node.(type) {
		case *ast.ReturnStmt:
			reportEvaluationDependencies(pass, candidate.Results)
		case *ast.CallExpr:
			reportEvaluationDependencies(pass, candidate.Args)
		}
	})
	return nil, nil
}

func reportEvaluationDependencies(pass *analysis.Pass, expressions []ast.Expr) {
	for laterIndex := 1; laterIndex < len(expressions); laterIndex++ {
		earlierObjects := map[types.Object]bool{}
		for _, earlier := range expressions[:laterIndex] {
			ast.Inspect(earlier, func(node ast.Node) bool {
				identifier, ok := node.(*ast.Ident)
				if ok {
					if object := pass.TypesInfo.ObjectOf(identifier); object != nil {
						earlierObjects[object] = true
					}
				}
				return true
			})
		}
		ast.Inspect(expressions[laterIndex], func(node ast.Node) bool {
			call, ok := node.(*ast.CallExpr)
			if !ok {
				return true
			}
			for argumentIndex, argument := range call.Args {
				address, ok := argument.(*ast.UnaryExpr)
				if !ok || address.Op != token.AND {
					continue
				}
				identifier, ok := address.X.(*ast.Ident)
				if !ok || !earlierObjects[pass.TypesInfo.ObjectOf(identifier)] || !callMutatesArgument(pass, call, argumentIndex) {
					continue
				}
				reportf(pass, checkEvaluationOrder, address.Pos(), "later operand may mutate %s after its earlier value was evaluated", identifier.Name)
			}
			return true
		})
	}
}

func callMutatesArgument(pass *analysis.Pass, call *ast.CallExpr, argumentIndex int) bool {
	if knownMutatingArgument(pass, call, argumentIndex) {
		return true
	}
	function := calledFunctionObject(pass, call)
	if function == nil || function.Pkg() != pass.Pkg {
		return false
	}
	for _, file := range pass.Files {
		for _, declaration := range file.Decls {
			declared, ok := declaration.(*ast.FuncDecl)
			if !ok || pass.TypesInfo.Defs[declared.Name] != function {
				continue
			}
			parameter := functionParameter(pass, declared, argumentIndex)
			return parameter != nil && functionBodyMutates(pass, declared.Body, parameter)
		}
	}
	return false
}

func calledFunctionObject(pass *analysis.Pass, call *ast.CallExpr) *types.Func {
	switch function := call.Fun.(type) {
	case *ast.Ident:
		result, _ := pass.TypesInfo.ObjectOf(function).(*types.Func)
		return result
	case *ast.SelectorExpr:
		result, _ := pass.TypesInfo.ObjectOf(function.Sel).(*types.Func)
		return result
	default:
		return nil
	}
}

func knownMutatingArgument(pass *analysis.Pass, call *ast.CallExpr, argumentIndex int) bool {
	selector, ok := call.Fun.(*ast.SelectorExpr)
	if !ok {
		return false
	}
	function, ok := pass.TypesInfo.ObjectOf(selector.Sel).(*types.Func)
	if !ok || function.Pkg() == nil {
		return false
	}
	switch function.Pkg().Path() + "." + function.Name() {
	case "encoding/json.Unmarshal", "encoding/xml.Unmarshal":
		return argumentIndex == 1
	default:
		return false
	}
}

func functionParameter(pass *analysis.Pass, declaration *ast.FuncDecl, target int) types.Object {
	index := 0
	for _, field := range declaration.Type.Params.List {
		if len(field.Names) == 0 {
			index++
			continue
		}
		for _, name := range field.Names {
			if index == target {
				return pass.TypesInfo.Defs[name]
			}
			index++
		}
	}
	return nil
}

func functionBodyMutates(pass *analysis.Pass, body *ast.BlockStmt, parameter types.Object) bool {
	mutates := false
	ast.Inspect(body, func(node ast.Node) bool {
		if mutates {
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
			if writesThroughObject(pass, expression, parameter) {
				mutates = true
				return false
			}
		}
		return true
	})
	return mutates
}

func writesThroughObject(pass *analysis.Pass, expression ast.Expr, parameter types.Object) bool {
	switch candidate := expression.(type) {
	case *ast.StarExpr:
		return expressionUsesObject(pass, candidate.X, parameter)
	case *ast.SelectorExpr:
		return expressionUsesObject(pass, candidate.X, parameter)
	case *ast.IndexExpr:
		return expressionUsesObject(pass, candidate.X, parameter)
	case *ast.ParenExpr:
		return writesThroughObject(pass, candidate.X, parameter)
	default:
		return false
	}
}
