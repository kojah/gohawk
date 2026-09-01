package testpolicy

import (
	"go/ast"
	"go/types"

	"github.com/kojah/gohawk/internal/check"
	"github.com/kojah/gohawk/internal/syntax"
	analysisTrace "github.com/kojah/gohawk/internal/trace"

	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/ssa"
)

// This file classifies named functions and selected methods used as exact
// testing callbacks. Any direct call or other escape keeps helper diagnostics.

func namedTestingCallbacks(pass *analysis.Pass) map[types.Object]bool {
	// Classification is deliberately whole-package and two-pass. First collect
	// named functions used in parameters whose exact contract is a testing
	// callback; then reject a candidate if any use is not one of those arguments.
	// The second pass preserves helper diagnostics for direct calls and mixed
	// callback/helper use instead of treating one callback registration as an
	// exemption for every call site.
	candidates := map[types.Object]bool{}
	callbackUses := map[*ast.Ident]bool{}
	for _, file := range pass.Files {
		ast.Inspect(file, func(node ast.Node) bool {
			call, ok := node.(*ast.CallExpr)
			if !ok {
				return true
			}
			functionType := pass.TypesInfo.TypeOf(call.Fun)
			if functionType == nil {
				return true
			}
			signature, ok := functionType.Underlying().(*types.Signature)
			if !ok {
				return true
			}
			for index, argument := range call.Args {
				if !testingCallbackParameter(signature, index) {
					continue
				}
				function, identifier := testingCallbackFunction(pass, syntax.Unparen(argument))
				if function == nil {
					continue
				}
				candidates[function] = true
				callbackUses[identifier] = true
			}
			return true
		})
	}
	// A function remains a callback only when every reference is one of the
	// exact argument nodes recorded above. A direct call, assignment, return, or
	// other escape leaves its testing role ambiguous and keeps the diagnostic.
	for identifier, object := range pass.TypesInfo.Uses {
		function, ok := object.(*types.Func)
		if ok && candidates[function] && !callbackUses[identifier] {
			delete(candidates, function)
		}
	}
	return candidates
}

func testingCallbackFunction(pass *analysis.Pass, expression ast.Expr) (*types.Func, *ast.Ident) {
	switch typed := expression.(type) {
	case *ast.Ident:
		function, _ := pass.TypesInfo.Uses[typed].(*types.Func)
		return function, typed
	case *ast.SelectorExpr:
		selection := pass.TypesInfo.Selections[typed]
		if selection == nil {
			return nil, nil
		}
		function, _ := selection.Obj().(*types.Func)
		return function, typed.Sel
	default:
		return nil, nil
	}
}

func testingCallbackParameter(signature *types.Signature, argument int) bool {
	parameters := signature.Params()
	if argument < 0 || argument >= parameters.Len() {
		return false
	}
	callback, ok := parameters.At(argument).Type().Underlying().(*types.Signature)
	return ok && !callback.Variadic() && callback.Params().Len() == 1 && callback.Results().Len() == 0 &&
		testingHandle(callback.Params().At(0).Type())
}

func emitTestingCallbackDecision(pass *analysis.Pass, function *ssa.Function) {
	checkID := string(check.TestHelperMarker)
	if !analysisTrace.Enabled("testpolicy", checkID) {
		return
	}
	// Named functions and selected methods have the same testing-owned boundary
	// as function literals. Dranet passes namespace test bodies to a runner, and
	// Incus does the same with method values:
	// https://github.com/kubernetes-sigs/dranet/blob/53e6c967d7b0b8e2c46e070c7129f712c631a2ab/pkg/inventory/net_test.go#L32-L39
	// https://github.com/lxc/incus-compose/blob/a7da6db1112780ad83c75a9a5136c111ad1d9b71/cmd/ic-dns/e2e_visibility_test.go#L74-L81
	analysisTrace.Emit(pass, analysisTrace.Event{
		Analyzer: "testpolicy",
		Check:    checkID,
		Phase:    "decision",
		Reason:   "testing-callback",
		Outcome:  analysisTrace.OutcomeAccepted,
		Pos:      function.Pos(),
		Function: function.String(),
	})
}
