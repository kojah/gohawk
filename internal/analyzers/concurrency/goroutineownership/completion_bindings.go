package goroutineownership

import (
	"go/token"

	"github.com/kojah/gohawk/internal/heapmodel"
	proofs "github.com/kojah/gohawk/internal/proof"
	"github.com/kojah/gohawk/internal/ssaflow"
	"github.com/kojah/gohawk/internal/syntax"
	"golang.org/x/tools/go/ssa"
)

// Completion binding maps worker-local handles onto the caller's exact values.
// Mutable captures and possible parameter matches stop obligation discovery;
// an aggregate root retains only possible ownership, never an exact join.

// signalSuppliedAtCall maps a worker-side channel back to the parent's value.
// A selection from a captured aggregate resolves to the aggregate itself.
// Its possible element observations and handoffs supply unknown ownership,
// never an exact join of the worker's channel.
// https://github.com/nacos-group/nacos-sdk-go/blob/002486583df5ad370ab809cd19dfd97e71b2ef6d/clients/cache/concurrent_map.go#L199-L219
func signalSuppliedAtCall(
	spawn *ssa.Go,
	function *ssa.Function,
	closure *ssa.MakeClosure,
	channel ssa.Value,
	budget *proofs.SearchBudget,
) ssa.Value { //nolint:ireturn // Completion signals retain their concrete SSA value types.
	if supplied := completionValueAtCall(spawn, function, closure, channel, budget); supplied != nil {
		return supplied
	}
	root := aggregateRootWithin(channel, budget)
	if root == channel {
		return nil
	}
	return completionValueAtCall(spawn, function, closure, root, budget)
}

// completionValueAtCall requires an exact worker-to-caller binding. Possible
// identity chooses candidates elsewhere, but cannot establish an obligation.
// Captured cells must stay read-only in the worker and stable in its caller;
// the first initializer is not a guarantee about a later asynchronous load.
func completionValueAtCall(
	spawn *ssa.Go, function *ssa.Function, closure *ssa.MakeClosure, value ssa.Value, budget *proofs.SearchBudget,
) ssa.Value { //nolint:ireturn // Completion handles retain their concrete SSA value types.
	storage := heapmodel.NewStorage(budget)
	for _, captured := range []bool{true, false} {
		// Captures retain precedence over parameters. Prepare only the selected
		// family lazily, so irrelevant arguments cannot consume its allowance.
		bindings := ssaflow.CallBindingsWithin(spawn.Common(), function, nil, budget)
		if captured {
			bindings = ssaflow.CallBindingsWithin(nil, function, closure, budget)
		}
		for binding := range bindings {
			local := value
			if captured {
				if source, ok := ssaflow.IdentitySource(value); ok {
					local = source
				}
			}
			if !storage.Same(local, binding.Local).Proven() {
				continue
			}
			if !captured || syntax.PointerStruct(binding.Supplied.Type()) != nil {
				return binding.Supplied
			}
			if !ssaflow.CallbackCaptureReadOnly(closure, binding.Supplied, budget) {
				return nil
			}
			if stored := storage.StableContent(binding.Supplied, spawn); stored.Proven() {
				return stored.Value
			}
			return nil
		}
		if budget.Exhausted() {
			return nil
		}
	}
	return nil
}

// aggregateRoot strips element, field, and map selections and the loads
// between them, returning the aggregate a projected value was read from. The
// index operands are deliberately not followed: a loop counter used to select
// an element is not the aggregate that owns it.
func aggregateRoot(value ssa.Value) ssa.Value { //nolint:ireturn // Roots retain their concrete SSA forms.
	return aggregateRootWithin(value, nil)
}

func aggregateRootWithin(value ssa.Value, budget *proofs.SearchBudget) ssa.Value { //nolint:ireturn // Roots retain their concrete SSA forms.
	for {
		if !budget.Spend() {
			return nil
		}
		if inner, ok := ssaflow.UnwrapTransparentValue(
			value,
			ssaflow.TransparentChangeInterface|ssaflow.TransparentChangeType|ssaflow.TransparentConvert|ssaflow.TransparentMakeInterface,
		); ok {
			value = inner
			continue
		}
		switch typed := value.(type) {
		case *ssa.UnOp:
			if typed.Op != token.MUL {
				return value
			}
			value = typed.X
		case *ssa.IndexAddr:
			value = typed.X
		case *ssa.FieldAddr:
			value = typed.X
		case *ssa.Index:
			value = typed.X
		case *ssa.Field:
			value = typed.X
		case *ssa.Lookup:
			value = typed.X
		default:
			return value
		}
	}
}
