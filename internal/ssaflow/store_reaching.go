package ssaflow

import (
	"strings"

	"golang.org/x/tools/go/ssa"
)

// A backwards query stops at the first write on each predecessor path. Earlier
// assignments cannot defeat an unconditional overwrite, and joins retain only
// one agreed value. Cycles and missing initializations remain unknown.
func (storage *Storage) reachingContent(location storageLocation, observation ssa.Instruction, stores []*ssa.Store) StoredValue {
	writes := make(map[*ssa.Store]storageWrite)
	for _, store := range stores {
		written, ok := storage.location(store.Addr)
		if !ok || written.root != location.root {
			return storage.unknown(EvidenceStorageWriteThroughAlias, store)
		}
		if written.path == location.path || strings.HasPrefix(location.path, written.path+"/") {
			writes[store] = storageWrite{suffix: strings.TrimPrefix(location.path, written.path)}
		} else if strings.HasPrefix(written.path, location.path+"/") {
			writes[store] = storageWrite{partial: true}
		}
	}
	// A sole dominating initializer remains valid across unrelated loops.
	// There is no competing write to discover by walking their backedges.
	// A callback defined outside a range loop can therefore be bound inside it:
	// https://github.com/mariadb-operator/mariadb-operator/blob/e8ece7a8076954674e10e0381571bd80278ac35f/licenses/go-licenses/github.com/go-sql-driver/mysql/driver_test.go#L191-L205
	if len(writes) == 1 {
		for store, write := range writes {
			if !write.partial && InstructionDominates(store, observation) {
				return storage.projectStored(store.Val, write.suffix)
			}
		}
	}
	query := reachingStorage{storage: storage, location: location, writes: writes, active: make(map[*ssa.BasicBlock]bool)}
	return query.before(observation.Block(), InstructionIndex(observation))
}

// A field write invalidates a whole-aggregate value. It cannot be mistaken for
// that aggregate's previous initializer, nor for a write to a sibling field.
type storageWrite struct {
	suffix  string
	partial bool
}

type reachingStorage struct {
	storage  *Storage
	location storageLocation
	writes   map[*ssa.Store]storageWrite
	active   map[*ssa.BasicBlock]bool
}

func (query *reachingStorage) before(block *ssa.BasicBlock, index int) StoredValue {
	if block == nil {
		return query.storage.unknown(EvidenceStorageNoReachingWrite, nil)
	}
	if query.active[block] {
		return query.storage.unknown(EvidenceStorageWriteInCycle, block.Instrs[len(block.Instrs)-1])
	}
	if !query.storage.budget.Spend() {
		return query.storage.unknown(EvidenceBudgetExhausted, nil)
	}
	query.active[block] = true
	defer delete(query.active, block)
	for i := index - 1; i >= 0; i-- {
		if !query.storage.budget.Spend() {
			return query.storage.unknown(EvidenceBudgetExhausted, nil)
		}
		if block.Instrs[i] == query.location.root {
			return query.storage.unknown(EvidenceStorageNoReachingWrite, query.location.root)
		}
		if store, ok := block.Instrs[i].(*ssa.Store); ok {
			if write, relevant := query.writes[store]; relevant {
				if write.partial {
					return query.storage.unknown(EvidenceStoragePartialWrite, store)
				}
				return query.storage.projectStored(store.Val, write.suffix)
			}
		}
	}
	var agreed StoredValue
	for _, predecessor := range block.Preds {
		incoming := query.before(predecessor, len(predecessor.Instrs))
		if !incoming.Proven() {
			return incoming
		}
		if agreed.Proven() && !query.storage.Same(agreed.Value, incoming.Value).Proven() {
			return query.storage.unknown(EvidenceStorageConflictingWrites, predecessor.Instrs[len(predecessor.Instrs)-1])
		}
		agreed = incoming
	}
	if !agreed.Proven() {
		return query.storage.unknown(EvidenceStorageNoReachingWrite, nil)
	}
	return agreed
}
