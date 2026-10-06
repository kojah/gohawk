package lifecycle

import (
	"github.com/kojah/gohawk/internal/heapmodel"
	"github.com/kojah/gohawk/internal/ssaflow"
	ssacall "github.com/kojah/gohawk/internal/ssaflow/calls"
	"golang.org/x/tools/go/ssa"
)

// Closure evidence follows captured values through bindings, stored callbacks,
// and helper calls. These helpers accept only concrete SSA relationships and
// stop at cycles or unmodeled indirection so callers can treat a match as proof.

// DeferredClosureCallsValue reports whether a deferred closure calls target.
func DeferredClosureCallsValue(instruction ssa.Instruction, target ssa.Value) bool {
	if _, ok := instruction.(*ssa.Defer); !ok {
		return false
	}
	return ClosureCallsValue(instruction, target)
}

// DeferredClosureInvokesArgumentOnEveryReturn reports whether a deferred
// closure delegates target to a helper that invokes it on every normal path.
func DeferredClosureInvokesArgumentOnEveryReturn(instruction ssa.Instruction, target ssa.Value) bool {
	if _, ok := instruction.(*ssa.Defer); !ok {
		return false
	}
	common := ssaflow.InstructionCall(instruction)
	function, closure := ssacall.DirectCallee(common)
	if function == nil {
		return false
	}
	bindings := ssacall.CallBindings(common, function, closure)
	for _, block := range function.Blocks {
		for _, candidate := range block.Instrs {
			for _, binding := range bindings {
				// Captures name cells read by the deferred body; arguments
				// are values evaluated when the defer is registered. Keep
				// their matching policies distinct after pairing them once.
				matches := heapmodel.MayAlias(binding.Supplied, target)
				if binding.Captured {
					matches = heapmodel.CapturedBindingMatches(binding.Supplied, target)
				}
				if matches && CallInvokesArgumentOnEveryReturn(candidate, binding.Local) {
					return true
				}
			}
		}
	}
	return false
}

// ClosureCallsValue reports whether a call-like closure or created callback calls target.
func ClosureCallsValue(instruction ssa.Instruction, target ssa.Value) bool {
	var closure *ssa.MakeClosure
	if created, ok := instruction.(*ssa.MakeClosure); ok {
		if created.Referrers() == nil || len(*created.Referrers()) == 0 {
			return false
		}
		closure = created
	} else if common := ssaflow.InstructionCall(instruction); common != nil {
		closure, _ = common.Value.(*ssa.MakeClosure)
	}
	if closure == nil {
		return false
	}
	return closureCallsValue(closure, target)
}
