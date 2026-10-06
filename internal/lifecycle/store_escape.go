package lifecycle

import (
	"go/token"
	"go/types"

	"github.com/kojah/gohawk/internal/heapmodel"
	proofs "github.com/kojah/gohawk/internal/proof"
	"github.com/kojah/gohawk/internal/ssaflow"
	cfg "github.com/kojah/gohawk/internal/ssaflow/cfg"
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
	cfg.WalkStates([]ssa.Value{value}, func(value ssa.Value) ssa.Value { return value }, func(value ssa.Value) ([]ssa.Value, bool) {
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

func closureCallsValue(closure *ssa.MakeClosure, target ssa.Value) bool {
	return closureCallsCapturedValue(closure, func(binding ssa.Value) bool {
		return heapmodel.CapturedBindingMatches(binding, target)
	})
}

func closureCallsCapturedValue(closure *ssa.MakeClosure, owns func(ssa.Value) bool) bool {
	function, ok := closure.Fn.(*ssa.Function)
	if !ok {
		return false
	}
	bindings := ssaflow.ClosureBindingPairs(function, closure)
	for _, block := range function.Blocks {
		for _, candidate := range block.Instrs {
			if nested, ok := candidate.(*ssa.MakeClosure); ok && closureCallsCapturedValue(nested, func(binding ssa.Value) bool {
				for _, pair := range bindings {
					if heapmodel.CapturedBindingMatches(binding, pair.Free) && owns(pair.Binding) {
						return true
					}
				}
				return false
			}) {
				return true
			}
			common := ssaflow.InstructionCall(candidate)
			if common == nil {
				continue
			}
			for _, pair := range bindings {
				if heapmodel.ValueDerivesFrom(common.Value, pair.Free) && owns(pair.Binding) {
					return true
				}
			}
		}
	}
	return false
}

// MayContainValue reports whether owner may be an aggregate or closure that
// transitively contains value. Possible containment only: it can hide a
// diagnostic behind an opaque owner, never prove that the owner settles it.
func MayContainValue(owner, value ssa.Value) bool {
	return ProveMayContainValueWithin(owner, value, nil).Proven()
}

// ProveMayContainValueWithin shares value, aggregate and capture traversal with
// budget. Graph construction, graph-query and type internals remain separate.
// Cutoff is unknown; a negative means no modeled containment, not actual absence.
func ProveMayContainValueWithin(owner, value ssa.Value, budget *proofs.SearchBudget) proofs.Proof {
	return proveContainmentWithin(owner, value, budget, func() bool { return heapmodel.Contains(owner, value) })
}

func proveContainmentWithin(owner, value ssa.Value, budget *proofs.SearchBudget, graphContains func() bool) proofs.Proof {
	if !budget.Spend() {
		return proofs.Proof{Reason: proofs.EvidenceBudgetExhausted}
	}
	found := false
	if heapmodel.CanHoldReference(owner.Type()) {
		search := newOwnershipSearch(nil)
		search.budget = budget
		found = valueOwnsValueWithin(owner, value, budget) || search.aggregateStoresValue(owner, value)
		// Graph containment adds copies, merges and captured cells; visible
		// constructors remain the recursive search's responsibility.
		if !found && !search.exhausted() && budget.Spend() {
			found = graphContains()
		}
	}
	if budget.Exhausted() || budget.PoolExhausted() {
		return proofs.Proof{Reason: proofs.EvidenceBudgetExhausted}
	}
	state, reason := proofs.EvidenceDisproven, proofs.EvidenceNotFound
	if found {
		state, reason = proofs.EvidenceProven, proofs.EvidenceStructuralWalk
	}
	return proofs.Proof{State: state, Reason: reason, Provenance: proofs.EvidenceFromLocalSSA}
}

// ProveMayContainValueAtWithin asks structural may-containment with its graph
// fallback observed at at. Later visible stores remain possible structural
// ownership, never an exact before-call guarantee or cleanup. The graph observes
// a call's argument before the callee's stores. Structural visits share budget;
// graph construction, graph-query and type internals remain independent. Cutoff
// is unknown; a completed negative means no modeled relation, including when
// the graph cannot answer. A nil budget retains the existing default policy.
func ProveMayContainValueAtWithin(owner, value ssa.Value, at ssa.Instruction, budget *proofs.SearchBudget) proofs.Proof {
	return proveContainmentWithin(owner, value, budget, func() bool {
		contained, known := heapmodel.ContainsAt(owner, value, at)
		return known && contained
	})
}

func valueOwnsValue(owner, value ssa.Value) bool { return valueOwnsValueWithin(owner, value, nil) }

func valueOwnsValueWithin(owner, value ssa.Value, budget *proofs.SearchBudget) bool {
	found := false
	cfg.WalkStatesWithin([]ssa.Value{owner}, func(owner ssa.Value) ssa.Value { return owner }, func(owner ssa.Value) ([]ssa.Value, bool) {
		if owner == nil {
			return nil, true
		}
		// A possible alias is evidence before wrappers are peeled. Only
		// wrappers and closure captures extend this narrow ownership query;
		// it does not independently fan out phi alternatives or call results.
		if budget.Spend() && heapmodel.MayAlias(owner, value) {
			found = true
			return nil, false
		}
		if inner, ok := ssaflow.UnwrapTransparentValue(
			owner, ssaflow.TransparentChangeInterface|ssaflow.TransparentChangeType|ssaflow.TransparentConvert|ssaflow.TransparentMakeInterface,
		); ok {
			return []ssa.Value{inner}, true
		}
		var successors []ssa.Value
		if closure, ok := owner.(*ssa.MakeClosure); ok {
			found = closureBindingsOwnValueWithin(closure, value, budget, func(binding ssa.Value) bool {
				successors = append(successors, binding)
				return false
			})
		}
		return successors, !found
	}, budget)
	return found && !budget.Exhausted() && !budget.PoolExhausted()
}

// Capture identity and cell contents are shared mechanics. The caller chooses
// whether to follow only nested callbacks or also owning aggregates.
func closureBindingsOwnValueWithin(closure *ssa.MakeClosure, value ssa.Value, budget *proofs.SearchBudget, owns func(ssa.Value) bool) bool {
	for _, binding := range closure.Bindings {
		if !budget.Spend() {
			return false
		}
		if heapmodel.CapturedBindingMatchesWithin(binding, value, budget) || owns(ssaflow.CapturedBindingValueWithin(binding, budget)) {
			return !budget.Exhausted() && !budget.PoolExhausted()
		}
	}
	return false
}

// ProvePossibleClosureCaptureWithin reports whether any closure reaching callback
// may transitively contain target. Phi alternatives are transparent; conversions
// and loads remain opaque. A positive capture is not proof of invocation or
// cleanup. Cutoff is unknown, and a negative means no modeled capture.
// Graph construction and graph queries retain their independent bounds.
func ProvePossibleClosureCaptureWithin(callback, target ssa.Value, budget *proofs.SearchBudget) proofs.Proof {
	if budget == nil {
		budget = proofs.NewSearchBudget(proofs.QueryBudget)
	}
	captured := ssaflow.NewReachingWalk(ssaflow.TransparentNone).Within(budget).Any(callback, func(_ ssaflow.ReachingWalk, leaf ssa.Value) bool {
		if _, closure := leaf.(*ssa.MakeClosure); !closure {
			return false
		}
		return ProveMayContainValueWithin(leaf, target, budget).Proven()
	})
	if budget.Exhausted() {
		return proofs.Proof{State: proofs.EvidenceUnknown, Reason: proofs.EvidenceBudgetExhausted}
	}
	if captured {
		return proofs.Proof{State: proofs.EvidenceProven, Reason: proofs.EvidenceCapturedByClosure}
	}
	return proofs.Proof{State: proofs.EvidenceDisproven, Reason: proofs.EvidenceNotFound}
}
