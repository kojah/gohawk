package heapmodel

import (
	proofs "github.com/kojah/gohawk/internal/proof"
	"github.com/kojah/gohawk/internal/ssaflow"
	"golang.org/x/tools/go/ssa"
)

// StableContent adds a lifetime check to Content: no other use may change the
// selected location after observation. The observation's own effects are
// excluded, as for argument evaluation; callers mapping a callback must prove
// that callback's accesses read-only. Use this for captured cells, not ordinary
// loads whose values are snapshots.
// Sidecar re-queries into one cell before deferring Close; selecting the
// latest store is sound only when it remains stable through deferred use:
// https://github.com/marcus/sidecar/blob/9b8739f753ab235dda2630676833e9b46a52696c/internal/adapter/warp/adapter.go#L337-L341
func (storage *Storage) StableContent(address ssa.Value, observation ssa.Instruction) StoredValue {
	location, ok := storage.location(address)
	if !ok {
		return storage.unknown(proofs.EvidenceStorageNotLocal, observation)
	}
	if observation == nil || location.root.Parent() != observation.Parent() {
		return storage.unknown(proofs.EvidenceStorageOutsideFunction, observation)
	}
	var stores []*ssa.Store
	if blocked, ok := storage.collect(location.root, observation, &stores, true); !ok {
		return storage.unknown(proofs.EvidenceStorageAddressEscapes, blocked)
	}
	for _, store := range stores {
		written, ok := storage.location(store.Addr)
		if !ok {
			return storage.unknown(proofs.EvidenceStorageWriteThroughAlias, store)
		}
		if !slotBeneath(location.path, written.path) && !slotBeneath(written.path, location.path) {
			continue
		}
		follows := StoreMayFollowWithin(location.root, observation, store, storage.budget)
		if storage.budget.Exhausted() || storage.budget.PoolExhausted() {
			return storage.unknown(proofs.EvidenceBudgetExhausted, store)
		}
		if follows || ssaflow.BlockInCycle(store.Block()) && store.Block() != location.root.Block() {
			return storage.unknown(proofs.EvidenceStorageWriteAfterObservation, store)
		}
	}
	return storage.content(location, observation)
}

// SliceOnlyObserved reports whether use constructs a slice whose only consumers
// are observation. The caller must account for observation's own effects.
func SliceOnlyObserved(use, observation ssa.Instruction, budget *proofs.SearchBudget) bool {
	slice, ok := use.(*ssa.Slice)
	if !ok || slice.Referrers() == nil {
		return false
	}
	for _, consumer := range *slice.Referrers() {
		if !budget.Spend() || consumer != observation {
			return false
		}
	}
	return true
}
