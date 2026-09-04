package lockorder

import (
	"go/types"
	"slices"

	"github.com/kojah/gohawk/internal/check"
	"github.com/kojah/gohawk/internal/ssaflow"

	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/ssa"
)

// A read lock is shared: sync.RWMutex documents RLock as held by any number of
// readers at once. Writing to the object that lock protects while holding only
// RLock therefore races with every other reader, and unlike an order inversion
// it needs nothing unusual from a second goroutine -- two ordinary readers are
// enough.
//
// The claim is deliberately about the OBJECT, not about which field the lock
// guards. Proving that a mutex guards a particular field needs guard inference
// this analyzer does not do, and one struct may hold several independent guard
// domains: a mutex over one map, an atomic counter, and fields fixed at
// construction. What is provable without that inference is narrower: the
// receiver whose read lock is held is the receiver being mutated.
//
// Two exclusions follow from the same rule and keep the claim honest. A value
// LOADED out of the owner is a different cell, so mutating it is not a write to
// the owner -- the distinction ssaflow.IdentitySource states for identity
// resolution. And an atomic update is a call rather than a store, so it never
// reaches here at all, which is correct: such a field is protected by atomics
// rather than by the lock.

// reportReadLockWrites reports a write to an object whose read lock is the only
// one held at this instruction.
func reportReadLockWrites(pass *analysis.Pass, instruction ssa.Instruction, held, readHeld []string, lockValues map[string][]ssa.Value) {
	for _, identity := range readHeld {
		// A lock the flow already transferred or released is no longer held,
		// even if it was taken for reading earlier on this path.
		if !slices.Contains(held, identity) {
			continue
		}
		for _, value := range lockValues[identity] {
			owner, ok := lockOwner(value)
			if !ok || !writeTargetsOwner(instruction, owner) {
				continue
			}
			// One struct may hold several independent guard domains: a mutex
			// over a map, an atomic counter, and a second mutex over its own
			// field. Deciding which of them covers this field is the guard
			// inference this check does not do, so any write lock held on the
			// same object leaves the write unproven rather than reportable.
			if writeLockHeldOnOwner(held, readHeld, owner, lockValues) {
				continue
			}
			check.Reportf(pass, check.LockReadLockWrite, instruction.Pos(),
				"write while only the read lock %s is held", identity)
			return
		}
	}
}

// writeLockHeldOnOwner reports whether some lock held for writing belongs to
// the same object as the read lock being judged.
func writeLockHeldOnOwner(held, readHeld []string, owner ssa.Value, lockValues map[string][]ssa.Value) bool {
	for _, identity := range held {
		if slices.Contains(readHeld, identity) {
			continue
		}
		for _, value := range lockValues[identity] {
			if other, ok := lockOwner(value); ok && ssaflow.SameValue(other, owner) {
				return true
			}
		}
	}
	return false
}

// lockOwner returns the value a lock is a field of. A package variable has no
// owner object, so what it protects is not decided here.
func lockOwner(value ssa.Value) (ssa.Value, bool) { //nolint:ireturn // SSA values have several concrete forms.
	field, ok := value.(*ssa.FieldAddr)
	if !ok {
		return nil, false
	}
	return field.X, true
}

func writeTargetsOwner(instruction ssa.Instruction, owner ssa.Value) bool {
	switch typed := instruction.(type) {
	case *ssa.Store:
		return addressWithinOwner(typed.Addr, owner)
	case *ssa.MapUpdate:
		// The map is loaded out of the owner's field, so peel that one load to
		// name the cell it came from before requiring the field path.
		source, ok := ssaflow.IdentitySource(typed.Map)
		return ok && addressWithinOwner(source, owner)
	case *ssa.Call:
		return mutatingBuiltinTargetsOwner(typed.Common(), owner)
	}
	return false
}

// mutatingBuiltinTargetsOwner reports whether a builtin that mutates its
// argument writes into the owner. These are calls rather than stores, so they
// reach none of the cases above, and a map emptied or a slice overwritten
// under a read lock races exactly as an assignment to it does.
//
// copy takes its destination first and reads its source, so only the first
// argument is a write: counting the source would report a copy out of the
// owner, which is the read a read lock permits.
func mutatingBuiltinTargetsOwner(common *ssa.CallCommon, owner ssa.Value) bool {
	builtin, ok := common.Value.(*ssa.Builtin)
	if !ok || len(common.Args) == 0 {
		return false
	}
	switch builtin.Name() {
	case "delete", "clear", "copy":
	default:
		return false
	}
	// The container is loaded out of the owner's field, so peel that one load
	// to name the cell it came from, exactly as the map update does.
	source, found := ssaflow.IdentitySource(common.Args[0])
	return found && addressWithinOwner(source, owner)
}

// addressWithinOwner reports whether address selects a field or a constant
// index of owner. It never peels a load, so a value read out of the owner and
// then mutated is not counted as a write to the owner.
func addressWithinOwner(address, owner ssa.Value) bool {
	for address != nil {
		if ssaflow.SameValue(address, owner) {
			return true
		}
		switch typed := address.(type) {
		case *ssa.FieldAddr:
			address = typed.X
		case *ssa.IndexAddr:
			// A read lock is shared, so two readers race only where they write
			// the SAME cell. Distinct elements are distinct memory, so an
			// element write at a caller-supplied index races only if two
			// readers can pass the same index -- which this analysis cannot
			// establish, and which a per-consumer cursor deliberately never
			// does. pyroscope's Tee hands each consumer its own index and
			// advances that cursor under the read lock:
			// https://github.com/grafana/pyroscope/blob/d1212251265e7dab4b03ef0d80af565f6d519e1b/pkg/iter/tee.go#L68-L76
			//
			// A constant index names one cell every reader shares, so it stays
			// reportable, as does a map update, which races whatever the key,
			// and a store to the field itself.
			if _, constant := typed.Index.(*ssa.Const); !constant {
				return false
			}
			// A slice header loaded out of the owner shares the owner's backing
			// array, so writing through it writes the owner. A pointer loaded
			// out of the owner is a different object, which is why only the
			// slice case peels its load.
			address = typed.X
			if _, slice := address.Type().Underlying().(*types.Slice); slice {
				if source, ok := ssaflow.IdentitySource(address); ok {
					address = source
				}
			}
		default:
			return false
		}
	}
	return false
}
