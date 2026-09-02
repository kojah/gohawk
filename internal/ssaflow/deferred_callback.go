package ssaflow

import (
	"maps"

	"github.com/kojah/gohawk/internal/syntax"

	"golang.org/x/tools/go/ssa"
)

var syncOnceFunc = syntax.PackageFunction("sync", "OnceFunc")

// Deferred bindings are observed when the deferred callee runs, after any
// assignment that follows the defer. These helpers recover the one value an
// addressable capture or stored callback can hold at that point, and refuse
// captures with several stores or a store that does not dominate the defer.

//nolint:ireturn // SSA bindings have several concrete forms.
func deferredBindingValue(binding, target ssa.Value, invocation ssa.Instruction) (ssa.Value, bool) {
	if valueHasDirectStore(binding) {
		// Addressable captures must use the unique-store proof below. General
		// identity traversal may otherwise match one historical value while the
		// deferred callback observes a later reassignment.
		return uniquelyStoredValueBefore(binding, invocation)
	}
	if SameValue(binding, target) || ValueIsAccessPathFrom(target, binding) {
		return binding, true
	}
	stored, ok := uniquelyStoredValueBefore(binding, invocation)
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

//nolint:ireturn // Stored callbacks and captures may be any SSA value.
func uniquelyStoredValueBefore(address ssa.Value, invocation ssa.Instruction) (ssa.Value, bool) {
	if address == nil || address.Referrers() == nil {
		return nil, false
	}
	var selected *ssa.Store
	for _, reference := range *address.Referrers() {
		store, ok := reference.(*ssa.Store)
		if !ok || store.Addr != address {
			continue
		}
		if selected != nil || !InstructionDominates(store, invocation) {
			return nil, false
		}
		selected = store
	}
	if selected == nil {
		return nil, false
	}
	return selected.Val, true
}

func cloneValueSet(source map[ssa.Value]bool) map[ssa.Value]bool {
	result := make(map[ssa.Value]bool, len(source))
	maps.Copy(result, source)
	return result
}
