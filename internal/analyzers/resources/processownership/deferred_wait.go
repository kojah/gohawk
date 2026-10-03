package processownership

import (
	"github.com/kojah/gohawk/internal/heapmodel"
	"github.com/kojah/gohawk/internal/lifecycle"
	"github.com/kojah/gohawk/internal/ssaflow"
	"golang.org/x/tools/go/ssa"
)

// Deferred waiters use successful Start's non-nil Process guarantee only for
// the exact captured or supplied command. Discovery and coverage share one
// allowance; interrupted evidence is unknown and cannot prove a join or leak.
type deferredWaitSearch struct {
	function *ssa.Function
	calls    []ssa.Instruction
	stores   []*ssa.Store
	loads    []*ssa.UnOp
	budget   *ssaflow.SearchBudget
}

func deferredClosureWaitsForCommand(instruction ssa.Instruction, command ssa.Value, budget *ssaflow.SearchBudget) ssaflow.EvidenceState {
	if _, ok := instruction.(*ssa.Defer); !ok {
		return ssaflow.EvidenceDisproven
	}
	common := ssaflow.InstructionCall(instruction)
	if common == nil {
		return ssaflow.EvidenceDisproven
	}
	closure, _ := common.Value.(*ssa.MakeClosure)
	if closure == nil {
		return ssaflow.EvidenceDisproven
	}
	function, _ := closure.Fn.(*ssa.Function)
	if function == nil {
		return ssaflow.EvidenceDisproven
	}
	search := deferredWaitSearch{function: function, budget: budget}
	if !search.collect() {
		return ssaflow.EvidenceUnknown
	}
	for captured := range ssaflow.ClosureBindingPairsWithin(function, closure, budget) {
		if heapmodel.CapturedBindingMatches(captured.Binding, command) {
			if proof := search.waitsOnEveryReturn(captured.Free); proof != ssaflow.EvidenceDisproven {
				return proof
			}
		}
	}
	if budget.Exhausted() {
		return ssaflow.EvidenceUnknown
	}
	// Keep capture evidence first: an unknown captured waiter must not be
	// reordered behind an argument proof by the shared positional mapping.
	for binding := range ssaflow.CallBindingsWithin(common, function, nil, budget) {
		if heapmodel.MayAlias(binding.Supplied, command) {
			if proof := search.waitsOnEveryReturn(binding.Local); proof != ssaflow.EvidenceDisproven {
				return proof
			}
		}
	}
	if budget.Exhausted() {
		return ssaflow.EvidenceUnknown
	}
	return ssaflow.EvidenceDisproven
}

// A deferred waiter guarded only by its captured Cmd.Process field may own
// reaping, but distinct loads do not establish stable identity. Preserve that
// uncertainty without teaching shared non-nil flow that possible aliases are
// equal. A Boolean condition still leaves an unowned path, and a visible field
// replacement defeats even this possible successful-Start contract.
func (search *deferredWaitSearch) guardedWait(command ssa.Value) ssaflow.EvidenceState {
	for _, store := range search.stores {
		if !search.budget.Spend() {
			return ssaflow.EvidenceUnknown
		}
		if heapmodel.ValueDerivesFrom(store.Addr, command) {
			return ssaflow.EvidenceDisproven
		}
	}
	for _, load := range search.loads {
		if !search.budget.Spend() {
			return ssaflow.EvidenceUnknown
		}
		if !osProcessDerivedFromCommand(load, command) {
			continue
		}
		result := search.coverage(command, load)
		if result.State == ssaflow.EvidenceUnknown || result.Proven() {
			return ssaflow.EvidenceUnknown
		}
	}
	return ssaflow.EvidenceDisproven
}

func (search *deferredWaitSearch) waitsOnEveryReturn(command ssa.Value) ssaflow.EvidenceState {
	// A successful Cmd.Start guarantees Cmd.Process is non-nil. Each concrete
	// Wait receiver supplies that assumption for a defensive Process guard.
	for _, candidate := range search.calls {
		if !search.budget.Spend() {
			return ssaflow.EvidenceUnknown
		}
		if !waitsForCommand(candidate, command) {
			continue
		}
		receiver := ssaflow.CallReceiver(ssaflow.InstructionCall(candidate))
		result := search.coverage(command, receiver)
		if result.State != ssaflow.EvidenceDisproven {
			return result.State
		}
	}
	return search.guardedWait(command)
}

func (search *deferredWaitSearch) coverage(command, nonNil ssa.Value) ssaflow.Proof {
	return lifecycle.ProveMethodCallCoverageWithin(search.function, func(candidate ssa.Instruction) bool {
		return waitsForCommand(candidate, command)
	}, lifecycle.CoverageEveryReturn, nonNil, search.budget)
}

func (search *deferredWaitSearch) collect() bool {
	for candidate := range ssaflow.InstructionsWithin(search.function, search.budget) {
		if ssaflow.InstructionCall(candidate) != nil {
			search.calls = append(search.calls, candidate)
		}
		switch typed := candidate.(type) {
		case *ssa.Store:
			search.stores = append(search.stores, typed)
		case *ssa.UnOp:
			search.loads = append(search.loads, typed)
		}
	}
	return !search.budget.Exhausted()
}
