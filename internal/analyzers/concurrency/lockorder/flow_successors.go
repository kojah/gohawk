package lockorder

import (
	"slices"

	"github.com/kojah/gohawk/internal/ssaflow"

	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/ssa"
)

// Successor selection for the lock walk: which edges of a block a state
// follows, and which are infeasible under the literal, carried-constant,
// and stable-guard evidence the state holds. Infeasibility here is always
// a proof about the branch, never a guess about a mutable field.

func lockSuccessorStates(pass *analysis.Pass, state lockFlowState, budget *ssaflow.SearchBudget) []lockFlowState {
	block := state.block
	states := make([]lockFlowState, 0, len(block.Succs))
	// A local release flag can merge true and false after only one branch
	// unlocked. Preserve the incoming edge for the shared constant-phi query;
	// exploring both values invents a still-held return on the released path.
	// Carried constants and stable parameter constraints are applied separately.
	// https://github.com/enetx/surf/blob/7da0502899af06f8318f95e632797cb2ac0c6c20/pkg/connectproxy/connectproxy.go#L256-L294
	feasible := ssaflow.SuccessorPolicy{Feasible: func(block, predecessor *ssa.BasicBlock) []*ssa.BasicBlock {
		return summaryKnowledge.Provider(pass).FeasibleSuccessors(block, predecessor, budget)
	}}.SuccessorsWithin(block, state.predecessor, budget)
	for index, successor := range block.Succs {
		if !budget.Spend() {
			return nil
		}
		if !slices.Contains(feasible, successor) {
			traceInfeasibleLockBranch(pass, block, lockReasonPredecessorConstantBranchInfeasible)
			continue
		}
		constraints, compatible := extendLockConstraints(state.constraints, block, index == 0, budget)
		if !compatible {
			traceInfeasibleLockBranch(pass, block, lockReasonStableParameterBranchInfeasible)
			continue
		}
		if branch, ok := block.Instrs[len(block.Instrs)-1].(*ssa.If); ok {
			if truth, known := lockBooleanValue(branch.Cond, state.constants); known && truth != (index == 0) {
				traceInfeasibleLockBranch(pass, block, lockReasonCarriedConstantBranchInfeasible)
				continue
			}
		}
		// Correlate only the exact condition identity accepted by the guard
		// policy. A cycle query cut short cannot make a computed value stable.
		nextCondition, nextValue := "", false
		if condition, ok := blockCondition(block, budget); ok && len(block.Succs) == 2 {
			nextCondition, nextValue = condition, index == 0
			if guardConflicts(state.held, state.guards, condition, nextValue, budget) {
				traceInfeasibleLockBranch(pass, block, lockReasonRepeatedConditionInfeasible)
				continue
			}
		}
		states = append(states, lockFlowState{
			block: successor, predecessor: block, held: state.held, readHeld: state.readHeld,
			deferred: state.deferred, guards: state.guards, origins: state.origins,
			condition: nextCondition, conditionValue: nextValue,
			constants:   state.constants,
			constraints: constraints,
		})
	}
	if budget.Exhausted() {
		return nil
	}
	return states
}
