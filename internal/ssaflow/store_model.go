package ssaflow

import (
	"go/token"
	"strconv"

	"golang.org/x/tools/go/ssa"
)

// Storage answers point-in-time questions about local storage. It follows
// exact fields, constant array indexes, and aggregate copies, never possible
// aliases. Ambiguous writes and address escapes make the answer unknown.
// Each query owns its budget; no state is shared between analyzed functions.
type Storage struct {
	budget  *SearchBudget
	effects *CallEffects
}

// StoredValue records the value proved to occupy a location. Unknown does not
// mean empty, unequal, or released, and must not establish a lifecycle action.
type StoredValue struct {
	Proof
	Value ssa.Value
}

// NewStorage creates a bounded storage query using the caller's search budget.
func NewStorage(budget *SearchBudget) *Storage {
	if budget == nil {
		budget = NewSearchBudget(1000)
	}
	return &Storage{budget: budget, effects: NewCallEffects(budget)}
}

type storageLocation struct {
	root *ssa.Alloc
	path string
}

// Resolve follows loads at their own execution points, not at a later use.
// This preserves a saved value when its original cell is subsequently changed.
func (storage *Storage) Resolve(value ssa.Value) StoredValue {
	if value == nil || !storage.budget.Spend() {
		return storage.unknown()
	}
	if inner, ok := UnwrapTransparentValue(value,
		TransparentChangeInterface|TransparentChangeType|TransparentConvert|TransparentMakeInterface); ok {
		return storage.Resolve(inner)
	}
	if load, ok := value.(*ssa.UnOp); ok && load.Op == token.MUL {
		content := storage.Content(load.X, load)
		if content.Proven() {
			return storage.Resolve(content.Value)
		}
		return content
	}
	return StoredValue{Proof: Proof{State: EvidenceProven, Reason: EvidenceSameValue, Provenance: EvidenceFromLocalSSA}, Value: value}
}

// Same proves equality after resolving local loads. Failure means unknown,
// never inequality: two opaque loads might still contain the same value.
func (storage *Storage) Same(left, right ssa.Value) IdentityProof {
	if DefinitelySameValue(left, right) {
		return IdentityProof{Proof{State: EvidenceProven, Reason: EvidenceSameValue, Provenance: EvidenceFromLocalSSA}}
	}
	a, b := storage.Resolve(left), storage.Resolve(right)
	if a.Proven() && b.Proven() && DefinitelySameValue(a.Value, b.Value) {
		return IdentityProof{Proof{State: EvidenceProven, Reason: EvidenceSameValue, Provenance: EvidenceFromLocalSSA}}
	}
	return IdentityProof{storage.unknown().Proof}
}

// Content returns the value agreed on by every reaching write. Conflicting
// branch writes, dynamic indexes, and opaque mutation stop the proof. Writes
// after observation do not invalidate an earlier snapshot.
func (storage *Storage) Content(address ssa.Value, observation ssa.Instruction) StoredValue {
	location, ok := storage.location(address)
	if !ok || observation == nil || location.root.Parent() != observation.Parent() {
		return storage.unknown()
	}
	return storage.content(location, observation)
}

func (storage *Storage) unknown() StoredValue {
	reason := EvidenceUnavailable
	if storage.budget.Exhausted() {
		reason = EvidenceBudgetExhausted
	}
	return StoredValue{Proof: Proof{State: EvidenceUnknown, Reason: reason}}
}

func (storage *Storage) location(value ssa.Value) (storageLocation, bool) {
	if value == nil || !storage.budget.Spend() {
		return storageLocation{}, false
	}
	switch value := value.(type) {
	case *ssa.Alloc:
		return storageLocation{root: value}, true
	case *ssa.FieldAddr:
		base, ok := storage.location(value.X)
		base.path += "/f" + strconv.Itoa(value.Field)
		return base, ok
	case *ssa.IndexAddr:
		return storage.indexLocation(value)
	case *ssa.UnOp:
		if value.Op == token.MUL {
			resolved := storage.Resolve(value)
			if resolved.Proven() && resolved.Value != value {
				return storage.location(resolved.Value)
			}
		}
	}
	return storageLocation{}, false
}

func (storage *Storage) content(location storageLocation, observation ssa.Instruction) StoredValue {
	var stores []*ssa.Store
	if !storage.collect(location.root, observation, &stores, false) {
		return storage.unknown()
	}
	return storage.reachingContent(location, observation, stores)
}

// An aggregate copy reads its source when the load ran. Looking at that source
// at the destination's later use would incorrectly observe intervening writes.
func (storage *Storage) projectStored(value ssa.Value, suffix string) StoredValue {
	if suffix == "" {
		return StoredValue{Proof: Proof{State: EvidenceProven, Reason: EvidenceSameValue, Provenance: EvidenceFromLocalSSA}, Value: value}
	}
	load, ok := value.(*ssa.UnOp)
	if !ok || load.Op != token.MUL {
		return storage.unknown()
	}
	location, ok := storage.location(load.X)
	if !ok {
		return storage.unknown()
	}
	location.path += suffix
	return storage.content(location, load)
}

// Walk address uses, not pointee uses. Calling Close on a pointer loaded from
// a field cannot replace that field, whereas passing &field to a call can.
func (storage *Storage) collect(address ssa.Value, observation ssa.Instruction, stores *[]*ssa.Store, whole bool) bool {
	if address.Referrers() == nil {
		return false
	}
	for _, use := range *address.Referrers() {
		if !storage.budget.Spend() {
			return false
		}
		if use == observation || !whole && !InstructionMayFollow(use, observation) {
			continue
		}
		if !storage.collectUse(address, use, observation, stores, whole) {
			return false
		}
	}
	return true
}

func (storage *Storage) collectUse(address ssa.Value, use, observation ssa.Instruction, stores *[]*ssa.Store, whole bool) bool {
	switch use := use.(type) {
	case *ssa.DebugRef:
		return true
	case *ssa.FieldAddr, *ssa.IndexAddr:
		return storage.collect(use.(ssa.Value), observation, stores, whole)
	case *ssa.UnOp:
		return use.Op == token.MUL
	case *ssa.Store:
		if use.Addr != address {
			return false
		}
		*stores = append(*stores, use)
		return true
	case *ssa.MakeClosure:
		return callbackCaptureReadOnly(use, address, storage.budget)
	case *ssa.Call, *ssa.Defer, *ssa.Go:
		return storage.effects.Call(use, address).PreservesStorage()
	case *ssa.Slice:
		if callbackSliceOnlyObserved(use, observation, storage.budget) {
			return true
		}
		_, known := storage.arrayView(use)
		return known && storage.collect(use, observation, stores, whole)
	}
	return false
}
