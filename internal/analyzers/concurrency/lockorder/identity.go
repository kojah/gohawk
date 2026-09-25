package lockorder

import (
	"go/token"
	"go/types"

	"github.com/kojah/gohawk/internal/heapmodel"
	"github.com/kojah/gohawk/internal/ssaflow"
	"golang.org/x/tools/go/ssa"
)

// lockIdentityOf names the lock a receiver value denotes, or returns the
// empty string when its origin cannot be told apart from another lock's.
func lockIdentityOf(value ssa.Value) string {
	return lockIdentity(ssaflow.NewReachingWalk(ssaflow.TransparentNone), value)
}

func lockIdentity(walk ssaflow.ReachingWalk, value ssa.Value) string {
	if value == nil || !walk.Mark(value) {
		return ""
	}
	if resolved := heapmodel.NewStorage(nil).Resolve(value); resolved.Proven() && resolved.Value != value {
		return lockIdentity(walk, resolved.Value)
	}
	if source, ok := ssaflow.IdentitySource(value); ok {
		return lockIdentity(walk, source)
	}
	switch typed := value.(type) {
	case *ssa.Call:
		owner, field := mutexGetter(typed)
		if field == nil {
			// A call result is not a stable lock across executions. Without the
			// body, matching separate calls by name guesses at aliasing, while
			// keeping them separate invents held locks across loop iterations.
			return ""
		}
		if identity := lockIdentity(walk, owner); identity != "" {
			return identity + "." + field.Name()
		}
		return ""
	case *ssa.Global:
		return typed.Name()
	case *ssa.FieldAddr, *ssa.Field:
		return projectedFieldLockIdentity(walk, typed)
	case *ssa.IndexAddr:
		return indexedLockIdentity(walk, typed.X, typed.Index)
	case *ssa.Index:
		return indexedLockIdentity(walk, typed.X, typed.Index)
	case *ssa.Parameter:
		return typed.Parent().String() + "." + typed.Name()
	case *ssa.FreeVar:
		// Two captured owners of the same type are distinct locks inside the
		// closure, just as two parameters are; the closure's own name keeps the
		// identity from colliding with any other function's captures.
		// https://github.com/trickstercache/trickster/blob/7818ae3c39e725eb998f04fa31e5d315ede84b79/integration/alb_request_headers_test.go#L93-L99
		return typed.Parent().String() + ":free:" + typed.Name()
	case *ssa.Alloc:
		// SSA uses generic comments such as "complit" for distinct local
		// allocations of the same type. Include the stable SSA value name so two
		// local owners do not collapse into one apparent recursive lock.
		return typed.Parent().String() + ":local:" + typed.Comment + ":" + typed.Name()
	case *ssa.Const:
		if typed.Value != nil {
			return "constant:" + typed.Value.ExactString()
		}
	}
	if parent := value.Parent(); parent != nil && value.Name() != "" {
		// Dynamic values of the same type can still identify different lock
		// instances. Keep their SSA identities distinct instead of collapsing
		// them to the field type. Prometheus transfers state while holding locks
		// on two alertmanagerSet values of the same type:
		// https://github.com/prometheus/prometheus/blob/e06b2dc5a6149e20ca82fe936fb044a6dfe45958/notifier/manager.go#L165-L180
		return parent.String() + ":value:" + value.Name()
	}
	return ""
}

// mutexGetter recognizes only a pointer receiver returning the address of one
// direct field. Loads, branches, calls and side effects are intentionally opaque;
// in particular a pointer-valued field could change between calls. This is the
// body of Account.Mu, not a convention attached to its name:
// https://github.com/james-6-23/codex2api/blob/4f96afe95bb16132347f4ab74e63b0b1fa0f778b/auth/store.go#L553-L555
func mutexGetter(call *ssa.Call) (ssa.Value, *types.Var) { //nolint:ireturn // The owner is an SSA value.
	callee := call.Common().StaticCallee()
	if callee == nil || callee.Signature.Recv() == nil || len(callee.Params) != 1 ||
		len(call.Common().Args) != 1 || len(callee.Blocks) != 1 {
		return nil, nil
	}
	if _, ok := callee.Params[0].Type().Underlying().(*types.Pointer); !ok {
		return nil, nil
	}
	instructions := callee.Blocks[0].Instrs
	if len(instructions) != 2 {
		return nil, nil
	}
	field, ok := instructions[0].(*ssa.FieldAddr)
	if !ok || field.X != callee.Params[0] {
		return nil, nil
	}
	returned, ok := instructions[1].(*ssa.Return)
	if !ok || len(returned.Results) != 1 || returned.Results[0] != field {
		return nil, nil
	}
	return call.Common().Args[0], structField(field.X.Type(), field.Field)
}

func fieldLockIdentity(walk ssaflow.ReachingWalk, fieldAddress *ssa.FieldAddr) string {
	field := structField(fieldAddress.X.Type(), fieldAddress.Field)
	if field == nil {
		return ""
	}
	if owner := lockIdentity(walk, fieldAddress.X); owner != "" {
		return owner + "." + field.Name()
	}
	// A declaration identifies a lock class, not an instance. When a call
	// returns an unknown owner, its field must remain unknown too: a pool can
	// return a different container on each loop iteration.
	// https://github.com/encodeous/nylon/blob/c4a96c804f7aa08512721dec7994907eab100bc8/polyamide/device/receive.go#L176-L180
	return ""
}

func projectedFieldLockIdentity(walk ssaflow.ReachingWalk, value ssa.Value) string {
	switch field := value.(type) {
	case *ssa.FieldAddr:
		return fieldLockIdentity(walk, field)
	case *ssa.Field:
		return copiedFieldLockIdentity(walk, field)
	}
	return ""
}

// A value receiver can be copied into a local SSA slot and reloaded for each
// field access. Only an exact aggregate read back to the immutable parameter
// or capture identifies those copies as the same field. Partial/conflicting
// writes leave the read unresolved, so they cannot equate different locks.
// https://github.com/tikv/client-go/blob/b9fc0b7719d3ea62bd9904cd31fbd27715ff08bc/txnkv/transaction/pessimistic.go#L489-L518
func copiedFieldLockIdentity(walk ssaflow.ReachingWalk, fieldValue *ssa.Field) string {
	field := structField(fieldValue.X.Type(), fieldValue.Field)
	if field == nil {
		return ""
	}
	source := heapmodel.NewStorage(nil).Resolve(fieldValue.X)
	if !source.Proven() {
		return ""
	}
	switch source.Value.(type) {
	case *ssa.Parameter, *ssa.FreeVar:
		if owner := lockIdentity(walk, source.Value); owner != "" {
			return owner + "." + field.Name()
		}
	}
	return ""
}

// privateMutexOnly reports the deliberately narrow case where a local mutex
// allocation is used exclusively by direct synchronous mutex operations.
// No pointer, callback, or owner leaves these uses, so a held mutex cannot
// affect another caller after return. This says nothing about recursive
// acquisition before return, which is still checked.
// https://github.com/alajmo/sake/blob/86986df901293db0f7d1e548ef34c849bb1f709d/core/run/exec.go#L1070-L1084
func privateMutexOnly(value ssa.Value) bool {
	allocation, ok := value.(*ssa.Alloc)
	if !ok || allocation.Referrers() == nil {
		return false
	}
	for _, use := range *allocation.Referrers() {
		if _, ok := use.(*ssa.DebugRef); ok {
			continue
		}
		if _, ok := use.(*ssa.Call); !ok {
			return false
		}
		_, _, receiver, ok := mutexAction(use)
		if !ok || receiver != allocation {
			return false
		}
	}
	return true
}

func indexedLockIdentity(walk ssaflow.ReachingWalk, ownerValue, indexValue ssa.Value) string {
	owner := lockIdentity(walk, ownerValue)
	index := lockIdentity(walk, indexValue)
	if owner == "" || index == "" {
		return ""
	}
	return owner + "[" + index + "]"
}

func structField(value types.Type, index int) *types.Var {
	if pointer, ok := value.Underlying().(*types.Pointer); ok {
		value = pointer.Elem()
	}
	structure, ok := value.Underlying().(*types.Struct)
	if !ok || index < 0 || index >= structure.NumFields() {
		return nil
	}
	return structure.Field(index)
}

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
			for _, binding := range ssaflow.CallBindings(use.Common(), callee, closure) {
				if binding.Supplied == owner && visibleMutexSlotReplacement(walk, binding.Local, field, observation, budget) {
					return true
				}
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
