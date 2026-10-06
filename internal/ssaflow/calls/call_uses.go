package calls

import (
	proofs "github.com/kojah/gohawk/internal/proof"
	ssaflow "github.com/kojah/gohawk/internal/ssaflow"
	"golang.org/x/tools/go/ssa"
	// PrivateFunctionUses describes declaration-resolved uses in a supplied body
	// inventory. It establishes no ownership, completion or execution count.
)

type PrivateFunctionUses struct {
	// Calls contains at most 32 direct synchronous calls.
	Calls []*ssa.Call
	// Escaped includes opaque, asynchronous, deferred and excess uses.
	Escaped bool
}

// CollectPrivateFunctionUsesWithin collects uses of unexported non-method
// declarations. Callers supply the complete body scope, including initialization
// and closures when relevant. A nil result means an interrupted census; no
// discovered prefix may establish absence of other uses. Missing entries have
// no observed use, rather than a guarantee about execution outside the scope.
func CollectPrivateFunctionUsesWithin(functions []*ssa.Function, budget *proofs.SearchBudget) map[*ssa.Function]PrivateFunctionUses {
	uses := map[*ssa.Function]PrivateFunctionUses{}
	for _, function := range functions {
		if function == nil {
			continue
		}
		for instruction := range ssaflow.InstructionsWithin(function, budget) {
			if _, debug := instruction.(*ssa.DebugRef); debug {
				continue
			}
			for _, operand := range instruction.Operands(nil) {
				if !budget.Spend() {
					return nil
				}
				if operand == nil {
					continue
				}
				callee, ok := (*operand).(*ssa.Function)
				if !ok || callee.Object() == nil || callee.Object().Exported() || callee.Signature.Recv() != nil {
					continue
				}
				entry := uses[callee]
				call, synchronous := instruction.(*ssa.Call)
				if synchronous && operand == &call.Common().Value && len(entry.Calls) < 32 {
					entry.Calls = append(entry.Calls, call)
				} else {
					entry.Escaped = true
				}
				uses[callee] = entry
			}
		}
		if budget.Exhausted() {
			return nil
		}
	}
	return uses
}
