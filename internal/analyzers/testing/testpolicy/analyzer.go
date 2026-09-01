// Package testpolicy implements the testpolicy gohawk analyzer.
package testpolicy

import (
	"go/ast"
	"go/token"
	"go/types"
	"strings"

	"github.com/kojah/gohawk/internal/check"
	"github.com/kojah/gohawk/internal/ssaflow"
	"github.com/kojah/gohawk/internal/syntax"
	analysisTrace "github.com/kojah/gohawk/internal/trace"

	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/analysis/passes/buildssa"
	"golang.org/x/tools/go/ssa"
)

// Analyzer returns this package's configured Go analysis pass.
func Analyzer() *analysis.Analyzer {
	return &analysis.Analyzer{
		Name:     "testpolicy",
		Doc:      "checks lifecycle ownership in test helpers",
		Requires: []*analysis.Analyzer{buildssa.Analyzer},
		Run:      runTestPolicy,
	}
}

func runTestPolicy(pass *analysis.Pass) (any, error) {
	functions, err := ssaflow.SourceSSAFunctions(pass)
	if err != nil {
		return nil, err
	}
	callbacks := namedTestingCallbacks(pass)
	for _, function := range functions {
		file := ssaflow.FunctionFile(pass, function)
		_, declaration := function.Syntax().(*ast.FuncDecl)
		// Function literals that accept *testing.T are callbacks, not helpers:
		// t.Run bodies and table-driven builders should retain the caller's
		// location rather than marking themselves as reusable helper boundaries.
		if !declaration || file == nil || !strings.HasSuffix(pass.Fset.Position(file.Pos()).Filename, "_test.go") || testEntryPoint(function.Name()) {
			continue
		}
		handle := testingSSAParameter(function)
		if handle == nil {
			continue
		}
		if callbacks[function.Object()] {
			emitTestingCallbackDecision(pass, function)
			continue
		}
		if testingHandleCapturedOnlyByReturnedClosures(function, handle) {
			emitReturnedTestingClosureDecision(pass, function)
			continue
		}
		if ssaflow.UnownedReturnFromEntry(function, func(instruction ssa.Instruction) bool {
			common := ssaflow.InstructionCall(instruction)
			return ssaflow.CallName(common) == "Helper" && ssaflow.ValueDerivesFrom(ssaflow.CallReceiver(common), handle, map[ssa.Value]bool{})
		}) {
			source := syntax.SourceRange(pass, function.Pos())
			check.Report(pass, check.TestHelperMarker, analysis.Diagnostic{
				Pos:            source.Pos(),
				End:            source.End(),
				Message:        "test helper accepting " + handle.Name() + " must call " + handle.Name() + ".Helper() on every return path",
				SuggestedFixes: testHelperFix(pass, function, handle),
			})
		}
	}
	return nil, nil
}

func testingHandleCapturedOnlyByReturnedClosures(function *ssa.Function, handle *ssa.Parameter) bool {
	references := handle.Referrers()
	if references == nil || len(*references) == 0 {
		return false
	}
	closures := map[*ssa.MakeClosure]bool{}
	bindings := map[ssa.Value]bool{}
	for _, block := range function.Blocks {
		for _, instruction := range block.Instrs {
			closure, ok := instruction.(*ssa.MakeClosure)
			if !ok || !closureCapturesTestingHandle(closure, handle) || !closureUsedOnlyByReturns(closure) {
				continue
			}
			closures[closure] = true
			for _, binding := range closure.Bindings {
				if ssaflow.CapturedBindingMatches(binding, handle) {
					bindings[binding] = true
				}
			}
		}
	}
	if len(closures) == 0 {
		return false
	}
	return testingHandleReferencesOnlyClosures(*references, handle, closures, bindings) &&
		capturedBindingsReferenceOnlyClosures(bindings, handle, closures)
}

func testingHandleReferencesOnlyClosures(
	references []ssa.Instruction,
	handle *ssa.Parameter,
	closures map[*ssa.MakeClosure]bool,
	bindings map[ssa.Value]bool,
) bool {
	for _, reference := range references {
		switch typed := reference.(type) {
		case *ssa.DebugRef:
			continue
		case *ssa.MakeClosure:
			if closures[typed] {
				continue
			}
		case *ssa.Store:
			if ssaflow.SameValue(typed.Val, handle) && bindings[typed.Addr] {
				continue
			}
		}
		return false
	}
	return true
}

func capturedBindingsReferenceOnlyClosures(
	bindings map[ssa.Value]bool,
	handle *ssa.Parameter,
	closures map[*ssa.MakeClosure]bool,
) bool {
	for binding := range bindings {
		if binding.Referrers() == nil {
			return false
		}
		for _, reference := range *binding.Referrers() {
			switch typed := reference.(type) {
			case *ssa.DebugRef:
				continue
			case *ssa.MakeClosure:
				if closures[typed] {
					continue
				}
			case *ssa.Store:
				if typed.Addr == binding && ssaflow.SameValue(typed.Val, handle) {
					continue
				}
			}
			return false
		}
	}
	return true
}

func closureCapturesTestingHandle(closure *ssa.MakeClosure, handle *ssa.Parameter) bool {
	for _, binding := range closure.Bindings {
		if ssaflow.CapturedBindingMatches(binding, handle) {
			return true
		}
	}
	return false
}

func closureUsedOnlyByReturns(closure *ssa.MakeClosure) bool {
	onlyReturned, reachesReturn := valueUsedOnlyByReturns(closure, map[ssa.Value]bool{})
	return onlyReturned && reachesReturn
}

// Returned callbacks may pass through compiler-created interface, conversion,
// or phi values before the Return instruction. Follow only those transparent
// wrappers and reject every operational use: invoking, storing, or passing the
// callback would make the outer helper part of the eventual failure path.
func valueUsedOnlyByReturns(value ssa.Value, seen map[ssa.Value]bool) (bool, bool) {
	if value == nil || value.Referrers() == nil {
		return false, false
	}
	if seen[value] {
		return true, false
	}
	seen[value] = true
	reachesReturn := false
	for _, reference := range *value.Referrers() {
		if _, debug := reference.(*ssa.DebugRef); debug {
			continue
		}
		if returned, ok := reference.(*ssa.Return); ok && ssaflow.ReturnSameValue(returned, value) {
			reachesReturn = true
			continue
		}
		wrapped, ok := transparentClosureWrapper(reference, value)
		if !ok {
			return false, false
		}
		onlyReturned, wrappedReturns := valueUsedOnlyByReturns(wrapped, seen)
		if !onlyReturned {
			return false, false
		}
		reachesReturn = reachesReturn || wrappedReturns
	}
	return true, reachesReturn
}

func transparentClosureWrapper(reference ssa.Instruction, value ssa.Value) (ssa.Value, bool) { //nolint:ireturn // SSA wrappers have distinct concrete types.
	switch typed := reference.(type) {
	case *ssa.ChangeInterface:
		return typed, ssaflow.SameValue(typed.X, value)
	case *ssa.ChangeType:
		return typed, ssaflow.SameValue(typed.X, value)
	case *ssa.Convert:
		return typed, ssaflow.SameValue(typed.X, value)
	case *ssa.MakeInterface:
		return typed, ssaflow.SameValue(typed.X, value)
	case *ssa.Phi:
		for _, edge := range typed.Edges {
			if ssaflow.SameValue(edge, value) {
				return typed, true
			}
		}
	}
	return nil, false
}

func emitReturnedTestingClosureDecision(pass *analysis.Pass, function *ssa.Function) {
	checkID := string(check.TestHelperMarker)
	if !analysisTrace.Enabled("testpolicy", checkID) {
		return
	}
	// Calling Helper in an outer factory cannot affect a failure raised later by
	// its returned callback. Civitai uses this shape for an HTTP test handler:
	// https://github.com/civitai/cli/blob/bc830b105867ae4234ddd7dd23f3f7680a2cbe3c/internal/cmd/app_listing_test.go#L321-L348
	analysisTrace.Emit(pass, analysisTrace.Event{
		Analyzer: "testpolicy",
		Check:    checkID,
		Phase:    "decision",
		Reason:   "returned-testing-callback",
		Outcome:  analysisTrace.OutcomeAccepted,
		Pos:      function.Pos(),
		Function: function.String(),
	})
}

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
				identifier, ok := syntax.Unparen(argument).(*ast.Ident)
				if !ok {
					continue
				}
				function, ok := pass.TypesInfo.Uses[identifier].(*types.Func)
				if !ok {
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
	// Named callbacks have the same testing-owned boundary as function literals.
	// Dranet passes namespace test bodies to a runner that invokes them with t.Run:
	// https://github.com/kubernetes-sigs/dranet/blob/53e6c967d7b0b8e2c46e070c7129f712c631a2ab/pkg/inventory/net_test.go#L32-L39
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

func testHelperFix(pass *analysis.Pass, function *ssa.Function, handle *ssa.Parameter) []analysis.SuggestedFix {
	name := handle.Name()
	if name == "" || name == "_" || !token.IsIdentifier(name) || hasHelperCall(function, handle) {
		return nil
	}
	var body *ast.BlockStmt
	switch syntax := function.Syntax().(type) {
	case *ast.FuncDecl:
		body = syntax.Body
	case *ast.FuncLit:
		body = syntax.Body
	}
	if body == nil {
		return nil
	}
	position, newText := body.Rbrace, []byte("\n\t"+name+".Helper()\n")
	if file := pass.Fset.File(body.Lbrace); file != nil {
		braceLine := file.Line(body.Lbrace)
		if file.Line(body.Rbrace) > braceLine && braceLine < file.LineCount() {
			position = file.LineStart(braceLine + 1)
			newText = []byte("\t" + name + ".Helper()\n")
		}
	}
	return []analysis.SuggestedFix{{
		Message: "Call " + name + ".Helper() at function entry",
		TextEdits: []analysis.TextEdit{{
			Pos:     position,
			NewText: newText,
		}},
	}}
}

func hasHelperCall(function *ssa.Function, handle *ssa.Parameter) bool {
	for _, block := range function.Blocks {
		for _, instruction := range block.Instrs {
			common := ssaflow.InstructionCall(instruction)
			if ssaflow.CallName(common) == "Helper" && ssaflow.ValueDerivesFrom(ssaflow.CallReceiver(common), handle, map[ssa.Value]bool{}) {
				return true
			}
		}
	}
	return false
}

func testEntryPoint(name string) bool {
	return strings.HasPrefix(name, "Test") || strings.HasPrefix(name, "Benchmark") || strings.HasPrefix(name, "Fuzz")
}

func testingSSAParameter(function *ssa.Function) *ssa.Parameter {
	for _, parameter := range function.Params {
		if testingHandle(parameter.Type()) {
			return parameter
		}
	}
	return nil
}

func testingHandle(value types.Type) bool {
	pointer, ok := value.(*types.Pointer)
	if !ok {
		return false
	}
	return syntax.NamedType(pointer.Elem(), "testing", "T") || syntax.NamedType(pointer.Elem(), "testing", "B")
}
