package ssaflow

import (
	"go/token"
	"go/types"

	"golang.org/x/tools/go/ssa"
)

// Storage evidence distinguishes local retention from ownership transfers to
// callers, receivers, closures, globals, and escaping aggregates. The helpers
// require a traceable stored value or owner so ambiguous aliases remain local.

func StoresValueInField(instruction ssa.Instruction, value ssa.Value) bool {
	store, ok := instruction.(*ssa.Store)
	if !ok || !SameValue(store.Val, value) {
		return false
	}
	_, ok = store.Addr.(*ssa.FieldAddr)
	return ok
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
	// A wrapper that holds the value, such as a log-file record keyed by
	// session, transfers it to the map's owner exactly as the value would.
	// https://github.com/askie/grix/blob/dbf8ad10477d7458c7b8c9900ce2e2a6296d4063/backend/internal/pkg/adapterlog/adapterlog.go#L115-L129
	return ok && (SameValue(update.Value, value) || ValueContainsValue(update.Value, value)) && ExternallyOwnedValue(update.Map)
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
		if referenceTransfersValue(reference, value, seen) {
			return true
		}
	}
	return false
}

func referenceTransfersValue(reference ssa.Instruction, value ssa.Value, seen map[ssa.Value]bool) bool {
	switch typed := reference.(type) {
	case *ssa.Return:
		return true
	case *ssa.Call:
		// Fluent builders preserve an escaping owner through same-typed links.
		// https://github.com/erpc/erpc/blob/2b7e807d7d147422cf47c473153eaf9979afdcc9/clients/http_json_rpc_client.go#L755-L771
		receiver := CallReceiver(typed.Common())
		return receiver != nil && SameValue(receiver, value) &&
			types.Identical(typed.Type(), value.Type()) && valueTransferred(typed, seen)
	case *ssa.Store:
		return storeTransfersValue(typed, seen)
	}
	forwarded, ok := forwardedValue(reference)
	return ok && valueTransferred(forwarded, seen)
}

func storeTransfersValue(store *ssa.Store, seen map[ssa.Value]bool) bool {
	if _, ok := store.Addr.(*ssa.FieldAddr); ok {
		return true
	}
	if _, ok := store.Addr.(*ssa.Alloc); !ok || store.Addr.Referrers() == nil {
		return false
	}
	for _, use := range *store.Addr.Referrers() {
		load, ok := use.(*ssa.UnOp)
		if ok && load.Op == token.MUL && valueTransferred(load, seen) {
			return true
		}
	}
	return false
}

// CallTransfersArgumentToReturnedOwner reports whether a source-visible
// constructor stores an argument in an aggregate it returns.

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
		if store, isStore := reference.(*ssa.Store); isStore {
			if _, isField := store.Addr.(*ssa.FieldAddr); isField {
				return true
			}
			continue
		}
		if forwarded, ok := forwardedValue(reference); ok && valueStoredInField(forwarded, seen) {
			return true
		}
	}
	return false
}
