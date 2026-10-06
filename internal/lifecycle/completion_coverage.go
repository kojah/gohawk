package lifecycle

import (
	proofs "github.com/kojah/gohawk/internal/proof"
	"github.com/kojah/gohawk/internal/ssaflow"
	"golang.org/x/tools/go/ssa"
)

// ProveMethodCallCoverageWithin shares witness and normal-return coverage work
// with budget. A cutoff supplies unknown, never completed coverage or its absence.
// Callbacks should use the same budget. Nil retains the default unbounded walk.
func ProveMethodCallCoverageWithin(
	function *ssa.Function, calls func(ssa.Instruction) bool, coverage CompletionCoverage, nonNil ssa.Value, budget *proofs.SearchBudget,
) proofs.Proof {
	return proveMethodCallCoverageAssumingWithin(function, calls, coverage, ssaflow.EntryAssumptions{NonNil: nonNil}, budget)
}

// Constant-bound census must finish before its blocks become witness evidence.
// The caller retains one budget through that census and the coverage walk.
func proveMethodCallCoverageAssumingWithin(
	function *ssa.Function, calls func(ssa.Instruction) bool, coverage CompletionCoverage,
	assumptions ssaflow.EntryAssumptions, budget *proofs.SearchBudget,
) proofs.Proof {
	if function == nil || len(function.Blocks) == 0 {
		return proofs.Proof{State: proofs.EvidenceUnknown, Reason: proofs.EvidenceUnavailable}
	}
	blocks := function.Blocks
	if len(assumptions.Constants) != 0 {
		blocks = ssaflow.ReachableBlocksAssumingWithin(function, assumptions.Constants, budget)
		if budget.Exhausted() {
			return proofs.Proof{State: proofs.EvidenceUnknown, Reason: proofs.EvidenceBudgetExhausted}
		}
	}
	return proveMethodCoverageWithin(function, blocks, calls, coverage, assumptions, budget)
}

// Keep the independent return/action witness contract of ordinary coverage.
// The shared obligation walk then proves ordering on feasible return paths;
// witnessing an action on an unrelated path alone cannot prove completion.
func proveMethodCoverageWithin(
	function *ssa.Function, blocks []*ssa.BasicBlock, calls func(ssa.Instruction) bool,
	coverage CompletionCoverage, assumptions ssaflow.EntryAssumptions, budget *proofs.SearchBudget,
) proofs.Proof {
	missing := proofs.Proof{State: proofs.EvidenceDisproven, Reason: proofs.EvidenceUnavailable}
	cut := proofs.Proof{State: proofs.EvidenceUnknown, Reason: proofs.EvidenceBudgetExhausted}
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
				return proofs.Proof{State: proofs.EvidenceProven, Reason: proofs.EvidenceCalledCompletion}
			}
		}
	}
	if !hasReturn || !hasAction {
		return missing
	}
	// A completed census prevents vacuous success on no-return/no-action bodies.
	// Budget uncertainty from either the flow or its predicate cannot settle it.
	return proveMethodReturnCoverageWithin(function, calls, assumptions, budget)
}

// This is the one feasible-return walk, shared by ordinary witness coverage
// and the existing exact-type path. Predicate and CFG work share the allowance.
func proveMethodReturnCoverageWithin(
	function *ssa.Function, calls func(ssa.Instruction) bool, assumptions ssaflow.EntryAssumptions, budget *proofs.SearchBudget,
) proofs.Proof {
	cut := proofs.Proof{State: proofs.EvidenceUnknown, Reason: proofs.EvidenceBudgetExhausted}
	missing := proofs.Proof{State: proofs.EvidenceDisproven, Reason: proofs.EvidenceUnavailable}
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
	return proofs.Proof{State: proofs.EvidenceProven, Reason: proofs.EvidenceCalledCompletion}
}
