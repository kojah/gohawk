package calls

import (
	"iter"

	proofs "github.com/kojah/gohawk/internal/proof"
	ssaflow "github.com/kojah/gohawk/internal/ssaflow"
	"golang.org/x/tools/go/ssa"
)

// CallBinding pairs one callee-local parameter or capture with its caller value.
// Captured distinguishes lexical cells from eagerly evaluated arguments. No
// alias, stability, completion, or ownership guarantee is implied by a binding.
type CallBinding struct {
	Local, Supplied ssa.Value
	Captured        bool
}

// CallBindings maps arguments and captures onto a known callee. A nil common
// supports a closure examined before invocation. Matching values and deciding
// what their uses mean remain the consumer's responsibility.
func CallBindings(common *ssa.CallCommon, callee *ssa.Function, closure *ssa.MakeClosure) []CallBinding {
	var bindings []CallBinding
	for binding := range CallBindingsWithin(common, callee, closure, nil) {
		bindings = append(bindings, binding)
	}
	return bindings
}

// CallBindingsWithin yields arguments followed by captures, charging metadata
// visits before yielding. It allocates no binding slice and stops when the
// consumer stops. A cutoff does not prove an unvisited binding is absent;
// callers inspect budget availability. A nil budget retains default policy.
func CallBindingsWithin(
	common *ssa.CallCommon, callee *ssa.Function, closure *ssa.MakeClosure, budget *proofs.SearchBudget,
) iter.Seq[CallBinding] {
	return func(yield func(CallBinding) bool) {
		if callee == nil {
			return
		}
		if common != nil {
			for index, argument := range common.Args {
				if !budget.Spend() {
					return
				}
				if index < len(callee.Params) && !yield(CallBinding{Local: callee.Params[index], Supplied: argument}) {
					return
				}
			}
		}
		for capture := range ssaflow.ClosureBindingPairsWithin(callee, closure, budget) {
			if !yield(CallBinding{Local: capture.Free, Supplied: capture.Binding, Captured: true}) {
				return
			}
		}
	}
}

// DirectCallee returns only the statically named function or literal body.
// Dynamic dispatch remains unresolved; this does not chase callback bindings.
func DirectCallee(common *ssa.CallCommon) (*ssa.Function, *ssa.MakeClosure) {
	if common == nil {
		return nil, nil
	}
	closure, _ := common.Value.(*ssa.MakeClosure)
	function := common.StaticCallee()
	if closure != nil {
		function, _ = closure.Fn.(*ssa.Function)
	}
	return ResolvedFunction(function), closure
}
