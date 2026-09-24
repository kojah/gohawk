package lifecycle

import (
	"maps"

	"github.com/kojah/gohawk/internal/heapmodel"
	"github.com/kojah/gohawk/internal/ssaflow"
	"github.com/kojah/gohawk/internal/syntax"
	"golang.org/x/tools/go/ssa"
)

var syncOnceFunc = syntax.PackageFunction("sync", "OnceFunc")

// Deferred bindings are observed when the deferred callee runs, after any
// assignment that follows the defer. These helpers recover the one value an
// addressable capture or stored callback holds at that point. Stable storage
// queries require agreeing incoming writes and no later mutation; specialized
// target-relative proofs also account for conditional acquisition paths.

// deferredBindingValue recovers the value a non-cell binding stands for
// when the deferred callee runs. A captured cell is read by the points-to
// graph instead; see deferredCellLocal.
//
//nolint:ireturn // SSA bindings have several concrete forms.
func deferredBindingValue(binding, target ssa.Value, invocation ssa.Instruction) (ssa.Value, bool) {
	if heapmodel.MayAlias(binding, target) || ssaflow.ValueIsAccessPathFrom(target, binding) {
		return binding, true
	}
	stored := heapmodel.NewStorage(nil).StableContent(binding, invocation)
	return stored.Value, stored.Proven()
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

// targetStoredOnPath reports whether a store of the target itself reaches the
// observation with no other store between them and none after. The target is
// defined where it is stored, so every path on which it is live passes that
// store; a resource acquired in one branch of an if/else and closed by a
// deferred literal after the merge is settled this way even though the store
// does not dominate the defer. traefikoidc assigns a response in either a
// retry callback or a direct call before deferring its close:
// https://github.com/lukaszraczylo/traefikoidc/blob/61e60733a5be38428dee42eed626490f9609dad6/token_introspection.go#L84-L115
func targetStoredOnPath(address, target ssa.Value, observation ssa.Instruction) bool {
	if address == nil || address.Referrers() == nil {
		return false
	}
	var stores []*ssa.Store
	for _, reference := range *address.Referrers() {
		store, ok := reference.(*ssa.Store)
		if ok && store.Addr == address {
			stores = append(stores, store)
		}
	}
	for _, candidate := range stores {
		if !heapmodel.MayAlias(candidate.Val, target) || !ssaflow.InstructionMayFollow(candidate, observation) {
			continue
		}
		intervening := false
		for _, other := range stores {
			if other == candidate {
				continue
			}
			if heapmodel.StoreMayFollow(address, observation, other) ||
				ssaflow.InstructionMayFollow(candidate, other) && ssaflow.InstructionMayFollow(other, observation) {
				intervening = true
				break
			}
		}
		if !intervening {
			return true
		}
	}
	return false
}

func cloneValueSet(source map[ssa.Value]bool) map[ssa.Value]bool {
	result := make(map[ssa.Value]bool, len(source))
	maps.Copy(result, source)
	return result
}
