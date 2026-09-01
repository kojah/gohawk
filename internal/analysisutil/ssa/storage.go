package ssautil

import (
	"go/token"
	"go/types"

	"golang.org/x/tools/go/ssa"
)

func StoresValueInField(instruction ssa.Instruction, value ssa.Value) bool {
	store, ok := instruction.(*ssa.Store)
	if !ok || !SameValue(store.Val, value) {
		return false
	}
	_, ok = store.Addr.(*ssa.FieldAddr)
	return ok
}

// StoresValueThroughExternalFieldPointer reports assignment through a pointer
// slot held by an owner outside the current closure, such as
// `*supervisor.cancel = cancel`.
func StoresValueThroughExternalFieldPointer(instruction ssa.Instruction, value ssa.Value) bool {
	store, ok := instruction.(*ssa.Store)
	if !ok || !SameValue(store.Val, value) {
		return false
	}
	load, ok := store.Addr.(*ssa.UnOp)
	if !ok || load.Op != token.MUL {
		return false
	}
	field, ok := load.X.(*ssa.FieldAddr)
	return ok && ExternallyOwnedValue(field.X)
}

// StoresValueInGlobal reports whether instruction transfers value into
// package-owned storage.
func StoresValueInGlobal(instruction ssa.Instruction, value ssa.Value) bool {
	store, ok := instruction.(*ssa.Store)
	if !ok || !SameValue(store.Val, value) {
		return false
	}
	_, ok = store.Addr.(*ssa.Global)
	return ok
}

// StoresValueInEnclosingScope reports assignment to a captured local owned by
// the enclosing synchronous caller. The inner callback has transferred the
// obligation; the enclosing function is responsible for its later cleanup.
// https://github.com/shini4i/argo-watcher/blob/283d6c6b618b3ade906728ee12a438fd22a328ef/internal/argocd/argo_api.go#L100-L119
func StoresValueInEnclosingScope(instruction ssa.Instruction, value ssa.Value) bool {
	store, ok := instruction.(*ssa.Store)
	if !ok || !ValueDerivesFrom(store.Val, value, map[ssa.Value]bool{}) {
		return false
	}
	_, ok = store.Addr.(*ssa.FreeVar)
	return ok
}

// SendsValue reports whether instruction hands value to a channel receiver.
func SendsValue(instruction ssa.Instruction, value ssa.Value) bool {
	send, ok := instruction.(*ssa.Send)
	return ok && SameValue(send.X, value)
}

// StoresOwnerOfValueInField reports whether instruction stores a callback or
// aggregate that transitively captures value into a struct field.
func StoresOwnerOfValueInField(instruction ssa.Instruction, value ssa.Value) bool {
	store, ok := instruction.(*ssa.Store)
	if !ok {
		return false
	}
	if _, ok := store.Addr.(*ssa.FieldAddr); !ok {
		return false
	}
	return valueOwnsValue(store.Val, value, map[ssa.Value]bool{})
}

// StoresOwnerOfValueInExternalField reports whether an aggregate containing
// value is installed on a receiver or caller-owned struct.
func StoresOwnerOfValueInExternalField(instruction ssa.Instruction, value ssa.Value) bool {
	store, ok := instruction.(*ssa.Store)
	if !ok {
		return false
	}
	field, ok := store.Addr.(*ssa.FieldAddr)
	return ok && ExternallyOwnedValue(field.X) && ValueContainsValue(store.Val, value)
}

// StoresValueInEscapingField reports whether value is installed in a field of
// an owner that already outlives the function or is subsequently transferred.
func StoresValueInEscapingField(instruction ssa.Instruction, value ssa.Value) bool {
	store, ok := instruction.(*ssa.Store)
	if !ok || !SameValue(store.Val, value) {
		return false
	}
	field, ok := store.Addr.(*ssa.FieldAddr)
	return ok && (ExternallyOwnedValue(field.X) || valueTransferred(field.X, map[ssa.Value]bool{}))
}

// ValueContainsValue reports whether owner is an aggregate or closure that
// transitively contains value.

func StoresValueInOwnedMap(instruction ssa.Instruction, value ssa.Value) bool {
	update, ok := instruction.(*ssa.MapUpdate)
	return ok && SameValue(update.Value, value) && ExternallyOwnedValue(update.Map)
}

// ExternallyOwnedValue reports whether value comes from storage that outlives
// the current function invocation.
func ExternallyOwnedValue(value ssa.Value) bool {
	return externallyOwnedValue(value, map[ssa.Value]bool{})
}

func externallyOwnedValue(value ssa.Value, seen map[ssa.Value]bool) bool {
	if value == nil || seen[value] {
		return false
	}
	seen[value] = true
	switch typed := value.(type) {
	case *ssa.Parameter, *ssa.FreeVar, *ssa.Global:
		return true
	case *ssa.Alloc:
		if typed.Referrers() != nil {
			for _, reference := range *typed.Referrers() {
				if store, ok := reference.(*ssa.Store); ok && store.Addr == typed && externallyOwnedValue(store.Val, seen) {
					return true
				}
			}
		}
	case *ssa.FieldAddr:
		return externallyOwnedValue(typed.X, seen)
	case *ssa.Field:
		return externallyOwnedValue(typed.X, seen)
	case *ssa.IndexAddr:
		return externallyOwnedValue(typed.X, seen)
	case *ssa.Index:
		return externallyOwnedValue(typed.X, seen)
	case *ssa.UnOp:
		return externallyOwnedValue(typed.X, seen)
	case *ssa.Lookup:
		return externallyOwnedValue(typed.X, seen)
	case *ssa.ChangeInterface:
		return externallyOwnedValue(typed.X, seen)
	case *ssa.ChangeType:
		return externallyOwnedValue(typed.X, seen)
	case *ssa.Convert:
		return externallyOwnedValue(typed.X, seen)
	case *ssa.MakeInterface:
		return externallyOwnedValue(typed.X, seen)
	case *ssa.Phi:
		for _, edge := range typed.Edges {
			if externallyOwnedValue(edge, seen) {
				return true
			}
		}
	case *ssa.Slice:
		return externallyOwnedValue(typed.X, seen)
	}
	return false
}

// ClosureCapturesValue reports whether instruction creates a closure that owns value.
func ClosureCapturesValue(instruction ssa.Instruction, value ssa.Value) bool {
	closure, ok := instruction.(*ssa.MakeClosure)
	if !ok || !valueTransferred(closure, map[ssa.Value]bool{}) {
		return false
	}
	for _, binding := range closure.Bindings {
		if CapturedBindingMatches(binding, value) {
			return true
		}
	}
	return false
}

func valueTransferred(value ssa.Value, seen map[ssa.Value]bool) bool {
	if value == nil || seen[value] || value.Referrers() == nil {
		return false
	}
	seen[value] = true
	for _, reference := range *value.Referrers() {
		switch typed := reference.(type) {
		case *ssa.Return:
			return true
		case *ssa.Call:
			// Fluent builders preserve an escaping owner through same-typed links.
			// https://github.com/erpc/erpc/blob/2b7e807d7d147422cf47c473153eaf9979afdcc9/clients/http_json_rpc_client.go#L755-L771
			receiver := CallReceiver(typed.Common())
			if receiver != nil && SameValue(receiver, value) && types.Identical(typed.Type(), value.Type()) && valueTransferred(typed, seen) {
				return true
			}
		case *ssa.Store:
			if _, ok := typed.Addr.(*ssa.FieldAddr); ok {
				return true
			}
			if _, ok := typed.Addr.(*ssa.Alloc); ok && typed.Addr.Referrers() != nil {
				for _, use := range *typed.Addr.Referrers() {
					load, loadOK := use.(*ssa.UnOp)
					if loadOK && load.Op == token.MUL && valueTransferred(load, seen) {
						return true
					}
				}
			}
		case *ssa.ChangeInterface:
			if valueTransferred(typed, seen) {
				return true
			}
		case *ssa.ChangeType:
			if valueTransferred(typed, seen) {
				return true
			}
		case *ssa.Convert:
			if valueTransferred(typed, seen) {
				return true
			}
		case *ssa.Extract:
			if valueTransferred(typed, seen) {
				return true
			}
		case *ssa.MakeInterface:
			if valueTransferred(typed, seen) {
				return true
			}
		case *ssa.Phi:
			if valueTransferred(typed, seen) {
				return true
			}
		}
	}
	return false
}

// CallTransfersValueToField reports whether a call consumes value and stores
// its result in a struct field, transferring cleanup to the receiving owner.
func CallTransfersValueToField(instruction ssa.Instruction, value ssa.Value) bool {
	call, ok := instruction.(*ssa.Call)
	if !ok {
		return false
	}
	usesValue := false
	for _, argument := range call.Common().Args {
		usesValue = usesValue || SameValue(argument, value)
	}
	return usesValue && valueStoredInField(call, map[ssa.Value]bool{})
}

func valueStoredInField(value ssa.Value, seen map[ssa.Value]bool) bool {
	if value == nil || seen[value] || value.Referrers() == nil {
		return false
	}
	seen[value] = true
	for _, reference := range *value.Referrers() {
		switch typed := reference.(type) {
		case *ssa.Store:
			if _, ok := typed.Addr.(*ssa.FieldAddr); ok {
				return true
			}
		case *ssa.ChangeInterface:
			if valueStoredInField(typed, seen) {
				return true
			}
		case *ssa.ChangeType:
			if valueStoredInField(typed, seen) {
				return true
			}
		case *ssa.Convert:
			if valueStoredInField(typed, seen) {
				return true
			}
		case *ssa.Extract:
			if valueStoredInField(typed, seen) {
				return true
			}
		case *ssa.MakeInterface:
			if valueStoredInField(typed, seen) {
				return true
			}
		case *ssa.Phi:
			if valueStoredInField(typed, seen) {
				return true
			}
		}
	}
	return false
}

// CallTransfersArgumentToReturnedOwner reports whether a source-visible
// constructor stores an argument in an aggregate it returns.
