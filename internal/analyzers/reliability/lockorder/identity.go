package lockorder

import (
	"go/types"

	"github.com/kojah/gohawk/internal/ssaflow"

	"golang.org/x/tools/go/ssa"
)

func lockIdentity(value ssa.Value, seen map[ssa.Value]bool) string {
	if value == nil || seen[value] {
		return ""
	}
	seen[value] = true
	if source, ok := lockIdentitySource(value); ok {
		return lockIdentity(source, seen)
	}
	switch typed := value.(type) {
	case *ssa.Global:
		return typed.Name()
	case *ssa.FieldAddr:
		return fieldLockIdentity(typed, seen)
	case *ssa.IndexAddr:
		return indexedLockIdentity(typed.X, typed.Index, seen)
	case *ssa.Index:
		return indexedLockIdentity(typed.X, typed.Index, seen)
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

func lockIdentitySource(value ssa.Value) (ssa.Value, bool) {
	if inner, ok := ssaflow.UnwrapTransparentValue(
		value,
		ssaflow.TransparentChangeInterface|ssaflow.TransparentChangeType|ssaflow.TransparentConvert|ssaflow.TransparentMakeInterface,
	); ok {
		return inner, true
	}
	switch typed := value.(type) {
	case *ssa.UnOp:
		return typed.X, true
	default:
		return nil, false
	}
}

func fieldLockIdentity(fieldAddress *ssa.FieldAddr, seen map[ssa.Value]bool) string {
	field := structField(fieldAddress.X.Type(), fieldAddress.Field)
	if field == nil {
		return ""
	}
	if owner := lockIdentity(fieldAddress.X, seen); owner != "" {
		return owner + "." + field.Name()
	}
	return types.TypeString(fieldAddress.X.Type(), nil) + "." + field.Name()
}

func indexedLockIdentity(ownerValue, indexValue ssa.Value, seen map[ssa.Value]bool) string {
	owner := lockIdentity(ownerValue, seen)
	index := lockIdentity(indexValue, seen)
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
