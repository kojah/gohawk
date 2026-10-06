package heapmodel

import (
	proofs "github.com/kojah/gohawk/internal/engine/proof"
	"github.com/kojah/gohawk/internal/engine/ssaflow"
	cfg "github.com/kojah/gohawk/internal/engine/ssaflow/cfg"
	"golang.org/x/tools/go/ssa"
)

// Possible identity, answered by the points-to graph where one is available
// and by the structural value walk otherwise. Both keep the same contract:
// two objects are the same only when the function's own flow connects
// them.

// MayAlias reports a possible identity through conversions, any phi edge,
// and local storage. It is not a must-alias proof: use DefinitelySameValue
// when a diagnostic or guaranteed action requires exact identity. It does
// not equate a field or index with its containing aggregate; use
// ValueDerivesFrom or MayContainValue for containment instead. It is the
// Boolean of ProveMayAlias, which is the one decision path.
func MayAlias(value, target ssa.Value) bool {
	return ProveMayAlias(value, target).Aliases
}

// CapturedBindingMatches reports whether a closure binding directly contains
// target or refers to an addressable local that has contained target. Unlike
// CapturedBindingValue, it handles variables reassigned before a callback is
// installed without depending on referrer iteration order.
func CapturedBindingMatches(binding, target ssa.Value) bool {
	return CapturedBindingMatchesWithin(binding, target, nil)
}

// CapturedBindingMatchesWithin shares alias dispatch and store-referrer visits
// with budget. Graph and alias internals remain separate; cutoff is unavailable.
func CapturedBindingMatchesWithin(binding, target ssa.Value, budget *proofs.SearchBudget) bool {
	if !budget.Spend() {
		return false
	}
	if MayAlias(binding, target) {
		return true
	}
	if binding == nil || binding.Referrers() == nil {
		return false
	}
	for _, reference := range *binding.Referrers() {
		if !budget.Spend() {
			return false
		}
		store, ok := reference.(*ssa.Store)
		if ok && store.Addr == binding && MayAlias(store.Val, target) {
			return true
		}
	}
	return false
}

// DefinitelySameValue proves value identity: the values name one object
// on every path. The value walk proves it for one SSA value seen through
// wrappers, a phi whose alternatives all agree, and equal address
// selections; the points-to graph adds a cell resolved through a copy of
// its pointee, a join where every path stored one object, and two reads of
// one slot with no write between them. A false result means unproved, not
// necessarily different.
func DefinitelySameValue(left, right ssa.Value) bool {
	if ssaflow.StructurallyIdentical(left, right) {
		return true
	}
	return DefinitelySame(left, right)
}

// MayAliasAny reports whether value may alias any candidate; see MayAlias.
func MayAliasAny(value ssa.Value, candidates []ssa.Value) bool {
	return MayAliasAnyWithin(value, candidates, nil)
}

// MayAliasAnyWithin charges candidate visits and alias dispatch to budget.
// Graph construction and alias-query internals remain independent costs.
// Cutoff supplies no alias evidence; callers must retain its availability.
func MayAliasAnyWithin(value ssa.Value, candidates []ssa.Value, budget *proofs.SearchBudget) bool {
	for _, candidate := range candidates {
		if !budget.Spend() {
			return false
		}
		if MayAlias(value, candidate) {
			return !budget.Exhausted() && !budget.PoolExhausted()
		}
	}
	return false
}

// ReturnedMayAliasAnyWithin shares result and candidate visits with budget.
// Exhaustion cannot establish either a transfer or absence of one.
func ReturnedMayAliasAnyWithin(returned *ssa.Return, candidates []ssa.Value, budget *proofs.SearchBudget) bool {
	for _, result := range returned.Results {
		if !budget.Spend() {
			return false
		}
		if MayAliasAnyWithin(result, candidates, budget) {
			return !budget.Exhausted() && !budget.PoolExhausted()
		}
	}
	return false
}

// Whole-object exclusivity permits an imprecise selection within one non-stale
// object. It requires a complete identity and no recorded exposure at the
// observation; exact content identity remains the separate slot query policy.

// ExclusiveObject says who can reach an object at an instruction: only the
// function, through a local allocation that has not escaped there, or only
// the function and its caller, through a parameter that has not escaped
// there. Published reports whether the local object escapes later on some
// path, which separates an object being initialized before publication
// from one that never leaves the function.
type ExclusiveObject struct {
	Parameter int
	Local     bool
	Published bool
}

// exclusiveAt reports whether the one object the value refers into has not
// escaped when the instruction runs, and who else could reach it.
func (graph *regionGraph) exclusiveAt(value ssa.Value, at ssa.Instruction) (ExclusiveObject, bool) {
	defer graph.lock()()
	set, ok := graph.pointsToUnlocked(value)
	if !ok {
		return ExclusiveObject{}, false
	}
	target, ok := singleObjectSlot(set)
	if !ok {
		return ExclusiveObject{}, false
	}
	state := graph.stateAt(at)
	if state == nil || regionExposed(state, target.region) {
		return ExclusiveObject{}, false
	}
	switch target.region.kind {
	case regionSite:
		if state.escaped[target.region] {
			return ExclusiveObject{}, false
		}
		return ExclusiveObject{Local: true, Published: graph.publishedAfterUnlocked(target.region, at)}, true
	case regionExternal:
		parameter, ok := target.region.origin.(*ssa.Parameter)
		if !ok {
			return ExclusiveObject{}, false
		}
		for index, candidate := range graph.function.Params {
			if candidate == parameter {
				return ExclusiveObject{Parameter: index}, true
			}
		}
	case regionOpaque:
		// A language allocation is fresh even though its element content is
		// opaque. Any exposed selection defeats whole-object exclusivity.
		if freshMake(target.region.origin) {
			return ExclusiveObject{Local: true, Published: graph.publishedAfterUnlocked(target.region, at)}, true
		}
	case regionNil, regionUnknown, regionPlaceholder, regionSnapshot, regionClosure:
	}
	return ExclusiveObject{}, false
}

// freshMake separates language allocations from opaque calls and loaded values.
func freshMake(origin ssa.Value) bool {
	switch origin.(type) {
	case *ssa.MakeMap, *ssa.MakeSlice, *ssa.MakeChan:
		return true
	}
	return false
}

func regionExposed(state *regionState, object *region) bool {
	for target, escape := range state.escapes {
		if target.region == object && escape != 0 {
			return true
		}
	}
	return false
}

// publishedAfter reports whether the object is published on some path
// from the instruction: a return the instruction can reach whose state has
// the object stored into a global or a field, sent, or handed to a
// goroutine. Being handed to a call is not publication. Escapes accumulate
// along a path, so a publication seen at such a return and absent at the
// instruction happened between them.
func (graph *regionGraph) publishedAfterUnlocked(object *region, at ssa.Instruction) bool {
	const publication = HeapEscapedGlobal | HeapEscapedField | HeapEscapedSend | HeapEscapedAsync
	for _, returned := range ssaflow.InstructionsOf[*ssa.Return](graph.function) {
		if !cfg.InstructionMayFollow(at, returned) {
			continue
		}
		if state := graph.stateAt(returned); state != nil && state.escapes[slot{region: object}]&publication != 0 {
			return true
		}
	}
	return false
}

// singleObjectSlot permits different selections of one object, retaining the
// stale and unknown-object boundaries. A wildcard element still belongs to that
// object; exact storage identity continues to require singleSlot.
func singleObjectSlot(set pointees) (slot, bool) {
	var object slot
	for target, stale := range set {
		if stale || target.region.kind == regionUnknown || object.region != nil && object.region != target.region {
			return slot{}, false
		}
		object = slot{region: target.region}
	}
	return object, object.region != nil
}

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
		if follows || cfg.BlockInCycle(store.Block()) && store.Block() != location.root.Block() {
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

// Derivation is the may-relation "source contributes to value": through
// operands, through a local load and store pair, and, since the points-to
// graph answers identity, through any copy, join, or captured cell the
// graph resolves. Its polarity ends the walk at anything it cannot follow.

// ValueDerivesFrom reports whether source contributes to value through SSA
// operands or a local load/store pair. This is a may-relation: every store to
// the loaded address counts, not only the one that reaches the load.
//
// A load through a field or element address also derives from a value stored
// into the enclosing aggregate as a whole, when that aggregate is only ever
// written whole. The builder spills a struct or array parameter, and a copy
// such as k := j, into a local cell before it can select a field, so
// j.out.Close() in func (j job) reaches the parameter only through that
// spill. Without this step a by-value parameter could never be proven closed
// while the same body with a pointer parameter is, because the pointer's
// field address selects from the parameter directly. A cell with a store into
// one of its fields is not crossed: the field a later load returns may be the
// replacement rather than a component of the stored aggregate, and the
// analyzer must keep such a replaced resource reportable.
func ValueDerivesFrom(value, source ssa.Value) bool {
	return ValueDerivesFromWithin(value, source, nil)
}

// ValueDerivesFromWithin shares derivation visits with budget. Alias dispatch
// is charged, but graph construction and alias-query internals remain separate
// costs. Exhaustion is unavailable, never evidence that source is absent.
func ValueDerivesFromWithin(value, source ssa.Value, budget *proofs.SearchBudget) bool {
	return ssaflow.DerivesFromWithin(value, source, func(left, right ssa.Value) bool {
		return budget.Spend() && MayAlias(left, right)
	}, budget)
}
