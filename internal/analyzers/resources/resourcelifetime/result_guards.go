package resourcelifetime

import (
	"github.com/kojah/gohawk/internal/ssaflow"
	"golang.org/x/tools/go/ssa"
)

// Unconditional result guarantees refine only feasible edges. They do not
// create acquisition contracts, infer cleanup, or replace error/owner relations.
// Unknown results retain the existing paths and their reporting policy.
func (analysis *resourceAnalysis) feasibleSuccessors(state resourceFlowState) []*ssa.BasicBlock {
	if analysis.summaries == nil {
		return ssaflow.FeasibleSuccessors(state.block, state.predecessor)
	}
	return analysis.summaries.FeasibleSuccessors(state.block, state.predecessor, analysis.budget(2000))
}
