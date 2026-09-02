package ssaflow

import (
	"go/token"
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
		// Addressable captures must use the store proofs below. General
		// identity traversal may otherwise match one historical value while the
		// deferred callback observes a later reassignment.
		if targetStoredOnPath(binding, target, invocation) {
			return target, true
		}
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
		if !SameValue(candidate.Val, target) || !InstructionMayFollow(candidate, observation) {
			continue
		}
		intervening := false
		for _, other := range stores {
			if other == candidate {
				continue
			}
			if storeMayFollow(address, observation, other) || InstructionMayFollow(candidate, other) && InstructionMayFollow(other, observation) {
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
		if !InstructionDominates(store, observation) || storeMayFollow(address, observation, store) {
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

// storeMayFollow reports whether the store can run after the observation on
// the same cell. A local declared inside a loop is a fresh allocation each
// iteration, so a store reached only by re-executing the allocation writes a
// different cell and does not reassign the observed one. cb-spider retries a
// request in a loop and defers the body close inside each iteration:
// https://github.com/cloud-barista/cb-spider/blob/5aa6bd8a8a09003dc168ac78f6ea987617de9d31/cloud-control-manager/cloud-driver/drivers/ibm/resources/PriceInfoHandler.go#L153-L169
func storeMayFollow(address ssa.Value, observation ssa.Instruction, store *ssa.Store) bool {
	allocation, ok := address.(*ssa.Alloc)
	if !ok {
		return InstructionMayFollow(observation, store)
	}
	if observation.Block() == store.Block() {
		return InstructionIndex(observation) <= InstructionIndex(store)
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

// appendOnlyCell reports whether every store into the cell writes the result
// of appending to the cell's own current value, so the cell only grows. Such
// a cell holds, at any later point, everything ever appended to it.
func appendOnlyCell(address ssa.Value) bool {
	if address == nil || address.Referrers() == nil {
		return false
	}
	stores := 0
	for _, reference := range *address.Referrers() {
		store, ok := reference.(*ssa.Store)
		if !ok || store.Addr != address {
			continue
		}
		stores++
		if !appendsToCell(store.Val, address) {
			return false
		}
	}
	return stores > 0
}

func appendsToCell(value, address ssa.Value) bool {
	call, ok := value.(*ssa.Call)
	if !ok {
		return false
	}
	builtin, ok := call.Common().Value.(*ssa.Builtin)
	if !ok || builtin.Name() != "append" || len(call.Common().Args) == 0 {
		return false
	}
	load, ok := call.Common().Args[0].(*ssa.UnOp)
	return ok && load.Op == token.MUL && load.X == address
}

// targetOrNilCell reports whether every store into the cell writes the target
// or a nil constant, with at least one store of the target.
func targetOrNilCell(address, target ssa.Value) bool {
	if address == nil || address.Referrers() == nil {
		return false
	}
	storesTarget := false
	for _, reference := range *address.Referrers() {
		store, ok := reference.(*ssa.Store)
		if !ok || store.Addr != address {
			continue
		}
		switch {
		case SameValue(store.Val, target):
			storesTarget = true
		case DefinitelyNil(store.Val):
		default:
			return false
		}
	}
	return storesTarget
}
