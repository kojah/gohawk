package goroutineownership

import (
	"github.com/kojah/gohawk/internal/heapmodel"
	"github.com/kojah/gohawk/internal/lifecycle"
	"github.com/kojah/gohawk/internal/ssaflow"
	"golang.org/x/tools/go/ssa"
)

// Cleanup target discovery relates returned callbacks to sibling resources.
// Exact returned-cleanup and legacy possible cleanup both supply only owner
// participation here. The caller preserves budget availability separately;
// neither relation establishes completion of the spawned worker.

// A factory's returned callback is cleanup evidence only when a returned
// literal captures the exact sibling resource and the existing helper-use
// proof establishes its lifecycle call. Arbitrary tuple siblings prove nothing.
// https://github.com/containerd/ttrpc/blob/9e62ff76048c2565d85b243f70adc66aeea73290/server_test.go#L571-L582
func cleanupTargets(common *ssa.CallCommon, budget *ssaflow.SearchBudget) []ssa.Value {
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

func factoryCleanupTargets(factory *ssa.Call, callbackIndex int, budget *ssaflow.SearchBudget) []ssa.Value {
	function := ssaflow.ResolvedCallee(factory.Common())
	if function == nil {
		return nil
	}
	var targets []ssa.Value
	for index := range function.Signature.Results().Len() {
		if !budget.Spend() {
			return targets
		}
		target := ssaflow.CallResultWithin(factory, index, budget)
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
				target := ssaflow.CallResultWithin(factory, index, budget)
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

func callbackClosesSibling(closure *ssa.MakeClosure, sibling ssa.Value, budget *ssaflow.SearchBudget) bool {
	function, _ := closure.Fn.(*ssa.Function)
	if function == nil {
		return false
	}
	for pair := range ssaflow.CallBindingsWithin(nil, function, closure, budget) {
		if !heapmodel.DefinitelySameValue(ssaflow.CapturedBindingValueWithin(pair.Supplied, budget), sibling) {
			continue
		}
		search := newHelperSearch()
		search.budget = budget
		if search.use(function, pair.Local, trackedOwner) == actionJoin {
			return true
		}
	}
	return false
}
