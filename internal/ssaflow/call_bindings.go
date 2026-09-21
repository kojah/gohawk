package ssaflow

import "golang.org/x/tools/go/ssa"

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
	if callee == nil {
		return nil
	}
	var bindings []CallBinding
	if common != nil {
		for index, argument := range common.Args {
			if index < len(callee.Params) {
				bindings = append(bindings, CallBinding{Local: callee.Params[index], Supplied: argument})
			}
		}
	}
	for _, capture := range ClosureBindingPairs(callee, closure) {
		bindings = append(bindings, CallBinding{Local: capture.Free, Supplied: capture.Binding, Captured: true})
	}
	return bindings
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
