package cancellationownership

import (
	"slices"

	"github.com/kojah/gohawk/internal/ssaflow"
	"golang.org/x/tools/go/ssa"
)

// A constructor commonly stores its cancel function in a field of the struct
// it returns, directly or as a closure that calls it, and returns early with
// an error before that struct reaches the caller. The struct owns the cancel:
// returning it hands the cancel to the caller, and a return that drops it
// leaves the cancel uncalled. hermesx and similar constructors store a
// background cancel only on the success path this way:
// https://github.com/Colin4k1024/hermesx/commit/8f73c6ac5dec8c1a3f04da41808698b18061d60c
//
// The rule is deliberately narrow, because the owner can only be judged when
// every use of it is visible. The owner is a struct allocated in this
// function; the cancel reaches exactly one of its fields, either as the value
// itself or as a closure that captures a cell written once with the cancel
// and has no other use; and the owner is used only through field addresses
// and returns. Any other use, such as passing the owner to a function,
// publishing it, or capturing it, leaves the ordinary classification in
// place, which makes the obligation unknown.

type cancellationOwner struct {
	alloc *ssa.Alloc
	// holds are the instructions that only move the cancel into the owner:
	// the field store, its field address, a capture cell's store, and the
	// owner's other field addresses.
	holds map[ssa.Instruction]bool
}

// ownerHolds reports whether the instruction only moves the cancel into the
// owner or addresses one of the owner's fields.
func (classifier *cancellationClassifier) ownerHolds(instruction ssa.Instruction) bool {
	owner := classifier.cancellationOwner()
	return owner != nil && owner.holds[instruction]
}

// returnsOwner reports whether a return hands the owner to the caller.
func (classifier *cancellationClassifier) returnsOwner(returned *ssa.Return) bool {
	owner := classifier.cancellationOwner()
	return owner != nil && slices.ContainsFunc(returned.Results, func(result ssa.Value) bool { return result == owner.alloc })
}

func (classifier *cancellationClassifier) cancellationOwner() *cancellationOwner {
	if !classifier.ownerProved {
		classifier.ownerProved = true
		classifier.owner = findCancellationOwner(classifier.cancel)
	}
	return classifier.owner
}

// findCancellationOwner returns the owner the cancel is stored into, or nil
// when the cancel's uses do not match the rule exactly.
func findCancellationOwner(cancel ssa.Value) *cancellationOwner {
	if cancel == nil || cancel.Referrers() == nil {
		return nil
	}
	holds := map[ssa.Instruction]bool{}
	var field *ssa.FieldAddr
	for _, user := range *cancel.Referrers() {
		store, ok := user.(*ssa.Store)
		if !ok || store.Val != cancel {
			continue
		}
		target, carried := ownerField(store, cancel, holds)
		if target == nil || field != nil && target != field {
			return nil
		}
		field = target
		if carried {
			holds[store] = true
		}
	}
	if field == nil {
		return nil
	}
	alloc, ok := field.X.(*ssa.Alloc)
	if !ok || !alloc.Heap || !ownerUsesVisible(alloc, field, holds) {
		return nil
	}
	return &cancellationOwner{alloc: alloc, holds: holds}
}

// ownerField returns the owner field a store of the cancel reaches: the field
// address the cancel is stored into, or the one a capturing closure is stored
// into. carried reports that the store itself only moves the cancel there.
func ownerField(store *ssa.Store, cancel ssa.Value, holds map[ssa.Instruction]bool) (*ssa.FieldAddr, bool) {
	if field, ok := store.Addr.(*ssa.FieldAddr); ok {
		// The field address must serve only this store, so no other write
		// through it can replace the cancel before the owner is returned.
		if !onlyUse(field, store) {
			return nil, false
		}
		holds[field] = true
		return field, true
	}
	cell, ok := store.Addr.(*ssa.Alloc)
	if !ok {
		return nil, false
	}
	return closureOwnerField(store, cell, cancel, holds)
}

// closureOwnerField handles a cancel captured by a closure. SSA boxes a
// captured variable in a cell, so the closure calls whatever the cell holds
// when it runs. The cell must be written exactly once, with the cancel, and
// that store must precede the one closure that captures it; a second write,
// a write after capture, or a read before the store would let the closure
// call something else. The closure itself must have no use but being stored
// into one owner field, or the cancel could be run or kept elsewhere.
func closureOwnerField(store *ssa.Store, cell *ssa.Alloc, cancel ssa.Value, holds map[ssa.Instruction]bool) (*ssa.FieldAddr, bool) {
	if stored, once := ssaflow.WrittenOnceCell(cell); !once || stored != cancel {
		return nil, false
	}
	var closure *ssa.MakeClosure
	for _, user := range *cell.Referrers() {
		switch typed := user.(type) {
		case *ssa.Store:
			if typed != store {
				return nil, false
			}
		case *ssa.MakeClosure:
			if closure != nil || !ssaflow.InstructionDominates(store, typed) {
				return nil, false
			}
			closure = typed
		default:
			return nil, false
		}
	}
	if closure == nil || closure.Referrers() == nil || len(*closure.Referrers()) != 1 {
		return nil, false
	}
	closureStore, ok := (*closure.Referrers())[0].(*ssa.Store)
	if !ok || closureStore.Val != closure {
		return nil, false
	}
	field, ok := closureStore.Addr.(*ssa.FieldAddr)
	if !ok || !onlyUse(field, closureStore) {
		return nil, false
	}
	holds[closure] = true
	holds[closureStore] = true
	holds[field] = true
	return field, true
}

// ownerUsesVisible reports whether every use of the owner is a field address
// or a return. It marks the returns, and the addresses of the owner's other
// fields with the plain stores and loads through them, as holding nothing but
// the owner: writing or reading another field does not touch the cancel. A
// use of the cancel's own field, such as a load that is later called, stays
// with the ordinary classification.
func ownerUsesVisible(alloc *ssa.Alloc, cancelField *ssa.FieldAddr, holds map[ssa.Instruction]bool) bool {
	for _, user := range *alloc.Referrers() {
		switch typed := user.(type) {
		case *ssa.FieldAddr:
			if typed.Field == cancelField.Field {
				if typed != cancelField {
					return false
				}
				continue
			}
			holds[typed] = true
			for _, access := range *typed.Referrers() {
				if plainFieldAccess(access, typed) {
					holds[access] = true
				}
			}
		case *ssa.Return:
			holds[typed] = true
		case *ssa.DebugRef:
		default:
			return false
		}
	}
	return true
}

// plainFieldAccess reports whether an instruction only stores into or loads
// from the field address.
func plainFieldAccess(access ssa.Instruction, field *ssa.FieldAddr) bool {
	switch typed := access.(type) {
	case *ssa.Store:
		return typed.Addr == field
	case *ssa.UnOp:
		return typed.X == field
	}
	return false
}

func onlyUse(value ssa.Value, user ssa.Instruction) bool {
	return value.Referrers() != nil && len(*value.Referrers()) == 1 && (*value.Referrers())[0] == user
}
