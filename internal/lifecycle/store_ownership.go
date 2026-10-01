package lifecycle

import (
	"github.com/kojah/gohawk/internal/heapmodel"
	"github.com/kojah/gohawk/internal/ssaflow"
	"golang.org/x/tools/go/ssa"
)

func closureCallsValue(closure *ssa.MakeClosure, target ssa.Value) bool {
	return closureCallsCapturedValue(closure, func(binding ssa.Value) bool {
		return heapmodel.CapturedBindingMatches(binding, target)
	})
}

func closureCallsCapturedValue(closure *ssa.MakeClosure, owns func(ssa.Value) bool) bool {
	function, ok := closure.Fn.(*ssa.Function)
	if !ok {
		return false
	}
	bindings := ssaflow.ClosureBindingPairs(function, closure)
	for _, block := range function.Blocks {
		for _, candidate := range block.Instrs {
			if nested, ok := candidate.(*ssa.MakeClosure); ok && closureCallsCapturedValue(nested, func(binding ssa.Value) bool {
				for _, pair := range bindings {
					if heapmodel.CapturedBindingMatches(binding, pair.Free) && owns(pair.Binding) {
						return true
					}
				}
				return false
			}) {
				return true
			}
			common := ssaflow.InstructionCall(candidate)
			if common == nil {
				continue
			}
			for _, pair := range bindings {
				if heapmodel.ValueDerivesFrom(common.Value, pair.Free) && owns(pair.Binding) {
					return true
				}
			}
		}
	}
	return false
}

// MayContainValue reports whether owner may be an aggregate or closure that
// transitively contains value. Possible containment only: it can hide a
// diagnostic behind an opaque owner, never prove that the owner settles it.
func MayContainValue(owner, value ssa.Value) bool {
	if !heapmodel.CanHoldReference(owner.Type()) {
		return false
	}
	if valueOwnsValue(owner, value) || newOwnershipSearch(nil).aggregateStoresValue(owner, value) {
		return true
	}
	// The graph follows containment through copies, merges, and captured
	// cells the value walk does not; the walk keeps the visible-constructor
	// cases the graph, being intraprocedural, cannot see.
	return heapmodel.Contains(owner, value)
}

// MayContainValueAt is MayContainValue asked at one instruction: whether the
// owner may hold the value when the instruction runs. A call's argument is
// judged before the call, so a callee summarized as storing the value into
// the argument does not make the argument contain it already.
func MayContainValueAt(owner, value ssa.Value, at ssa.Instruction) bool {
	if !heapmodel.CanHoldReference(owner.Type()) {
		return false
	}
	if valueOwnsValue(owner, value) || newOwnershipSearch(nil).aggregateStoresValue(owner, value) {
		return true
	}
	contained, known := heapmodel.ContainsAt(owner, value, at)
	return known && contained
}

func valueOwnsValue(owner, value ssa.Value) bool {
	found := false
	ssaflow.WalkStates([]ssa.Value{owner}, func(owner ssa.Value) ssa.Value { return owner }, func(owner ssa.Value) ([]ssa.Value, bool) {
		if owner == nil {
			return nil, true
		}
		// A possible alias is evidence before wrappers are peeled. Only
		// wrappers and closure captures extend this narrow ownership query;
		// it does not independently fan out phi alternatives or call results.
		if heapmodel.MayAlias(owner, value) {
			found = true
			return nil, false
		}
		if inner, ok := ssaflow.UnwrapTransparentValue(
			owner, ssaflow.TransparentChangeInterface|ssaflow.TransparentChangeType|ssaflow.TransparentConvert|ssaflow.TransparentMakeInterface,
		); ok {
			return []ssa.Value{inner}, true
		}
		var successors []ssa.Value
		if closure, ok := owner.(*ssa.MakeClosure); ok {
			found = closureBindingsOwnValue(closure, value, func(binding ssa.Value) bool {
				successors = append(successors, binding)
				return false
			})
		}
		return successors, !found
	})
	return found
}

// Capture identity and cell contents are shared mechanics. The caller chooses
// whether to follow only nested callbacks or also owning aggregates.
func closureBindingsOwnValue(closure *ssa.MakeClosure, value ssa.Value, owns func(ssa.Value) bool) bool {
	for _, binding := range closure.Bindings {
		if heapmodel.CapturedBindingMatches(binding, value) || owns(ssaflow.CapturedBindingValue(binding)) {
			return true
		}
	}
	return false
}
