package lockorder

import (
	"go/token"
	"go/types"

	"github.com/kojah/gohawk/internal/heapmodel"
	"github.com/kojah/gohawk/internal/ssaflow"
	"golang.org/x/tools/go/ssa"
)

// Declaration classes widen instance identities only for ordering evidence.
// Fresh slot and constructor witnesses keep uncertain local instances separate;
// they never establish publication safety. These class/heap queries retain
// separate costs from request-owned instance identity resolution.

// lockClassOf names the declaration a lock comes from — the struct field or
// package variable it lives in — rather than the object holding it. Two
// different *T values yield the same class for the same field.
//
// Instance identity cannot answer whether the receiver in one method is the
// object another method locks, so a field mutex acquired through a receiver
// gets a per-function name and no two functions ever compare. That leaves
// contradictory-order able to see package-level mutexes and almost nothing
// else, while idiomatic Go keeps its mutex in a struct field.
//
// Classes make the comparison possible by changing what is claimed. A program
// that takes class A before class B in one place and B before A in another has
// no consistent acquisition order; that is a defect in the locking discipline
// even where today's callers happen to pass different objects, because nothing
// stops a later caller from passing the same one. This is the ordering model
// Linux lockdep reports on, and it is why contradictory-order is a hazard
// rather than a defect: it proves an inconsistent order, not a reachable
// interleaving.
// Field classes of the same owner type with both roots bound in one caller
// additionally require a same-owner relation at the ordering edge. Unproved
// cross-instance relationships retain only local instance ordering.
//
// An empty result means the value has no declaration shared across functions —
// a local, a parameter, or a dynamically selected mutex. Those keep instance
// identity, so this widening never makes an existing comparison less exact.
func lockClassOf(value ssa.Value) string {
	if path, known := lockResourcePath(value); known {
		if _, local := path.Root.(*ssa.Alloc); local {
			return ""
		}
	}
	// A known local mutex allocation retains its instance identity even when
	// stored in an owner field. Replacing that witness with the field's class
	// merges construction-time locking with unrelated established instances.
	// Cross-function ordering after publication of this allocation is unknown.
	// https://github.com/ozontech/file.d/blob/5379bc2005906fde3aa0a05f6bf574dcd7111404/plugin/input/file/provider.go#L404-L477
	if localMutexAllocation(value) != nil || possibleFreshMutexField(value).possible {
		return ""
	}
	return lockClass(ssaflow.NewReachingWalk(ssaflow.TransparentNone), value)
}

func localMutexAllocation(value ssa.Value) *ssa.Alloc {
	resolved := heapmodel.NewStorage(nil).Resolve(value)
	if !resolved.Proven() {
		return nil
	}
	allocation, _ := resolved.Value.(*ssa.Alloc)
	return allocation
}

type freshMutexFieldProof struct {
	possible bool
	reason   lockReason
}

// A positively fresh field initializer is still relevant when publication
// makes its later load opaque. Do not widen that uncertain local slot to every
// instance of its declaration. This is not freshness or publication safety:
// opaque code could replace it with a shared mutex. Visible replacements veto
// this boundary, including writes reached through exact helper bindings.
// https://github.com/ozontech/file.d/blob/5379bc2005906fde3aa0a05f6bf574dcd7111404/plugin/input/file/provider.go#L404-L477
func possibleFreshMutexField(value ssa.Value) freshMutexFieldProof {
	unknown := freshMutexFieldProof{reason: lockReasonNoFreshFieldWitness}
	load, ok := value.(*ssa.UnOp)
	if !ok || load.Op != token.MUL {
		return unknown
	}
	field, ok := load.X.(*ssa.FieldAddr)
	if !ok {
		return unknown
	}
	owner, ok := field.X.(*ssa.Alloc)
	if !ok || owner.Parent() != load.Parent() {
		return unknown
	}
	if heapmodel.NewStorage(nil).Resolve(value).Proven() {
		return unknown
	}
	fresh := false
	for _, store := range ssaflow.InstructionsOf[*ssa.Store](owner.Parent()) {
		target, ok := store.Addr.(*ssa.FieldAddr)
		if !ok || target.X != owner || target.Field != field.Field || !ssaflow.InstructionDominates(store, load) {
			continue
		}
		_, allocated := store.Val.(*ssa.Alloc)
		fresh = fresh || allocated
	}
	if !fresh || visibleMutexSlotReplacement(ssaflow.NewReachingWalk(ssaflow.TransparentNone), owner, field.Field, load,
		ssaflow.NewSearchBudget(ssaflow.QueryBudget)) {
		return unknown
	}
	return freshMutexFieldProof{possible: true, reason: lockReasonFreshFieldIdentityUnknown}
}

func visibleMutexSlotReplacement(
	walk ssaflow.ReachingWalk, owner ssa.Value, field int, observation ssa.Instruction, budget *ssaflow.SearchBudget,
) bool {
	if !walk.Mark(owner) || owner.Referrers() == nil {
		return false
	}
	for _, use := range *owner.Referrers() {
		if !budget.Spend() {
			return true
		}
		if owner.Parent() == observation.Parent() && !ssaflow.InstructionMayFollow(use, observation) {
			continue
		}
		switch use := use.(type) {
		case *ssa.Store:
			if use.Addr == owner {
				return true
			}
		case *ssa.FieldAddr:
			if use.Field == field && mutexSlotWrittenShared(use, budget) {
				return true
			}
		case ssa.CallInstruction:
			callee, closure := ssaflow.DirectCallee(use.Common())
			for binding := range ssaflow.CallBindingsWithin(use.Common(), callee, closure, budget) {
				if binding.Supplied == owner && visibleMutexSlotReplacement(walk, binding.Local, field, observation, budget) {
					return true
				}
			}
			// Cutoff leaves replacement possible, never establishes its absence.
			if budget.Exhausted() {
				return true
			}
		case *ssa.ChangeType, *ssa.Convert, *ssa.MakeInterface, *ssa.Phi:
			// This boundary binds an exact owner, not possible aliases of it.
			return true
		}
	}
	return false
}

func mutexSlotWrittenShared(address *ssa.FieldAddr, budget *ssaflow.SearchBudget) bool {
	if address.Referrers() == nil {
		return false
	}
	for _, use := range *address.Referrers() {
		if !budget.Spend() {
			return true
		}
		if store, ok := use.(*ssa.Store); ok && store.Addr == address {
			if _, fresh := store.Val.(*ssa.Alloc); !fresh {
				return true
			}
		}
		if call, ok := use.(ssa.CallInstruction); ok {
			proof := ssaflow.NewCallEffects(budget).Call(call, address)
			if proof.Effects&ssaflow.EffectMutate != 0 {
				return true
			}
		}
	}
	return false
}

func lockClass(walk ssaflow.ReachingWalk, value ssa.Value) string {
	if value == nil || !walk.Mark(value) {
		return ""
	}
	if source, ok := ssaflow.IdentitySource(value); ok {
		return lockClass(walk, source)
	}
	switch typed := value.(type) {
	case *ssa.Call:
		owner, field := mutexGetter(typed)
		if field != nil {
			return types.TypeString(owner.Type(), nil) + "." + field.Name()
		}
	case *ssa.Global:
		// One package variable is one lock, so its class is its instance.
		return typed.Name()
	case *ssa.FieldAddr:
		field := structField(typed.X.Type(), typed.Field)
		if field == nil {
			return ""
		}
		return types.TypeString(typed.X.Type(), nil) + "." + field.Name()
	}
	// An index, a map lookup, a local, or a parameter names no declaration that
	// another function could reach, so there is no class to compare.
	return ""
}

// globalRootedLock reports whether the value's owner chain bottoms out in a
// package variable, which makes its instance identity mean the same object in
// every function that names it.
func globalRootedLock(value ssa.Value) bool {
	return globalRooted(ssaflow.NewReachingWalk(ssaflow.TransparentNone), value)
}

func globalRooted(walk ssaflow.ReachingWalk, value ssa.Value) bool {
	if value == nil || !walk.Mark(value) {
		return false
	}
	if source, ok := ssaflow.IdentitySource(value); ok {
		return globalRooted(walk, source)
	}
	switch typed := value.(type) {
	case *ssa.Call:
		owner, field := mutexGetter(typed)
		return field != nil && globalRooted(walk, owner)
	case *ssa.Global:
		return true
	case *ssa.FieldAddr:
		return globalRooted(walk, typed.X)
	}
	return false
}

// lockComparisonKey is the name contradictory-order reasons about.
//
// A lock reached from a package variable keeps its instance identity: two
// globals of one type are provably different objects, so an inversion between
// them is a real deadlock and reporting it needs no class reasoning. Every
// comparison the analyzer made before classes existed goes through this branch
// and is unchanged.
//
// Anything else — a field behind a receiver or parameter — has a
// function-scoped identity that can never match another function's, so it
// falls back to its class and becomes comparable. Where no class exists the
// instance identity stands, which compares only within one function.
func lockComparisonKey(identity string, receiver ssa.Value) string {
	if path, known := lockResourcePath(receiver); known {
		if _, local := path.Root.(*ssa.Alloc); local {
			return localMutexPathIdentity(path)
		}
	}
	if allocation := localMutexAllocation(receiver); allocation != nil {
		// One loop allocation instruction represents different runtime locks;
		// its SSA name must not connect ordering edges between iterations.
		if ssaflow.BlockInCycle(allocation.Block()) {
			return ""
		}
		return identity
	}
	if globalRootedLock(receiver) {
		return identity
	}
	if class := lockClassOf(receiver); class != "" {
		return class
	}
	return identity
}
