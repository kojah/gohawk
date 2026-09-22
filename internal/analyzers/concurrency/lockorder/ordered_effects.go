package lockorder

import (
	"slices"

	"github.com/kojah/gohawk/internal/passes/concurrencyfacts"
	"github.com/kojah/gohawk/internal/ssaflow"
	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/ssa"
)

// Complete effects use the same lock-state transfer as direct operations.
// Unknown calls retain the older completion/witness path; absence is not purity.
type mutexEffect struct {
	operation   mutexOperation
	identity    string
	receiver    ssa.Value
	acquired    lockAcquisition
	readRelease bool
}

func directMutexEffect(instruction ssa.Instruction) (mutexEffect, bool) {
	operation, identity, receiver, ok := mutexAction(instruction)
	if !ok {
		return mutexEffect{}, false
	}
	return mutexEffect{
		operation: operation, identity: identity, receiver: receiver,
		acquired:    acquisitionAt(instruction, lockComparisonKey(identity, receiver)),
		readRelease: readModeRelease(instruction),
	}, ok
}

func summarizedMutexEffects(pass *analysis.Pass, function *ssa.Function) map[ssa.Instruction][]mutexEffect {
	result := make(map[ssa.Instruction][]mutexEffect)
	engine, _ := summaryKnowledge.Provider(pass).Concurrency()
	if engine == nil {
		return result
	}
	budget := ssaflow.NewSearchBudget(2000)
	for _, call := range ssaflow.InstructionsOf[*ssa.Call](function) {
		if _, _, _, direct := mutexAction(call); direct {
			continue
		}
		summary := engine.AtCall(call, budget)
		if !summary.Complete() {
			continue
		}
		effects, complete := bindMutexEffects(call, summary.Operations)
		if complete {
			result[call] = effects
		}
	}
	return result
}

// No held state can arise without a direct or fully bound acquisition. Calls
// with incomplete effects only contribute ordering when another lock is held.
func hasMutexAcquisition(function *ssa.Function, summaries map[ssa.Instruction][]mutexEffect) bool {
	for _, effects := range summaries {
		for _, effect := range effects {
			if effect.operation == mutexAcquire {
				return true
			}
		}
	}
	for _, block := range function.Blocks {
		for _, instruction := range block.Instrs {
			if effect, known := directMutexEffect(instruction); known && effect.operation == mutexAcquire {
				return true
			}
		}
	}
	return false
}

func bindMutexEffects(call *ssa.Call, operations []concurrencyfacts.Operation) ([]mutexEffect, bool) {
	var effects []mutexEffect
	for _, operation := range operations {
		value := operation.Resource.Value
		if operation.Resource.Indirect || (operation.Kind != concurrencyfacts.Lock && operation.Kind != concurrencyfacts.Unlock) {
			return nil, false
		}
		identity := lockIdentityOf(value)
		if identity == "" || dynamicIndexedMutex(value) {
			return nil, false
		}
		resource, _ := lockResourcePath(value)
		class := lockComparisonKey(identity, value)
		acquired := lockAcquisition{
			class: class, position: operation.Source, resource: resource, instance: identity, widened: class != "" && class != identity,
			variant: loopVariantValue(ssaflow.NewReachingWalk(ssaflow.TransparentNone), value),
		}
		if operation.Source != call.Pos() {
			acquired = acquired.through(call)
		}
		kind := mutexAcquire
		if operation.Kind == concurrencyfacts.Unlock {
			kind = mutexRelease
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
