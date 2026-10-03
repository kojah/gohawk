package lockorder

import (
	"slices"

	"github.com/kojah/gohawk/internal/passes/concurrencyfacts"
	"github.com/kojah/gohawk/internal/ssaflow"
	"golang.org/x/tools/go/ssa"
)

// Complete effects use the same lock-state transfer as direct operations.
// Unknown calls retain the older completion/witness path; absence is not purity.
type mutexEffect struct {
	operation mutexOperation
	identity  string
	receiver  ssa.Value
	acquired  lockAcquisition
}

func directMutexEffectWithin(instruction ssa.Instruction, budget *ssaflow.SearchBudget) (mutexEffect, bool) {
	operation, identity, receiver, ok := mutexActionWithin(instruction, budget)
	if !ok {
		return mutexEffect{}, false
	}
	effect := mutexEffect{
		operation: operation, identity: identity, receiver: receiver,
		acquired: acquisitionAtWithin(instruction, lockComparisonKey(identity, receiver), budget),
	}
	if budget.Exhausted() || budget.PoolExhausted() {
		return mutexEffect{}, false
	}
	return effect, true
}

func bindMutexEffects(call *ssa.Call, operations []concurrencyfacts.Operation, budget *ssaflow.SearchBudget) ([]mutexEffect, bool) {
	var effects []mutexEffect
	for _, operation := range operations {
		if !budget.Spend() {
			return nil, false
		}
		value := operation.Resource.Value
		if operation.Resource.Indirect || (operation.Kind != concurrencyfacts.Lock && operation.Kind != concurrencyfacts.Unlock) {
			return nil, false
		}
		identity := lockIdentityWithin(value, budget)
		if identity == "" || dynamicIndexedMutex(value) {
			return nil, false
		}
		resource, _ := lockResourcePath(value)
		class := lockComparisonKey(identity, value)
		acquired := lockAcquisition{
			class: class, position: operation.Source, resource: resource, instance: identity, widened: class != "" && class != identity,
			variant: loopVariantValue(ssaflow.NewReachingWalk(ssaflow.TransparentNone).Within(budget), value),
		}
		if operation.Source != call.Pos() {
			acquired = acquired.through(call)
		}
		kind := mutexAcquire
		if operation.Kind == concurrencyfacts.Unlock {
			kind = mutexRelease
		}
		if budget.Exhausted() {
			return nil, false
		}
		effects = append(effects, mutexEffect{operation: kind, identity: identity, receiver: value, acquired: acquired})
	}
	return effects, true
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
