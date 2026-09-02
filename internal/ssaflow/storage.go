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
	if inner, ok := UnwrapTransparentValue(
		value,
		TransparentChangeInterface|TransparentChangeType|TransparentConvert|TransparentMakeInterface,
	); ok {
		return externallyOwnedValue(inner, seen)
	}
	if source, ok := ownershipSource(value); ok {
		return externallyOwnedValue(source, seen)
	}
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
	case *ssa.Phi:
		for _, edge := range typed.Edges {
			if externallyOwnedValue(edge, seen) {
				return true
			}
		}
	}
	return false
}

func ownershipSource(value ssa.Value) (ssa.Value, bool) {
	switch typed := value.(type) {
	case *ssa.FieldAddr:
		return typed.X, true
	case *ssa.Field:
		return typed.X, true
	case *ssa.IndexAddr:
		return typed.X, true
	case *ssa.Index:
		return typed.X, true
	case *ssa.UnOp:
		return typed.X, true
	case *ssa.Lookup:
		return typed.X, true
	case *ssa.Slice:
		return typed.X, true
	case *ssa.TypeAssert:
		// A type assertion denotes the same object as the interface value it
		// narrows, so a channel field read through an asserted event is owned
		// by whoever owns the event.
		return typed.X, true
	case *ssa.Extract:
		return typed.Tuple, true
	case *ssa.Next:
		// An element produced by ranging over a map or string belongs to the
		// aggregate being ranged, so a channel taken from a receiver's map is
		// owned by the receiver. policyserv closes such channels from a
		// goroutine in its pubsub Close:
		// https://github.com/matrix-org/policyserv/blob/16995e82a8518f66c5bdc5b6d6d3be04a7640f33/pubsub/psql.go#L62-L72
		return typed.Iter, true
	case *ssa.Range:
		return typed.X, true
	default:
		return nil, false
	}
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
	case *ssa.ChangeInterface, *ssa.ChangeType, *ssa.Convert, *ssa.Extract, *ssa.MakeInterface, *ssa.Phi:
		transformed, ok := reference.(ssa.Value)
		return ok && valueTransferred(transformed, seen)
	default:
		return false
	}
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
