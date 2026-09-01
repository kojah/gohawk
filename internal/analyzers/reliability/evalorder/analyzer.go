// Package evalorder implements the evalorder gohawk analyzer.
package evalorder

import (
	"go/ast"
	"go/token"
	"go/types"

	"github.com/kojah/gohawk/analysisutil"
	analysisTrace "github.com/kojah/gohawk/analysisutil/trace"
	"github.com/kojah/gohawk/internal/analyzerbase"
	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/analysis/passes/inspect"
	"golang.org/x/tools/go/ast/inspector"
)

// Analyzer returns this package's configured Go analysis pass.
func Analyzer() *analysis.Analyzer {
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
				object := pass.TypesInfo.ObjectOf(identifier)
				if !ok || !earlierObjects[object] || !callMutatesArgument(pass, call, argumentIndex) ||
					disjointFieldMutation(pass, expressions[:laterIndex], call, argumentIndex, object) {
					continue
				}
				analyzerbase.Reportf(pass, analyzerbase.CheckEvaluationOrder, address.Pos(), "later operand may mutate %s after its earlier value was evaluated", identifier.Name)
			}
			return true
		})
	}
}

func disjointFieldMutation(pass *analysis.Pass, earlier []ast.Expr, call *ast.CallExpr, argumentIndex int, object types.Object) bool {
	function := calledFunctionObject(pass, call)
	if function == nil || function.Pkg() != pass.Pkg {
		return false
	}
	var declaration *ast.FuncDecl
	for _, file := range pass.Files {
		for _, candidate := range file.Decls {
			declared, ok := candidate.(*ast.FuncDecl)
			if ok && pass.TypesInfo.Defs[declared.Name] == function {
				declaration = declared
				break
			}
		}
	}
	if declaration == nil {
		return false
	}
	parameter := functionParameter(pass, declaration, argumentIndex)
	if parameter == nil {
		return false
	}
	earlierFields := map[types.Object]bool{}
	for _, expression := range earlier {
		selector, ok := unparenthesized(expression).(*ast.SelectorExpr)
		if !ok || !analysisutil.ExpressionUsesObject(pass, selector.X, object) {
			if analysisutil.ExpressionUsesObject(pass, expression, object) {
				return false
			}
			continue
		}
		earlierFields[pass.TypesInfo.ObjectOf(selector.Sel)] = true
	}
	if len(earlierFields) == 0 {
		return false
	}
	mutatedFields := map[types.Object]bool{}
	wholeMutation := false
	ast.Inspect(declaration.Body, func(node ast.Node) bool {
		var targets []ast.Expr
		switch candidate := node.(type) {
		case *ast.AssignStmt:
			targets = candidate.Lhs
		case *ast.IncDecStmt:
			targets = []ast.Expr{candidate.X}
		default:
			return true
		}
		for _, target := range targets {
			selector, ok := unparenthesized(target).(*ast.SelectorExpr)
			if ok && analysisutil.ExpressionUsesObject(pass, selector.X, parameter) {
				mutatedFields[pass.TypesInfo.ObjectOf(selector.Sel)] = true
			} else if writesThroughObject(pass, target, parameter) {
				wholeMutation = true
			}
		}
		return true
	})
	if wholeMutation || len(mutatedFields) == 0 {
		return false
	}
	for field := range earlierFields {
		if mutatedFields[field] {
			return false
		}
	}
	if analysisTrace.Enabled("evalorder", string(analyzerbase.CheckEvaluationOrder)) {
		analysisTrace.Emit(pass, analysisTrace.Event{Analyzer: "evalorder", Check: string(analyzerbase.CheckEvaluationOrder), Phase: "evidence", Reason: "disjoint-field-mutation", Outcome: analysisTrace.OutcomeAccepted, Pos: call.Pos()})
	}
	return true
}

func unparenthesized(expression ast.Expr) ast.Expr {
	for {
		parenthesized, ok := expression.(*ast.ParenExpr)
		if !ok {
			return expression
		}
		expression = parenthesized.X
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
		return analysisutil.ExpressionUsesObject(pass, candidate.X, parameter)
	case *ast.SelectorExpr:
		return analysisutil.ExpressionUsesObject(pass, candidate.X, parameter)
	case *ast.IndexExpr:
		return analysisutil.ExpressionUsesObject(pass, candidate.X, parameter)
	case *ast.ParenExpr:
		return writesThroughObject(pass, candidate.X, parameter)
	default:
		return false
	}
}
