// Package evalorder implements the evalorder gohawk analyzer.
package evalorder

import (
	"go/ast"
	"go/token"
	"go/types"

	"github.com/kojah/gohawk/internal/analysisutil"
	"github.com/kojah/gohawk/internal/check"
	analysisTrace "github.com/kojah/gohawk/internal/trace"
	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/analysis/passes/inspect"
	"golang.org/x/tools/go/ast/inspector"
)

var knownArgumentMutators = []analysisutil.Symbol{
	analysisutil.PackageFunction("encoding/json", "Unmarshal"),
	analysisutil.PackageFunction("encoding/xml", "Unmarshal"),
}

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
				check.Reportf(pass, check.EvaluationOrder, address.Pos(), "later operand may mutate %s after its earlier value was evaluated", identifier.Name)
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
	declaration := localFunctionDeclaration(pass, function)
	if declaration == nil {
		return false
	}
	parameter := analysisutil.FunctionParameterObject(pass, declaration, argumentIndex)
	if parameter == nil {
		return false
	}
	earlierFields, fieldsOnly := selectedFields(pass, earlier, object)
	if !fieldsOnly || len(earlierFields) == 0 {
		return false
	}
	mutatedFields, wholeMutation := functionMutatedFields(pass, declaration, parameter)
	if wholeMutation || len(mutatedFields) == 0 {
		return false
	}
	for field := range earlierFields {
		if mutatedFields[field] {
			return false
		}
	}
	if analysisTrace.Enabled("evalorder", string(check.EvaluationOrder)) {
		analysisTrace.Emit(
			pass,
			analysisTrace.Event{
				Analyzer: "evalorder",
				Check:    string(check.EvaluationOrder),
				Phase:    "evidence",
				Reason:   "disjoint-field-mutation",
				Outcome:  analysisTrace.OutcomeAccepted,
				Pos:      call.Pos(),
			},
		)
	}
	return true
}

func localFunctionDeclaration(pass *analysis.Pass, function *types.Func) *ast.FuncDecl {
	for _, file := range pass.Files {
		for _, candidate := range file.Decls {
			declaration, ok := candidate.(*ast.FuncDecl)
			if ok && pass.TypesInfo.Defs[declaration.Name] == function {
				return declaration
			}
		}
	}
	return nil
}

func selectedFields(pass *analysis.Pass, expressions []ast.Expr, object types.Object) (map[types.Object]bool, bool) {
	fields := map[types.Object]bool{}
	for _, expression := range expressions {
		selector, ok := analysisutil.Unparen(expression).(*ast.SelectorExpr)
		if ok && analysisutil.ExpressionUsesObject(pass, selector.X, object) {
			fields[pass.TypesInfo.ObjectOf(selector.Sel)] = true
			continue
		}
		if analysisutil.ExpressionUsesObject(pass, expression, object) {
			return nil, false
		}
	}
	return fields, true
}

func functionMutatedFields(
	pass *analysis.Pass,
	declaration *ast.FuncDecl,
	parameter types.Object,
) (map[types.Object]bool, bool) {
	fields := map[types.Object]bool{}
	wholeMutation := false
	ast.Inspect(declaration.Body, func(node ast.Node) bool {
		for _, target := range mutationTargets(node) {
			selector, ok := analysisutil.Unparen(target).(*ast.SelectorExpr)
			if ok && analysisutil.ExpressionUsesObject(pass, selector.X, parameter) {
				fields[pass.TypesInfo.ObjectOf(selector.Sel)] = true
			} else if writesThroughObject(pass, target, parameter) {
				wholeMutation = true
			}
		}
		return true
	})
	return fields, wholeMutation
}

func mutationTargets(node ast.Node) []ast.Expr {
	switch candidate := node.(type) {
	case *ast.AssignStmt:
		return candidate.Lhs
	case *ast.IncDecStmt:
		return []ast.Expr{candidate.X}
	default:
		return nil
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
			parameter := analysisutil.FunctionParameterObject(pass, declared, argumentIndex)
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
	return argumentIndex == 1 && analysisutil.IsCallToAny(pass, call, knownArgumentMutators...)
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
