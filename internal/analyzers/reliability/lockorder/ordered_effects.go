package lockorder

// Complete ordered mutex summaries complement the positive class witnesses.
// They bind parameter locks to caller objects and retain the held set at each
// acquisition, including releases before/after an acquisition. An unavailable
// sequence falls back to the existing witness search, never to assumed purity.

import (
	"slices"

	"github.com/kojah/gohawk/internal/passes/concurrencyfacts"
	"github.com/kojah/gohawk/internal/ssaflow"
	"golang.org/x/tools/go/ssa"
)

type heldWitness struct {
	value       ssa.Value
	acquisition lockAcquisition
}

func (flow lockFlowContext) recordSummarizedOrder(instruction ssa.Instruction, held []string, origins map[string]lockAcquisition) bool {
	call, ok := instruction.(*ssa.Call)
	if !ok {
		return false
	}
	if _, _, _, mutex := mutexAction(call); mutex {
		return false
	}
	engine, ok := flow.pass.ResultOf[concurrencyfacts.Analyzer].(*concurrencyfacts.Engine)
	if !ok {
		return false
	}
	result := engine.AtCall(call, ssaflow.NewSearchBudget(2000))
	if result.Reason != "" {
		return false
	}
	for _, operation := range result.Operations {
		if operation.Kind != concurrencyfacts.Lock && operation.Kind != concurrencyfacts.Unlock {
			return false
		}
	}
	var active []heldWitness
	for _, identity := range held {
		values := flow.lockValues[identity]
		if len(values) != 0 && !flow.unprovenRelease[identity] {
			active = append(active, heldWitness{value: values[0], acquisition: origins[identity]})
		}
	}
	flow.recordMutexEffects(call, result.Operations, active)
	return true
}

func (flow lockFlowContext) recordMutexEffects(call *ssa.Call, operations []concurrencyfacts.Operation, active []heldWitness) {
	for _, operation := range operations {
		value := operation.Resource.Value
		if operation.Kind == concurrencyfacts.Unlock {
			active = slices.DeleteFunc(active, func(held heldWitness) bool { return ssaflow.DefinitelySameValue(held.value, value) })
			continue
		}
		acquired := lockAcquisition{class: lockClassOf(value), position: operation.Source}
		if operation.Source != call.Pos() {
			acquired = acquired.through(call)
		}
		var guards []ssa.Value
		for _, held := range active {
			if global, ok := held.value.(*ssa.Global); ok && !held.acquisition.read {
				guards = append(guards, global)
			}
		}
		for _, held := range active {
			flow.relations.record(flow.pass, held.acquisition, acquired, guards...)
		}
		active = append(active, heldWitness{value: value, acquisition: acquired})
	}
}

func (flow lockFlowContext) exclusiveGlobalGuards(held, readHeld []string) []ssa.Value {
	var guards []ssa.Value
	for _, identity := range held {
		if slices.Contains(readHeld, identity) || flow.unprovenRelease[identity] {
			continue
		}
		values := flow.lockValues[identity]
		if len(values) != 1 {
			continue
		}
		if global, ok := values[0].(*ssa.Global); ok && concurrencyfacts.MutexPointer(global.Type()) {
			guards = append(guards, global)
		}
	}
	return guards
}
