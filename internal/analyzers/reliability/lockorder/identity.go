package lockorder

import (
	"go/types"

	"github.com/kojah/gohawk/internal/ssaflow"

	"golang.org/x/tools/go/ssa"
)

// lockIdentityOf names the lock a receiver value denotes, or returns the
// empty string when its origin cannot be told apart from another lock's.
func lockIdentityOf(value ssa.Value) string {
	return lockIdentity(ssaflow.NewReachingWalk(0), value)
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
