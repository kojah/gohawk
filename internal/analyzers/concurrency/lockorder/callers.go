package lockorder

import (
	"github.com/kojah/gohawk/internal/ssaflow"
	"golang.org/x/tools/go/ssa"
)

// Both caller preconditions require complete private function uses, including
// initialization and generated bodies. An opaque use or an interrupted census
// cannot establish either cleanup coverage or exclusive parameter ownership.
type conditionalCallerSet struct {
	calls   []*ssa.Call
	escaped bool
}

// callerSetBudget bounds the whole-package caller census rather than one
// candidate. Exhaustion invalidates every caller set for both consumers.
const callerSetBudget = 20_000

func collectLockCallers(initialization *ssa.Function, functions []*ssa.Function, budget *ssaflow.SearchBudget) map[*ssa.Function]conditionalCallerSet {
	callers := map[*ssa.Function]conditionalCallerSet{}
	for _, function := range append([]*ssa.Function{initialization}, functions...) {
		if function == nil {
			continue
		}
		for instruction := range ssaflow.InstructionsWithin(function, budget) {
			collectConditionalCaller(callers, instruction)
		}
		if budget.Exhausted() {
			return nil
		}
	}
	return callers
}

func collectConditionalCaller(callers map[*ssa.Function]conditionalCallerSet, instruction ssa.Instruction) {
	if _, debug := instruction.(*ssa.DebugRef); debug {
		return
	}
	for _, operand := range instruction.Operands(nil) {
		if operand == nil {
			continue
		}
		callee, ok := (*operand).(*ssa.Function)
		if !ok || callee.Object() == nil || callee.Object().Exported() || callee.Signature.Recv() != nil {
			continue
		}
		entry := callers[callee]
		call, synchronous := instruction.(*ssa.Call)
		if synchronous && operand == &call.Common().Value && len(entry.calls) < 32 {
			entry.calls = append(entry.calls, call)
		} else {
			entry.escaped = true
		}
		callers[callee] = entry
	}
}
