package cancellationownership

import (
	proofs "github.com/kojah/gohawk/internal/proof"
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
	// holds are the instructions that only move the cancel into the owner:
	// the field store, its field address, a capture cell's store, and the
	// owner's other field addresses.
	holds map[ssa.Instruction]bool
}

// cancellationOwnerProof preserves availability independently of a missing
// constructor owner. A request-local cutoff is retained only by this classifier;
// a fresh cancellation proof creates a new classifier and can search again.
type cancellationOwnerProof struct {
	proofs.Proof
	Owner *cancellationOwner
}

func (classifier *cancellationClassifier) cancellationOwner() cancellationOwnerProof {
	if classifier.owner == nil {
		proof := proveCancellationOwnerWithin(classifier.cancel, classifier.budget())
		classifier.owner = &proof
	}
	return *classifier.owner
}

func (classifier *cancellationClassifier) ownerReturnLabel(returned *ssa.Return) cancellationLabel {
	proof := classifier.cancellationOwner()
	if !proof.Known() {
		return labelled(cancellationActionUnknown, reasonLabelOwnerUnavailable)
	}
	// The completed owner-use census marks a return only when it directly uses
	// this exact allocation. Reuse that evidence rather than scan results again.
	if proof.Owner != nil && proof.Owner.holds[returned] {
		return labelled(cancellationActionTransfer, reasonLabelReturnedOwner)
	}
	return cancellationLabel{}
}

// proveCancellationOwnerWithin establishes the narrow constructor-owner contract.
// Interrupted censuses discard every hold and remain unavailable, not ownerless.
func proveCancellationOwnerWithin(cancel ssa.Value, budget *proofs.SearchBudget) (proof cancellationOwnerProof) {
	proof.Proof = proofs.Proof{State: proofs.EvidenceDisproven, Reason: proofs.EvidenceNotFound}
	defer func() {
		if budget.Exhausted() || budget.PoolExhausted() {
			proof = cancellationOwnerProof{Proof: proofs.Proof{Reason: proofs.EvidenceBudgetExhausted}}
		}
	}()
	if !budget.Spend() || cancel == nil || cancel.Referrers() == nil {
		return proof
	}
	holds := map[ssa.Instruction]bool{}
	var field *ssa.FieldAddr
	for user := range ssaflow.ReferrersWithin(cancel, budget) {
		store, ok := user.(*ssa.Store)
		if !ok || store.Val != cancel {
			continue
		}
		target, carried := ownerField(store, cancel, holds, budget)
		if target == nil || field != nil && target != field {
			return proof
		}
		field = target
		if carried {
			holds[store] = true
		}
	}
	if field == nil {
		return proof
	}
	alloc, ok := field.X.(*ssa.Alloc)
	if !ok || !alloc.Heap || !ownerUsesVisible(alloc, field, holds, budget) {
		return proof
	}
	proof.Owner = &cancellationOwner{holds: holds}
	proof.Proof = proofs.Proof{State: proofs.EvidenceProven, Reason: proofs.EvidenceOwnerStoredInField}
	return proof
}

// ownerField returns the owner field a store of the cancel reaches: the field
// address the cancel is stored into, or the one a capturing closure is stored
// into. carried reports that the store itself only moves the cancel there.
func ownerField(store *ssa.Store, cancel ssa.Value, holds map[ssa.Instruction]bool, budget *proofs.SearchBudget) (*ssa.FieldAddr, bool) {
	if field, ok := store.Addr.(*ssa.FieldAddr); ok {
		// The field address must serve only this store, so no other write
		// through it can replace the cancel before the owner is returned.
		if !onlyUseWithin(field, store, budget) {
			return nil, false
		}
		holds[field] = true
		return field, true
	}
	cell, ok := store.Addr.(*ssa.Alloc)
	if !ok {
		return nil, false
	}
	return closureOwnerField(store, cell, cancel, holds, budget)
}

// closureOwnerField handles a cancel captured by a closure. SSA boxes a
// captured variable in a cell, so the closure calls whatever the cell holds
// when it runs. The cell must be written exactly once, with the cancel, and
// that store must precede the one closure that captures it; a second write,
// a write after capture, or a read before the store would let the closure
// call something else. The closure itself must have no use but being stored
// into one owner field, or the cancel could be run or kept elsewhere.
func closureOwnerField(
	store *ssa.Store, cell *ssa.Alloc, cancel ssa.Value, holds map[ssa.Instruction]bool, budget *proofs.SearchBudget,
) (*ssa.FieldAddr, bool) {
	var closure *ssa.MakeClosure
	for user := range ssaflow.ReferrersWithin(cell, budget) {
		switch typed := user.(type) {
		case *ssa.Store:
			if typed != store {
				return nil, false
			}
		case *ssa.MakeClosure:
			if closure != nil {
				return nil, false
			}
			closure = typed
		default:
			return nil, false
		}
	}
	if closure == nil {
		return nil, false
	}
	if stored, once := ssaflow.WrittenOnceCellAtWithin(cell, closure, budget); !once || stored != cancel {
		return nil, false
	}
	if !budget.Spend() || closure.Referrers() == nil || len(*closure.Referrers()) != 1 {
		return nil, false
	}
	closureStore, ok := (*closure.Referrers())[0].(*ssa.Store)
	if !ok || closureStore.Val != closure {
		return nil, false
	}
	field, ok := closureStore.Addr.(*ssa.FieldAddr)
	if !ok || !onlyUseWithin(field, closureStore, budget) {
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
func ownerUsesVisible(alloc *ssa.Alloc, cancelField *ssa.FieldAddr, holds map[ssa.Instruction]bool, budget *proofs.SearchBudget) bool {
	for user := range ssaflow.ReferrersWithin(alloc, budget) {
		switch typed := user.(type) {
		case *ssa.FieldAddr:
			if typed.Field == cancelField.Field {
				if typed != cancelField {
					return false
				}
				continue
			}
			holds[typed] = true
			for access := range ssaflow.ReferrersWithin(typed, budget) {
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

func onlyUseWithin(value ssa.Value, user ssa.Instruction, budget *proofs.SearchBudget) bool {
	return budget.Spend() && value.Referrers() != nil && len(*value.Referrers()) == 1 && (*value.Referrers())[0] == user
}
