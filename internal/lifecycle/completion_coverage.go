package lifecycle

import (
	"github.com/kojah/gohawk/internal/ssaflow"
	"golang.org/x/tools/go/ssa"
)

// ProveMethodCallCoverageWithin shares witness and normal-return coverage work
// with budget. A cutoff supplies unknown, never completed coverage or its absence.
// Callbacks should use the same budget. Nil retains the default unbounded walk.
func ProveMethodCallCoverageWithin(
	function *ssa.Function, calls func(ssa.Instruction) bool, coverage CompletionCoverage, nonNil ssa.Value, budget *ssaflow.SearchBudget,
) ssaflow.Proof {
	if function == nil || len(function.Blocks) == 0 {
		return ssaflow.Proof{State: ssaflow.EvidenceUnknown, Reason: ssaflow.EvidenceUnavailable}
	}
	return proveMethodCoverageWithin(function, function.Blocks, calls, coverage, ssaflow.EntryAssumptions{NonNil: nonNil}, budget)
}

// Keep the independent return/action witness contract of ordinary coverage.
// The shared obligation walk then proves ordering on feasible return paths;
// witnessing an action on an unrelated path alone cannot prove completion.
func proveMethodCoverageWithin(
	function *ssa.Function, blocks []*ssa.BasicBlock, calls func(ssa.Instruction) bool,
	coverage CompletionCoverage, assumptions ssaflow.EntryAssumptions, budget *ssaflow.SearchBudget,
) ssaflow.Proof {
	missing := ssaflow.Proof{State: ssaflow.EvidenceDisproven, Reason: ssaflow.EvidenceUnavailable}
	cut := ssaflow.Proof{State: ssaflow.EvidenceUnknown, Reason: ssaflow.EvidenceBudgetExhausted}
	hasReturn, hasAction := false, false
	for _, block := range blocks {
		for _, instruction := range block.Instrs {
			if !budget.Spend() {
				return cut
			}
			if _, ok := instruction.(*ssa.Return); ok {
				hasReturn = true
			}
			if !hasAction {
				hasAction = calls(instruction)
			}
			if budget.Exhausted() {
				return cut
			}
			if coverage == CoverageAnywhere && hasAction {
				return ssaflow.Proof{State: ssaflow.EvidenceProven, Reason: ssaflow.EvidenceCalledCompletion}
			}
		}
	}
	if !hasReturn || !hasAction {
		return missing
	}
	// A completed census prevents vacuous success on no-return/no-action bodies.
	// Budget uncertainty from either the flow or its predicate cannot settle it.
	outcome := ssaflow.EvaluateObligationFromEntry(function, ssaflow.ObligationFlow{
		Budget: budget, NonNil: assumptions.NonNil, NonNilType: assumptions.NonNilType, Constants: assumptions.Constants,
		Instruction: ssaflow.ExactOrNone(calls),
	})
	if budget.Exhausted() || outcome == ssaflow.ObligationUncertain {
		return cut
	}
	if outcome != ssaflow.ObligationHonored {
		return missing
	}
	return ssaflow.Proof{State: ssaflow.EvidenceProven, Reason: ssaflow.EvidenceCalledCompletion}
}
