package heapmodel

import (
	proofs "github.com/kojah/gohawk/internal/proof"
	"github.com/kojah/gohawk/internal/ssaflow"
	cfg "github.com/kojah/gohawk/internal/ssaflow/cfg"
	"golang.org/x/tools/go/ssa"
)

// Deferred-cell relations require complete observation and occupant evidence.
// Request allowance covers selection, reachability and relation visits; graph
// construction and state replay retain their independent costs. An interrupted
// relation is unavailable, so completion cannot fall back to another mapping.

// DeferredCellMatch distinguishes a captured cell that contains exactly the
// target from one whose every possible occupant contains it indirectly.
type DeferredCellMatch uint8

const (
	DeferredCellUnknown DeferredCellMatch = iota
	DeferredCellExact
	DeferredCellContains
)

// DeferredCellRelationWithin reads the cell when deferred calls execute. Known is
// false when either side could not be read; callers must not use a fallback
// proof in that case. A stale or unrelated occupant prevents an exact claim.
// Census, reachability, union and history visits share budget; cutoff publishes
// no relation. Graph construction/replay and points-to internals retain separate
// costs. A nil budget retains the unbounded observation policy.
func DeferredCellRelationWithin(cell *ssa.Alloc, target ssa.Value, invocation ssa.Instruction, budget *proofs.SearchBudget) (DeferredCellMatch, bool) {
	if !budget.Spend() {
		return DeferredCellUnknown, false
	}
	graph := regionsOf(cell)
	held, ok := graph.contentWhenDeferredRunWithin(cell, invocation, budget)
	if !ok || len(held) == 0 {
		return DeferredCellUnknown, false
	}
	object, ok := graph.pointsTo(target)
	if !ok {
		return DeferredCellUnknown, false
	}
	targetSlot, exact := singleSlot(object)
	isTarget, contains := exact, true
	for entry, stale := range held {
		if !budget.Spend() {
			return DeferredCellUnknown, false
		}
		if entry.region.kind == regionNil {
			continue
		}
		if entry != targetSlot || stale {
			isTarget = false
		}
		if !graph.everContainedWithin(entry, object, budget) {
			contains = false
		}
	}
	if budget.Exhausted() || budget.PoolExhausted() {
		return DeferredCellUnknown, false
	}
	if isTarget {
		return DeferredCellExact, true
	}
	if contains {
		return DeferredCellContains, true
	}
	return DeferredCellUnknown, true
}

// contentWhenDeferredRunWithin returns what the addressed slots hold when the
// function's deferred calls run: the union over every RunDefers the
// registration can reach, read before the deferred calls' own effects, or
// over every reachable return when the function defers nothing and the
// callback was registered with a test instead. A deferred literal observes
// its captured cell then, not at the registration.
func (graph *regionGraph) contentWhenDeferredRunWithin(address ssa.Value, registration ssa.Instruction, budget *proofs.SearchBudget) (pointees, bool) {
	defer graph.lock()()
	if !graph.available || registration == nil {
		return nil, false
	}
	points, available := deferredObservationPoints(graph.function, budget)
	if !available {
		return nil, false
	}
	result := pointees{}
	found := false
	for _, point := range points {
		if !cfg.InstructionMayFollowWithin(registration, point, budget) {
			continue
		}
		set, ok := graph.contentAtUnlocked(address, point)
		if !ok {
			return nil, false
		}
		for target, stale := range set {
			if !budget.Spend() {
				return nil, false
			}
			result.add(target, stale)
		}
		found = true
	}
	if budget.Exhausted() || budget.PoolExhausted() {
		return nil, false
	}
	return result, found
}

// deferredObservationPoints completes one shared census before selecting
// RunDefers or, when none exist, returns for test-registered callbacks. A prefix
// cannot establish that a later deferred execution point or return is absent.
func deferredObservationPoints(function *ssa.Function, budget *proofs.SearchBudget) ([]ssa.Instruction, bool) {
	var runs, returns []ssa.Instruction
	for instruction := range ssaflow.InstructionsWithin(function, budget) {
		switch instruction.(type) {
		case *ssa.RunDefers:
			runs = append(runs, instruction)
		case *ssa.Return:
			returns = append(returns, instruction)
		}
	}
	if budget.Exhausted() || budget.PoolExhausted() {
		return nil, false
	}
	if len(runs) != 0 {
		return runs, true
	}
	return returns, true
}
