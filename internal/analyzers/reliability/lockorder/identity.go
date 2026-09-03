package lockorder

import (
	"go/types"

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
	if source, ok := ssaflow.IdentitySource(value); ok {
		return lockIdentity(walk, source)
	}
	switch typed := value.(type) {
	case *ssa.Global:
		return typed.Name()
	case *ssa.FieldAddr:
		return fieldLockIdentity(walk, typed)
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

func fieldLockIdentity(walk ssaflow.ReachingWalk, fieldAddress *ssa.FieldAddr) string {
	field := structField(fieldAddress.X.Type(), fieldAddress.Field)
	if field == nil {
		return ""
	}
	if owner := lockIdentity(walk, fieldAddress.X); owner != "" {
		return owner + "." + field.Name()
	}
	return types.TypeString(fieldAddress.X.Type(), nil) + "." + field.Name()
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
// Linux lockdep reports on, and it is why contradictory-order is a hazard in
// the extended tier rather than a core defect: it proves an inconsistent
// order, not a reachable interleaving.
//
// An empty result means the value has no declaration shared across functions —
// a local, a parameter, or a dynamically selected mutex. Those keep instance
// identity, so this widening never makes an existing comparison less exact.
func lockClassOf(value ssa.Value) string {
	return lockClass(ssaflow.NewReachingWalk(ssaflow.TransparentNone), value)
}

func lockClass(walk ssaflow.ReachingWalk, value ssa.Value) string {
	if value == nil || !walk.Mark(value) {
		return ""
	}
	if source, ok := ssaflow.IdentitySource(value); ok {
		return lockClass(walk, source)
	}
	switch typed := value.(type) {
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
	if globalRootedLock(receiver) {
		return identity
	}
	if class := lockClassOf(receiver); class != "" {
		return class
	}
	return identity
}
