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

func lockSuccessorStates(
	pass *analysis.Pass,
	state lockFlowState,
	held, readHeld, deferred []string,
	guards map[string]lockGuard,
	origins map[string]lockAcquisition,
) []lockFlowState {
	block := state.block
	states := make([]lockFlowState, 0, len(block.Succs))
	// A local release flag can merge true and false after only one branch
	// unlocked. Preserve the incoming edge for the shared constant-phi query;
	// exploring both values invents a still-held return on the released path.
	// Carried constants and stable parameter constraints are applied separately.
	// https://github.com/enetx/surf/blob/7da0502899af06f8318f95e632797cb2ac0c6c20/pkg/connectproxy/connectproxy.go#L256-L294
	feasible := summaryKnowledge.Provider(pass).FeasibleSuccessors(block, state.predecessor, ssaflow.NewSearchBudget(ssaflow.SummaryBudget))
	for index, successor := range block.Succs {
		if !slices.Contains(feasible, successor) {
			traceInfeasibleLockBranch(pass, block, "predecessor-constant-branch-infeasible")
			continue
		}
		constraints, compatible := extendLockConstraints(state.constraints, block, index == 0)
		if !compatible {
			traceInfeasibleLockBranch(pass, block, "stable-parameter-branch-infeasible")
			continue
		}
		if branch, ok := block.Instrs[len(block.Instrs)-1].(*ssa.If); ok {
			if truth, known := lockBooleanValue(branch.Cond, state.constants); known && truth != (index == 0) {
				traceInfeasibleLockBranch(pass, block, "carried-constant-branch-infeasible")
				continue
			}
		}
		nextCondition, nextValue := "", false
		if condition, ok := blockCondition(block); ok && len(block.Succs) == 2 {
			nextCondition, nextValue = condition, index == 0
			if guardConflicts(held, guards, condition, nextValue) {
				traceInfeasibleLockBranch(pass, block, "repeated-condition-infeasible")
				continue
			}
		}
		states = append(states, lockFlowState{
			block: successor, predecessor: block, held: held, readHeld: readHeld, deferred: deferred, guards: guards, origins: origins,
			condition: nextCondition, conditionValue: nextValue,
			constants:   state.constants,
			constraints: constraints,
		})
	}
	return states
}
