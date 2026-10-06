package goroutineownership

import (
	"go/token"

	"github.com/kojah/gohawk/internal/engine/heapmodel"
	"github.com/kojah/gohawk/internal/engine/lifecycle"
	proofs "github.com/kojah/gohawk/internal/engine/proof"
	"github.com/kojah/gohawk/internal/engine/ssaflow"
	ssacall "github.com/kojah/gohawk/internal/engine/ssaflow/calls"
	"github.com/kojah/gohawk/internal/engine/syntax"
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
		bindings := ssacall.CallBindingsWithin(spawn.Common(), function, nil, budget)
		if captured {
			bindings = ssacall.CallBindingsWithin(nil, function, closure, budget)
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
			if !ssacall.CallbackCaptureReadOnly(closure, binding.Supplied, budget) {
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

// Cleanup target discovery relates returned callbacks to sibling resources.
// Exact returned-cleanup and legacy possible cleanup both supply only owner
// participation here. The caller preserves budget availability separately;
// neither relation establishes completion of the spawned worker.

// A factory's returned callback is cleanup evidence only when a returned
// literal captures the exact sibling resource and the existing helper-use
// proof establishes its lifecycle call. Arbitrary tuple siblings prove nothing.
// https://github.com/containerd/ttrpc/blob/9e62ff76048c2565d85b243f70adc66aeea73290/server_test.go#L571-L582
func cleanupTargets(common *ssa.CallCommon, budget *proofs.SearchBudget) []ssa.Value {
	if receiver := ssaflow.CallReceiver(common); lifecycleOwnerWithin(receiver, budget) && lifecycleMethod(ssaflow.CallName(common)) {
		return []ssa.Value{receiver}
	}
	callback, ok := common.Value.(*ssa.Extract)
	if !ok {
		return nil
	}
	factory, ok := callback.Tuple.(*ssa.Call)
	if !ok {
		return nil
	}
	return factoryCleanupTargets(factory, callback.Index, budget)
}

func factoryCleanupTargets(factory *ssa.Call, callbackIndex int, budget *proofs.SearchBudget) []ssa.Value {
	function := ssacall.ResolvedCallee(factory.Common())
	if function == nil {
		return nil
	}
	var targets []ssa.Value
	for index := range function.Signature.Results().Len() {
		if !budget.Spend() {
			return targets
		}
		target := ssacall.CallResultWithin(factory, index, budget)
		if budget.Exhausted() {
			return targets
		}
		if !lifecycleOwnerWithin(target, budget) {
			continue
		}
		if lifecycle.ProveReturnedCleanup(function, lifecycle.ReturnedCleanupRelation{
			CallbackResult: callbackIndex, Target: index, TargetIsResult: true,
		}, lifecycle.CompletionRequest{Methods: []string{"Close", "Stop", "Shutdown"}, Budget: budget}).Proven() {
			targets = append(targets, target)
		}
	}
	// Preserve the older may-cleanup evidence below as Unknown only. A partial
	// literal return may explain shutdown participation, but cannot be promoted
	// to exact cleanup or a worker join. The shared relation above is stricter
	// and also understands forwarding factories.
	for _, block := range function.Blocks {
		for _, instruction := range block.Instrs {
			if !budget.Spend() {
				return targets
			}
			returned, ok := instruction.(*ssa.Return)
			if !ok {
				continue
			}
			closure, ok := lifecycle.ReturnedResultWithin(returned, callbackIndex, budget).(*ssa.MakeClosure)
			if !ok {
				continue
			}
			for index := range returned.Results {
				if !budget.Spend() {
					return targets
				}
				target := ssacall.CallResultWithin(factory, index, budget)
				if budget.Exhausted() {
					return targets
				}
				if lifecycleOwnerWithin(target, budget) && callbackClosesSibling(closure, lifecycle.ReturnedResultWithin(returned, index, budget), budget) {
					targets = append(targets, target)
				}
			}
		}
	}
	return targets
}

func callbackClosesSibling(closure *ssa.MakeClosure, sibling ssa.Value, budget *proofs.SearchBudget) bool {
	function, _ := closure.Fn.(*ssa.Function)
	if function == nil {
		return false
	}
	for pair := range ssacall.CallBindingsWithin(nil, function, closure, budget) {
		if !heapmodel.DefinitelySameValue(ssaflow.CapturedBindingValueWithin(pair.Supplied, budget), sibling) {
			continue
		}
		search := newHelperSearchWithin(budget)
		if search.use(function, pair.Local, trackedOwner) == actionJoin {
			return true
		}
	}
	return false
}
