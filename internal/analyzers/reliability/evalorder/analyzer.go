// Package evalorder implements the evalorder gohawk analyzer.
package evalorder

import (
	"go/ast"
	"go/token"
	"go/types"

	"github.com/kojah/gohawk/internal/check"
	"github.com/kojah/gohawk/internal/syntax"
	analysisTrace "github.com/kojah/gohawk/internal/trace"
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
	in.Nodes([]ast.Node{(*ast.File)(nil), (*ast.ReturnStmt)(nil), (*ast.CallExpr)(nil)}, func(node ast.Node, push bool) bool {
		if !push {
			return true
		}
		switch candidate := node.(type) {
		case *ast.File:
			// The standalone driver includes production syntax in both the ordinary package and its augmented test variant.
			// AnalyzeFile keeps that syntax canonical without dropping diagnostics that originate in _test.go files. This
			// duplicate was exposed by https://github.com/minio/madmin-go/blob/ef04ea3969c2177b22e13e9e61dfc4ddeccf3feb/user-commands.go#L1157-L1158.
			return syntax.AnalyzeFile(pass, candidate)
		case *ast.ReturnStmt:
			reportEvaluationDependencies(pass, candidate.Results)
		case *ast.CallExpr:
			reportEvaluationDependencies(pass, candidate.Args)
		}
		return true
	})
	return nil, nil
}

func reportEvaluationDependencies(pass *analysis.Pass, expressions []ast.Expr) {
	// Go evaluates operands left to right, so only a later call can invalidate a
	// value already captured by an earlier expression. Address-taking plus a
	// proved mutating callee supplies the required alias and write evidence.
	for laterIndex := 1; laterIndex < len(expressions); laterIndex++ {
		earlierObjects, stableAddresses := evaluatedObjectEvidence(pass, expressions[:laterIndex])
		walkEvaluatedExpression(expressions[laterIndex], func(node ast.Node) bool {
			call, ok := node.(*ast.CallExpr)
			if ok {
				reportCallEvaluationDependencies(pass, expressions[:laterIndex], call, earlierObjects, stableAddresses)
			}
			return true
		}, func(literal *ast.FuncLit) {
			traceDelayedFunctionBody(pass, literal)
		})
	}
}

func evaluatedObjectEvidence(pass *analysis.Pass, expressions []ast.Expr) (map[types.Object]bool, map[types.Object]token.Pos) {
	earlierObjects := map[types.Object]bool{}
	stableAddresses := map[types.Object]token.Pos{}
	for _, expression := range expressions {
		if object, position := stableAddressIdentity(pass, expression); object != nil {
			stableAddresses[object] = position
			continue
		}
		walkEvaluatedExpression(expression, func(node ast.Node) bool {
			identifier, ok := node.(*ast.Ident)
			if ok {
				if object := pass.TypesInfo.ObjectOf(identifier); object != nil {
					earlierObjects[object] = true
				}
			}
			return true
		}, nil)
	}
	return earlierObjects, stableAddresses
}

func reportCallEvaluationDependencies(
	pass *analysis.Pass,
	earlierExpressions []ast.Expr,
	call *ast.CallExpr,
	earlierObjects map[types.Object]bool,
	stableAddresses map[types.Object]token.Pos,
) {
	for argumentIndex, argument := range call.Args {
		address, ok := argument.(*ast.UnaryExpr)
		if !ok || address.Op != token.AND {
			continue
		}
		identifier, ok := address.X.(*ast.Ident)
		if !ok {
			continue
		}
		object := pass.TypesInfo.ObjectOf(identifier)
		if !callMutatesArgument(pass, call, argumentIndex) {
			continue
		}
		if position := stableAddresses[object]; position != token.NoPos && !earlierObjects[object] {
			traceStableAddressIdentity(pass, position)
			continue
		}
		if !earlierObjects[object] || disjointFieldMutation(pass, earlierExpressions, call, argumentIndex, object) {
			continue
		}
		check.Reportf(pass, check.EvaluationOrder, address.Pos(), "later operand may mutate %s after its earlier value was evaluated", identifier.Name)
	}
}

func stableAddressIdentity(pass *analysis.Pass, expression ast.Expr) (types.Object, token.Pos) {
	address, ok := syntax.Unparen(expression).(*ast.UnaryExpr)
	if !ok || address.Op != token.AND {
		return nil, token.NoPos
	}
	identifier, ok := syntax.Unparen(address.X).(*ast.Ident)
	if !ok {
		return nil, token.NoPos
	}
	// Taking the exact variable's address computes stable storage identity; it
	// does not snapshot the stored value. A later mutation through the same
	// address is therefore visible through the pointer already evaluated. Open
	// Next Router decodes tagged JSON values directly into returned pointers:
	// https://github.com/r9s-ai/open-next-router/blob/84e1fd334386352c9c3943562ef085ea17592d8a/onr-core/pkg/apitypes/claude.go#L4199-L4212
	return pass.TypesInfo.ObjectOf(identifier), address.Pos()
}

func traceStableAddressIdentity(pass *analysis.Pass, position token.Pos) {
	checkID := string(check.EvaluationOrder)
	if !analysisTrace.Enabled("evalorder", checkID) {
		return
	}
	analysisTrace.Emit(pass, analysisTrace.Event{
		Analyzer: "evalorder",
		Check:    checkID,
		Phase:    "evidence",
		Reason:   "stable-address-identity",
		Outcome:  analysisTrace.OutcomeAccepted,
		Pos:      position,
	})
}

func walkEvaluatedExpression(expression ast.Expr, visit func(ast.Node) bool, delayed func(*ast.FuncLit)) {
	var ancestors []ast.Node
	ast.Inspect(expression, func(node ast.Node) bool {
		if node == nil {
			ancestors = ancestors[:len(ancestors)-1]
			return true
		}
		literal, ok := node.(*ast.FuncLit)
		if ok && !directlyInvokedFunctionLiteral(literal, ancestors) {
			// Constructing a function value captures variables but does not run
			// its body. Only a direct or parenthesized invocation supplies exact
			// evidence that body effects occur while this operand is evaluated.
			// ToolHive passes mutating callbacks as data in this form:
			// https://github.com/stacklok/toolhive/blob/d859bfe06e62443cf9767f864ba06d294faf24fd/pkg/skills/skillsvc/install.go#L92-L105
			if delayed != nil {
				delayed(literal)
			}
			return false
		}
		if !visit(node) {
			return false
		}
		ancestors = append(ancestors, node)
		return true
	})
}

func directlyInvokedFunctionLiteral(literal *ast.FuncLit, ancestors []ast.Node) bool {
	index := len(ancestors) - 1
	for index >= 0 {
		if _, ok := ancestors[index].(*ast.ParenExpr); !ok {
			break
		}
		index--
	}
	if index < 0 {
		return false
	}
	call, ok := ancestors[index].(*ast.CallExpr)
	return ok && syntax.Unparen(call.Fun) == literal
}

func traceDelayedFunctionBody(pass *analysis.Pass, literal *ast.FuncLit) {
	checkID := string(check.EvaluationOrder)
	if !analysisTrace.Enabled("evalorder", checkID) {
		return
	}
	analysisTrace.Emit(pass, analysisTrace.Event{
		Analyzer: "evalorder",
		Check:    checkID,
		Phase:    "evidence",
		Reason:   "delayed-function-body",
		Outcome:  analysisTrace.OutcomeAccepted,
		Pos:      literal.Pos(),
	})
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
	parameter := syntax.FunctionParameterObject(pass, declaration, argumentIndex)
	if parameter == nil {
		return false
	}
	earlierFields, fieldsOnly := selectedFields(pass, earlier, object)
	if !fieldsOnly || len(earlierFields) == 0 {
		return false
	}
	mutatedFields, wholeMutation := functionMutatedFields(pass, declaration, parameter)
	// Suppress only when both sides are field-selective and their field sets are
	// disjoint. Any whole-object use or write preserves the diagnostic because it
	// may observe or invalidate the earlier value.
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
		selector, ok := syntax.Unparen(expression).(*ast.SelectorExpr)
		if ok && syntax.ExpressionUsesObject(pass, selector.X, object) {
			fields[pass.TypesInfo.ObjectOf(selector.Sel)] = true
			continue
		}
		if syntax.ExpressionUsesObject(pass, expression, object) {
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
			selector, ok := syntax.Unparen(target).(*ast.SelectorExpr)
			if ok && syntax.ExpressionUsesObject(pass, selector.X, parameter) {
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
	// External calls are mutators only when their contract is explicitly known.
	// For local functions, inspect the concrete parameter body instead of
	// inferring mutation from pointer type alone.
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
			parameter := syntax.FunctionParameterObject(pass, declared, argumentIndex)
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
	return argumentIndex == 1 && syntax.IsCallToAny(
		pass,
		call,
		syntax.PackageFunction("encoding/json", "Unmarshal"),
		syntax.PackageFunction("encoding/xml", "Unmarshal"),
	)
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
		return syntax.ExpressionUsesObject(pass, candidate.X, parameter)
	case *ast.SelectorExpr:
		return syntax.ExpressionUsesObject(pass, candidate.X, parameter)
	case *ast.IndexExpr:
		return syntax.ExpressionUsesObject(pass, candidate.X, parameter)
	case *ast.ParenExpr:
		return writesThroughObject(pass, candidate.X, parameter)
	default:
		return false
	}
}
