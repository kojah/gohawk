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
