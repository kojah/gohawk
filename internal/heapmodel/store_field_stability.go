package heapmodel

import (
	"go/token"
	"strings"

	"github.com/kojah/gohawk/internal/ssaflow"
	"golang.org/x/tools/go/ssa"
)

// StableFieldContent proves the contents of a fresh owner's embedded field
// remain unchanged through every visible use, including observation itself.
// Unlike StableContent, unrelated sibling fields are outside the question.
// Known asynchronous readers may read this slot but must not retain its
// address or write it. This says nothing about mutation of the loaded object.
func (storage *Storage) StableFieldContent(address ssa.Value, observation ssa.Instruction) StoredValue {
	path, known := ssaflow.ResolveEmbeddedFieldPath(ssaflow.NewReachingWalk(ssaflow.TransparentNone), address,
		func(root ssa.Value) bool { _, fresh := root.(*ssa.Alloc); return fresh })
	location, local := storage.location(address)
	if !known || path.Depth == 0 || !local || observation == nil || path.Root.Parent() != observation.Parent() {
		return storage.unknown(ssaflow.EvidenceStorageNotLocal, observation)
	}
	var stores []*ssa.Store
	if blocked, ok := storage.collectField(path, &stores); !ok {
		return storage.unknown(ssaflow.EvidenceStorageAddressEscapes, blocked)
	}
	for _, store := range stores {
		written, exact := storage.location(store.Addr)
		if !exact || written.path != location.path && !strings.HasPrefix(location.path, written.path+"/") {
			return storage.unknown(ssaflow.EvidenceStoragePartialWrite, store)
		}
		if store == observation || StoreMayFollow(location.root, observation, store) || ssaflow.BlockInCycle(store.Block()) {
			return storage.unknown(ssaflow.EvidenceStorageWriteAfterObservation, store)
		}
	}
	return storage.reachingContent(location, observation, stores)
}

// Inspect all address uses, not only those before observation: a worker may
// read later. The field-effect query preserves whole-owner escapes and writes
// while proving that operations through a different embedded field are disjoint.
func (storage *Storage) collectField(path ssaflow.EmbeddedFieldPath, stores *[]*ssa.Store) (ssa.Instruction, bool) {
	if path.Root.Referrers() == nil {
		return nil, false
	}
	for _, use := range *path.Root.Referrers() {
		if !storage.budget.Spend() {
			return use, false
		}
		switch use := use.(type) {
		case *ssa.FieldAddr:
			if path.Depth == 0 {
				return use, false
			}
			if use.Field != path.Fields[0] {
				continue
			}
			child := ssaflow.EmbeddedFieldPath{Root: use, Depth: path.Depth - 1}
			copy(child.Fields[:], path.Fields[1:path.Depth])
			if blocked, ok := storage.collectField(child, stores); !ok {
				return blocked, false
			}
		case *ssa.Store:
			if use.Addr != path.Root {
				return use, false
			}
			*stores = append(*stores, use)
		default:
			if !storage.readOnlyFieldUse(path, use) {
				return use, false
			}
		}
	}
	return nil, true
}

func (storage *Storage) readOnlyFieldUse(path ssaflow.EmbeddedFieldPath, use ssa.Instruction) bool {
	switch use := use.(type) {
	case *ssa.DebugRef:
		return true
	case *ssa.UnOp:
		return use.Op == token.MUL
	case *ssa.Call, *ssa.Go, *ssa.Defer:
		return storage.effects.FieldCall(use, path).PreservesField()
	}
	return false
}
