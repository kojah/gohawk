package ssaflow

import (
	"maps"

	"github.com/kojah/gohawk/internal/syntax"

	"golang.org/x/tools/go/ssa"
)

var syncOnceFunc = syntax.PackageFunction("sync", "OnceFunc")

// Deferred bindings are observed when the deferred callee runs, after any
// assignment that follows the defer. These helpers recover the one value an
// addressable capture or stored callback holds at that point: the latest
// store that dominates the observation, provided no store may run after it
// and no store sits on a branch before it.

//nolint:ireturn // SSA bindings have several concrete forms.
func deferredBindingValue(binding, target ssa.Value, invocation ssa.Instruction) (ssa.Value, bool) {
	if valueHasDirectStore(binding) {
		// Addressable captures must use the store proof below. General identity
		// traversal may otherwise match one historical value while the deferred
		// callback observes a later reassignment.
		return storedValueAt(binding, invocation)
	}
	if SameValue(binding, target) || ValueIsAccessPathFrom(target, binding) {
		return binding, true
	}
	stored, ok := storedValueAt(binding, invocation)
	return stored, ok
}

func valueHasDirectStore(value ssa.Value) bool {
	if value == nil || value.Referrers() == nil {
		return false
	}
	for _, reference := range *value.Referrers() {
		store, ok := reference.(*ssa.Store)
		if ok && store.Addr == value {
			return true
		}
	}
	return false
}

// storedValueAt returns the value address holds when observation runs. A
// reassigned local such as `rows, err = query()` after an earlier query is
// resolved to its latest dominating store; a store that may run after the
// observation, or one on a branch before it, leaves the value ambiguous.
// Sidecar re-queries into the same rows variable before deferring its Close:
// https://github.com/marcus/sidecar/blob/9b8739f753ab235dda2630676833e9b46a52696c/internal/adapter/warp/adapter.go#L337-L341
//
//nolint:ireturn // Stored callbacks and captures may be any SSA value.
func storedValueAt(address ssa.Value, observation ssa.Instruction) (ssa.Value, bool) {
	if address == nil || address.Referrers() == nil {
		return nil, false
	}
	var stores []*ssa.Store
	for _, reference := range *address.Referrers() {
		store, ok := reference.(*ssa.Store)
		if !ok || store.Addr != address {
			continue
		}
		if !InstructionDominates(store, observation) || InstructionMayFollow(observation, store) {
			return nil, false
		}
		stores = append(stores, store)
	}
	for _, candidate := range stores {
		latest := true
		for _, other := range stores {
			if other != candidate && !InstructionDominates(other, candidate) {
				latest = false
				break
			}
		}
		if latest {
			return candidate.Val, true
		}
	}
	return nil, false
}

func cloneValueSet(source map[ssa.Value]bool) map[ssa.Value]bool {
	result := make(map[ssa.Value]bool, len(source))
	maps.Copy(result, source)
	return result
}
