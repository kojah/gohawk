package lifecycle

import (
	"go/token"
	"go/types"

	"github.com/kojah/gohawk/internal/heapmodel"
	"github.com/kojah/gohawk/internal/ssaflow"
	"golang.org/x/tools/go/ssa"
)

// Storage evidence distinguishes local retention from ownership transfers to
// callers, receivers, closures, globals, and escaping aggregates. The helpers
// require a traceable stored value or owner so ambiguous aliases remain local.

// StoresValueInField reports whether instruction transfers value into a struct field.
func StoresValueInField(instruction ssa.Instruction, value ssa.Value) bool {
	store, ok := instruction.(*ssa.Store)
	if !ok || !heapmodel.MayAlias(store.Val, value) {
		return false
	}
	_, ok = store.Addr.(*ssa.FieldAddr)
	return ok
}

// StoresValueInGlobal reports whether instruction transfers value into
// package-owned storage.
func StoresValueInGlobal(instruction ssa.Instruction, value ssa.Value) bool {
	store, ok := instruction.(*ssa.Store)
	if !ok || !heapmodel.MayAlias(store.Val, value) {
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
	if !ok || !heapmodel.ValueDerivesFrom(store.Val, value) {
		return false
	}
	_, ok = store.Addr.(*ssa.FreeVar)
	return ok
}

// SendsValue reports whether instruction hands value to a channel receiver.
func SendsValue(instruction ssa.Instruction, value ssa.Value) bool {
	send, ok := instruction.(*ssa.Send)
	return ok && heapmodel.MayAlias(send.X, value)
}

// StoresOwnerOfValueInField reports whether instruction stores value or a
// callback that transitively captures it into a struct field. General aggregate
// containment belongs to MayContainValue and does not establish this transfer.
func StoresOwnerOfValueInField(instruction ssa.Instruction, value ssa.Value) bool {
	store, ok := instruction.(*ssa.Store)
	if !ok {
		return false
	}
	if _, ok := store.Addr.(*ssa.FieldAddr); !ok {
		return false
	}
	return valueOwnsValue(store.Val, value)
}

// StoresOwnerOfValueInExternalField reports whether an aggregate containing
// value is installed on a receiver or caller-owned struct.
func StoresOwnerOfValueInExternalField(instruction ssa.Instruction, value ssa.Value) bool {
	store, ok := instruction.(*ssa.Store)
	if !ok {
		return false
	}
	field, ok := store.Addr.(*ssa.FieldAddr)
	return ok && ssaflow.ExternallyOwnedValue(field.X) && MayContainValue(store.Val, value)
}

// StoresValueInEscapingField reports whether value is installed in a field of
// an owner that already outlives the function or is subsequently transferred.
func StoresValueInEscapingField(instruction ssa.Instruction, value ssa.Value) bool {
	store, ok := instruction.(*ssa.Store)
	if !ok || !heapmodel.MayAlias(store.Val, value) {
		return false
	}
	field, ok := store.Addr.(*ssa.FieldAddr)
	return ok && (ssaflow.ExternallyOwnedValue(field.X) || valueTransferred(field.X))
}

func StoresValueInOwnedMap(instruction ssa.Instruction, value ssa.Value) bool {
	update, ok := instruction.(*ssa.MapUpdate)
	// A wrapper that holds the value, such as a log-file record keyed by
	// session, transfers it to the map's owner exactly as the value would.
	// https://github.com/askie/grix/blob/dbf8ad10477d7458c7b8c9900ce2e2a6296d4063/backend/internal/pkg/adapterlog/adapterlog.go#L115-L129
	return ok && (heapmodel.MayAlias(update.Value, value) || MayContainValue(update.Value, value)) && ssaflow.ExternallyOwnedValue(update.Map)
}

// ClosureCapturesValue reports whether instruction creates a closure that owns value.
func ClosureCapturesValue(instruction ssa.Instruction, value ssa.Value) bool {
	closure, ok := instruction.(*ssa.MakeClosure)
	if !ok || !valueTransferred(closure) {
		return false
	}
	for _, binding := range closure.Bindings {
		if heapmodel.CapturedBindingMatches(binding, value) {
			return true
		}
	}
	return false
}

func valueTransferred(value ssa.Value) bool {
	return valueHasForwardUse(value, referenceTransfersValue)
}

func referenceTransfersValue(value ssa.Value, reference ssa.Instruction) ([]ssa.Value, bool) {
	switch typed := reference.(type) {
	case *ssa.Return:
		return nil, true
	case *ssa.Call:
		// Fluent builders preserve an escaping owner through same-typed links.
		// https://github.com/erpc/erpc/blob/2b7e807d7d147422cf47c473153eaf9979afdcc9/clients/http_json_rpc_client.go#L755-L771
		receiver := ssaflow.CallReceiver(typed.Common())
		if receiver != nil && heapmodel.MayAlias(receiver, value) && types.Identical(typed.Type(), value.Type()) {
			return []ssa.Value{typed}, false
		}
	case *ssa.Store:
		return storeTransferUses(typed)
	}
	return nil, false
}

func storeTransferUses(store *ssa.Store) ([]ssa.Value, bool) {
	if _, ok := store.Addr.(*ssa.FieldAddr); ok {
		return nil, true
	}
	if _, ok := store.Addr.(*ssa.Alloc); !ok || store.Addr.Referrers() == nil {
		return nil, false
	}
	var loads []ssa.Value
	for _, use := range *store.Addr.Referrers() {
		load, ok := use.(*ssa.UnOp)
		if ok && load.Op == token.MUL {
			loads = append(loads, load)
		}
	}
	return loads, false
}

// CallTransfersValueToField reports whether a call consumes value and stores
// its result in a struct field, transferring cleanup to the receiving owner.
func CallTransfersValueToField(instruction ssa.Instruction, value ssa.Value) bool {
	call, ok := instruction.(*ssa.Call)
	if !ok {
		return false
	}
	return callHasAliasedArgument(call.Common(), value) && valueStoredInField(call)
}

func valueStoredInField(value ssa.Value) bool {
	return valueHasForwardUse(value, func(_ ssa.Value, reference ssa.Instruction) ([]ssa.Value, bool) {
		if store, isStore := reference.(*ssa.Store); isStore {
			_, isField := store.Addr.(*ssa.FieldAddr)
			return nil, isField
		}
		return nil, false
	})
}

// valueHasForwardUse shares referrer enumeration, ForwardedValue transitions
// and cycle handling. Its caller adds query-specific transitions and positive
// witnesses; finding a use says nothing about cleanup coverage or other uses.
func valueHasForwardUse(value ssa.Value, step func(ssa.Value, ssa.Instruction) ([]ssa.Value, bool)) bool {
	found := false
	ssaflow.WalkStates([]ssa.Value{value}, func(value ssa.Value) ssa.Value { return value }, func(value ssa.Value) ([]ssa.Value, bool) {
		if value == nil || value.Referrers() == nil {
			return nil, true
		}
		var successors []ssa.Value
		for _, reference := range *value.Referrers() {
			next, matched := step(value, reference)
			if matched {
				found = true
				return nil, false
			}
			if forwarded, ok := ssaflow.ForwardedValue(reference); ok {
				successors = append(successors, forwarded)
			}
			successors = append(successors, next...)
		}
		return successors, true
	})
	return found
}
