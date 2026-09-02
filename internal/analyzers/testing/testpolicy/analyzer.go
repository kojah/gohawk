// Package testpolicy implements the testpolicy gohawk analyzer.
package testpolicy

import (
	"go/ast"
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
		if testingHandleUnused(handle) {
			emitUnusedTestingHandleDecision(pass, function)
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

func testingHandleUnused(handle *ssa.Parameter) bool {
	references := handle.Referrers()
	if references == nil {
		return true
	}
	for _, reference := range *references {
		if _, debug := reference.(*ssa.DebugRef); !debug {
			return false
		}
	}
	return true
}

func emitUnusedTestingHandleDecision(pass *analysis.Pass, function *ssa.Function) {
	// Helper only changes attribution for operations reached through the testing
	// handle. Calling it cannot help when the parameter is wholly unused, as in
	// these Armada and Incus helpers:
	// https://github.com/armadaproject/armada-operator/blob/2326513ebd93e3cf5153bc4f3fbec7199c0cb30e/internal/controller/install/common_helpers_test.go#L1030
	// https://github.com/lxc/incus-compose/blob/a7da6db1112780ad83c75a9a5136c111ad1d9b71/cmd/incus-compose/backup_test.go#L63-L71
	traceHelperMarkerDecision(pass, function, "unused-testing-handle")
}

// traceHelperMarkerDecision records why a function is accepted without a
// Helper call.
func traceHelperMarkerDecision(pass *analysis.Pass, function *ssa.Function, reason string) {
	checkID := string(check.TestHelperMarker)
	if !analysisTrace.Enabled("testpolicy", checkID) {
		return
	}
	analysisTrace.Emit(pass, analysisTrace.Event{
		Analyzer: "testpolicy",
		Check:    checkID,
		Phase:    "decision",
		Reason:   reason,
		Outcome:  analysisTrace.OutcomeAccepted,
		Pos:      function.Pos(),
		Function: function.String(),
	})
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
