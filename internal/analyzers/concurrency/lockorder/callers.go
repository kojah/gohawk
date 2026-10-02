package lockorder

import (
	"github.com/kojah/gohawk/internal/ssaflow"
	"golang.org/x/tools/go/ssa"
)

// Package caller discovery feeds two different preconditions. Conditional
// cleanup requires a complete bounded operand census, including initialization;
// exclusive ownership retains its synchronous static-call inventory outside
// initialization. A cleanup cutoff must not publish a partial caller set or
// shorten the independent exclusive inventory.
type lockCallerInventory struct {
	conditional map[*ssa.Function]conditionalCallerSet
	exclusive   map[*ssa.Function][]*ssa.Call
}

type conditionalCallerSet struct {
	calls   []*ssa.Call
	escaped bool
}

// callerSetBudget bounds the whole-package conditional caller census rather
// than one candidate. Exhaustion invalidates every conditional caller set.
const callerSetBudget = 20_000

func collectLockCallers(initialization *ssa.Function, functions []*ssa.Function, budget *ssaflow.SearchBudget) lockCallerInventory {
	inventory := lockCallerInventory{
		conditional: map[*ssa.Function]conditionalCallerSet{},
		exclusive:   map[*ssa.Function][]*ssa.Call{},
	}
	if initialization != nil {
		inventory.collect(initialization, false, budget)
	}
	// Include generated bodies: an omitted caller cannot establish a universal
	// private-function precondition. The caller selects the package scope.
	for _, function := range functions {
		if function != nil {
			inventory.collect(function, true, budget)
		}
	}
	return inventory
}

func (inventory *lockCallerInventory) collect(function *ssa.Function, exclusive bool, budget *ssaflow.SearchBudget) {
	for instruction := range ssaflow.InstructionsWithin(function, nil) {
		if exclusive {
			if call, ok := instruction.(*ssa.Call); ok {
				if callee := call.Common().StaticCallee(); callee != nil && !call.Common().IsInvoke() {
					inventory.exclusive[callee] = append(inventory.exclusive[callee], call)
				}
			}
		}
		if inventory.conditional == nil {
			continue
		}
		if !budget.Spend() {
			inventory.conditional = nil
			continue
		}
		inventory.collectConditional(instruction)
	}
}

func (inventory *lockCallerInventory) collectConditional(instruction ssa.Instruction) {
	if _, debug := instruction.(*ssa.DebugRef); debug {
		return
	}
	for _, operand := range instruction.Operands(nil) {
		if operand == nil {
			continue
		}
		callee, ok := (*operand).(*ssa.Function)
		if !ok || callee.Object() == nil || callee.Object().Exported() || callee.Signature.Recv() != nil {
			continue
		}
		entry := inventory.conditional[callee]
		call, synchronous := instruction.(*ssa.Call)
		if synchronous && operand == &call.Common().Value && len(entry.calls) < 32 {
			entry.calls = append(entry.calls, call)
		} else {
			entry.escaped = true
		}
		inventory.conditional[callee] = entry
	}
}
