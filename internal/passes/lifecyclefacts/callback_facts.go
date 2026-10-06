package lifecyclefacts

import (
	"github.com/kojah/gohawk/internal/heapmodel"
	proofs "github.com/kojah/gohawk/internal/proof"
	"github.com/kojah/gohawk/internal/ssaflow"
	ssacall "github.com/kojah/gohawk/internal/ssaflow/calls"
	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/ssa"
)

// Callbacks a callee in this package invokes are known as soon as the callee
// is summarized, and callees are summarized before their callers, but facts
// are exported only once the whole package is summarized. A helper that
// forwards its callback to a sibling that calls it would otherwise see no
// fact for the sibling and claim nothing. ubuntu/decorate's LogFuncOnError
// hands its callback to LogFuncOnErrorContext, which calls it, so
// `defer decorate.LogFuncOnError(file.Close)` closes the file:
// https://github.com/ubuntu/decorate/blob/b76bb81d1209ba92b56c759d828f75c19479a444/decorate.go#L35-L45
//
// Only the invocation claims read these summaries. Every other claim keeps
// importing exported facts, because some are completed after the package is
// summarized. An unexported sibling is never summarized, so its invocation
// claims are proved from its body when first asked. Recursion and budget cuts
// claim nothing and never become cached declaration guarantees.

// callbackInference owns invocation evidence for one package pass. Completed
// declaration summaries and bounded invocation-only summaries stay local to
// that pass; the shared summary engine owns recursion and incomplete answers.
type callbackInference struct {
	pass        *analysis.Pass
	completed   Summaries
	invocations *ssacall.FunctionSummaries[Fact]
}

func newCallbackInference(pass *analysis.Pass, completed Summaries) *callbackInference {
	callbacks := &callbackInference{pass: pass, completed: completed}
	callbacks.invocations = ssacall.NewFunctionSummaries(callbacks.computeInvocations, func(ssacall.SummaryUnavailable) Fact {
		return Fact{}
	})
	return callbacks
}

func (callbacks *callbackInference) computeInvocations(function *ssa.Function, budget *proofs.SearchBudget) Fact {
	fact := Fact{signature: function.Signature}
	for index, parameter := range function.Params {
		if index < 64 && ownershipCapableType(parameter.Type()) {
			fact.Discharges = append(fact.Discharges, callbacks.invocationDischarges(function, index, parameter, budget)...)
		}
	}
	return fact
}

// invocationDischarges proves invocation on every normal return separately
// from synchronous invocation. An asynchronous call supports only the former.
func (callbacks *callbackInference) invocationDischarges(
	function *ssa.Function, index int, parameter ssa.Value, budget *proofs.SearchBudget,
) []Discharge {
	var discharges []Discharge
	for _, method := range []string{InvokeMethod, SynchronousInvokeMethod} {
		if ownsOnEveryReturn(function, parameter, func(instruction ssa.Instruction) bool {
			return callbacks.invokesParameter(instruction, parameter, method, budget)
		}) {
			discharges = append(discharges, Discharge{Parameter: index, Method: method})
		}
	}
	return discharges
}

func (callbacks *callbackInference) invokesParameter(
	instruction ssa.Instruction, parameter ssa.Value, method string, budget *proofs.SearchBudget,
) bool {
	if !budget.Spend() {
		return false
	}
	if _, asynchronous := instruction.(*ssa.Go); asynchronous && method == SynchronousInvokeMethod {
		return false
	}
	common := ssaflow.InstructionCall(instruction)
	if common != nil && heapmodel.NewStorage(budget).Same(common.Value, parameter).Proven() {
		return true
	}
	fact, ok := callbacks.fact(instruction, budget)
	return ok && factOwnsExactArgument(instruction, parameter, fact.MethodMask(method))
}

// fact reads imported or completed declaration evidence before asking the
// shared engine for a visible sibling's invocation-only summary. Recursive or
// exhausted queries make no claim and cannot leave a partial cached fact.
func (callbacks *callbackInference) fact(instruction ssa.Instruction, budget *proofs.SearchBudget) (Fact, bool) {
	if fact, ok := importFact(callbacks.pass, instruction); ok {
		return fact, true
	}
	callee := ssacall.ResolvedCallee(ssaflow.InstructionCall(instruction))
	if callee == nil || callbacks.pass == nil || callee.Pkg == nil || callee.Pkg.Pkg != callbacks.pass.Pkg || len(callee.Blocks) == 0 {
		return Fact{}, false
	}
	if fact, ok := callbacks.completed[callee]; ok {
		return fact, true
	}
	return callbacks.invocations.Function(callee, budget), true
}
