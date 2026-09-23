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
		budget = NewSearchBudget(QueryBudget)
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
		return storage.unknown(EvidenceUnavailable, nil)
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

// Budget returns the budget this storage query spends, so a caller can hand
// a nested question the same allowance.
func (storage *Storage) Budget() *SearchBudget {
	return storage.budget
}

// Same proves equality after resolving local loads. Failure means unknown,
// never inequality: two opaque loads might still contain the same value.
func (storage *Storage) Same(left, right ssa.Value) IdentityProof {
	if DefinitelySameValue(left, right) {
		return IdentityProof{Proof{State: EvidenceProven, Reason: EvidenceSameValue, Provenance: EvidenceFromLocalSSA}}
	}
	a, b := storage.Resolve(left), storage.Resolve(right)
	if !a.Proven() {
		return IdentityProof{a.Proof}
	}
	if !b.Proven() {
		return IdentityProof{b.Proof}
	}
	if DefinitelySameValue(a.Value, b.Value) {
		return IdentityProof{Proof{State: EvidenceProven, Reason: EvidenceSameValue, Provenance: EvidenceFromLocalSSA}}
	}
	// The points-to graph sees identity the load-by-load resolution above
	// cannot: through a copy of a pointee, a merge of one object, or two
	// reads of one untouched slot. It only adds exact answers; an unknown
	// stays unknown.
	if regionsOf(left).mustSame(left, right) {
		return IdentityProof{Proof{State: EvidenceProven, Reason: EvidenceSameValue, Provenance: EvidenceFromLocalSSA}}
	}
	return IdentityProof{storage.unknown(EvidenceStoredValuesDiffer, nil).Proof}
}

// Content returns the value agreed on by every reaching write. Conflicting
// branch writes, dynamic indexes, and opaque mutation stop the proof. Writes
// after observation do not invalidate an earlier snapshot.
func (storage *Storage) Content(address ssa.Value, observation ssa.Instruction) StoredValue {
	location, ok := storage.location(address)
	if !ok {
		return storage.unknown(EvidenceStorageNotLocal, observation)
	}
	if observation == nil || location.root.Parent() != observation.Parent() {
		return storage.unknown(EvidenceStorageOutsideFunction, observation)
	}
	if proof := storage.content(location, observation); proof.Proven() {
		return proof
	}
	// The graph resolves what the reaching-write walk could not: a cell
	// filled from a copy of a pointee, or a join where every path stored
	// one object. It only adds exact answers.
	if value, ok := regionsOf(address).contentValue(address, observation); ok {
		return StoredValue{Proof: Proof{State: EvidenceProven, Reason: EvidenceSameValue, Provenance: EvidenceFromLocalSSA}, Value: value}
	}
	return storage.content(location, observation)
}

// unknown is the single give-up point of every storage query. It names the
// cause, so the proof a caller prints says which write, use, or merge stopped
// it, and reports that cause with the blocking instruction to whoever is
// observing the budget. An exhausted budget overrides the cause: the query was
// cut short, not decided.
func (storage *Storage) unknown(reason EvidenceReason, at ssa.Instruction) StoredValue {
	if storage.budget.Exhausted() {
		reason = EvidenceBudgetExhausted
	}
	var position token.Pos
	if at != nil {
		position = at.Pos()
	}
	storage.budget.observe(reason, position, func() map[string]string {
		if at == nil {
			return nil
		}
		return map[string]string{"instruction": at.String()}
	})
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
	if blocked, ok := storage.collect(location.root, observation, &stores, false); !ok {
		return storage.unknown(EvidenceStorageAddressEscapes, blocked)
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
		source, _ := value.(ssa.Instruction)
		return storage.unknown(EvidenceStorageProjectionNotLoad, source)
	}
	location, ok := storage.location(load.X)
	if !ok {
		return storage.unknown(EvidenceStorageNotLocal, load)
	}
	location.path += suffix
	return storage.content(location, load)
}

// Walk address uses, not pointee uses. Calling Close on a pointer loaded from
// a field cannot replace that field, whereas passing &field to a call can.
// On failure the returned instruction is the use the query could not see
// through, or nil when the budget ran out.
func (storage *Storage) collect(address ssa.Value, observation ssa.Instruction, stores *[]*ssa.Store, whole bool) (ssa.Instruction, bool) {
	if address.Referrers() == nil {
		return nil, false
	}
	for _, use := range *address.Referrers() {
		if !storage.budget.Spend() {
			return nil, false
		}
		if use == observation || !whole && !InstructionMayFollow(use, observation) {
			continue
		}
		if blocked, ok := storage.collectUse(address, use, observation, stores, whole); !ok {
			return blocked, false
		}
	}
	return nil, true
}

func (storage *Storage) collectUse(address ssa.Value, use, observation ssa.Instruction, stores *[]*ssa.Store, whole bool) (ssa.Instruction, bool) {
	switch typed := use.(type) {
	case *ssa.DebugRef:
		return nil, true
	case *ssa.FieldAddr, *ssa.IndexAddr:
		return storage.collect(typed.(ssa.Value), observation, stores, whole)
	case *ssa.UnOp:
		return use, typed.Op == token.MUL
	case *ssa.Store:
		if typed.Addr != address {
			return use, false
		}
		*stores = append(*stores, typed)
		return nil, true
	case *ssa.MakeClosure:
		return use, callbackCaptureReadOnly(typed, address, storage.budget)
	case *ssa.Call, *ssa.Defer, *ssa.Go:
		return use, storage.effects.Call(use, address).PreservesStorage()
	case *ssa.Slice:
		if callbackSliceOnlyObserved(typed, observation, storage.budget) {
			return nil, true
		}
		if _, known := storage.arrayView(typed); !known {
			return use, false
		}
		return storage.collect(typed, observation, stores, whole)
	}
	return use, false
}
