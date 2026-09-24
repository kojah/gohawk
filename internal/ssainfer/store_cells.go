package ssainfer

import (
	"maps"

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
	if MayAlias(binding, target) || ssaflow.ValueIsAccessPathFrom(target, binding) {
		return binding, true
	}
	return NewStorage(nil).stableValue(binding, invocation)
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
		if !MayAlias(candidate.Val, target) || !ssaflow.InstructionMayFollow(candidate, observation) {
			continue
		}
		intervening := false
		for _, other := range stores {
			if other == candidate {
				continue
			}
			if storeMayFollow(address, observation, other) ||
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

// storeMayFollow reports whether the store can run after the observation on
// the same cell. A local declared inside a loop is a fresh allocation each
// iteration, so a store reached only by re-executing the allocation writes a
// different cell and does not reassign the observed one. cb-spider retries a
// request in a loop and defers the body close inside each iteration:
// https://github.com/cloud-barista/cb-spider/blob/5aa6bd8a8a09003dc168ac78f6ea987617de9d31/cloud-control-manager/cloud-driver/drivers/ibm/resources/PriceInfoHandler.go#L153-L169
func storeMayFollow(address ssa.Value, observation ssa.Instruction, store *ssa.Store) bool {
	allocation, ok := address.(*ssa.Alloc)
	if !ok {
		return ssaflow.InstructionMayFollow(observation, store)
	}
	if observation.Block() == store.Block() {
		return ssaflow.InstructionIndex(observation) <= ssaflow.InstructionIndex(store)
	}
	seen := map[*ssa.BasicBlock]bool{allocation.Block(): true}
	queue := append([]*ssa.BasicBlock(nil), observation.Block().Succs...)
	for len(queue) > 0 {
		block := queue[0]
		queue = queue[1:]
		if seen[block] {
			// The allocation's block is never entered: reaching a store through
			// it means the allocation ran again and the store writes a new cell.
			continue
		}
		if block == store.Block() {
			return true
		}
		seen[block] = true
		queue = append(queue, block.Succs...)
	}
	return false
}

func cloneValueSet(source map[ssa.Value]bool) map[ssa.Value]bool {
	result := make(map[ssa.Value]bool, len(source))
	maps.Copy(result, source)
	return result
}
