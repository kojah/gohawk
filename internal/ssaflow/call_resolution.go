package ssaflow

import (
	proofs "github.com/kojah/gohawk/internal/proof"
	"golang.org/x/tools/go/ssa"
	// Callee resolution is one question with one answer: which function does this
	// call actually reach, in the form that carries evidence about it. A generic
	// call may point at an instantiated wrapper, and that wrapper is the wrong
	// place to look twice over. Its body may not survive instantiation, and a
	// lifecycle summary is recorded against the origin's object, so a proof that
	// reads the instantiation finds neither the source nor the facts.
)

// ResolvedCallee returns the callee a call reaches, answering a generic
// instantiation with its origin. The origin keeps the same parameter
// positions, so an argument index means the same thing in either form.
func ResolvedCallee(common *ssa.CallCommon) *ssa.Function {
	if common == nil {
		return nil
	}
	callee := common.StaticCallee()
	if callee == nil {
		return nil
	}
	return ResolvedFunction(callee)
}

// ResolvedFunction answers an instantiation with its origin for a function the
// caller already holds, such as the literal a launch names.
func ResolvedFunction(function *ssa.Function) *ssa.Function {
	if function == nil {
		return nil
	}
	if origin := function.Origin(); origin != nil {
		return origin
	}
	return function
}

// CallResultSource identifies a direct call result and its zero-based slot.
// It does not follow wrappers, loads, or aliases; consumers select that policy.
func CallResultSource(value ssa.Value) (*ssa.Call, int, bool) {
	if extract, ok := value.(*ssa.Extract); ok {
		call, ok := extract.Tuple.(*ssa.Call)
		return call, extract.Index, ok
	}
	call, ok := value.(*ssa.Call)
	return call, 0, ok
}

// CallResult returns the selected SSA result of call. A negative index denotes
// a single-result call represented by the call instruction itself.
func CallResult(call *ssa.Call, index int) ssa.Value { //nolint:ireturn // SSA call results have several concrete forms.
	return CallResultWithin(call, index, nil)
}

// CallResultWithin selects the same exact result under a shared allowance.
// Referrers are charged before inspection; a single-result lookup costs one
// visit. Nil at cutoff means unavailable, not an absent result. It never follows
// aliases or substitutes a sibling result. A nil budget retains default policy.
func CallResultWithin(call *ssa.Call, index int, budget *proofs.SearchBudget) ssa.Value { //nolint:ireturn // SSA results have several concrete forms.
	if index < 0 {
		if !budget.Spend() {
			return nil
		}
		return call
	}
	if call.Referrers() == nil {
		return nil
	}
	for _, reference := range *call.Referrers() {
		if !budget.Spend() {
			return nil
		}
		if extract, ok := reference.(*ssa.Extract); ok && extract.Index == index {
			return extract
		}
	}
	return nil
}
