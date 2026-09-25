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

// acquisitionReachable reports whether some feasible path from the entry
// reaches the acquisition. The obligation walk prunes edges only after the
// acquisition; this asks the same question of the path before it. A helper
// whose result is nil whenever its error is non-nil makes a retry guarded by
// res != nil after the error check unreachable, and a resource no feasible
// path acquires owes nothing. Only a proven prune removes a path: unknown
// results keep every edge, and an exhausted budget keeps the acquisition.
// https://github.com/gan-of-culture/get-sauce/blob/d726f56e7424bde4ff31e5329f37018343956103/request/request.go#L308-L318
func (analysis *resourceAnalysis) acquisitionReachable() bool {
	type position struct{ block, predecessor *ssa.BasicBlock }
	type positionKey struct{ block, predecessor int }
	target := analysis.acquisition.Block()
	budget := analysis.budget(2000)
	reached := false
	key := func(at position) positionKey {
		predecessor := -1
		if at.predecessor != nil {
			predecessor = at.predecessor.Index
		}
		return positionKey{block: at.block.Index, predecessor: predecessor}
	}
	ssaflow.WalkStates([]position{{block: analysis.function.Blocks[0]}}, key, func(at position) ([]position, bool) {
		if at.block == target || !budget.Spend() {
			reached = true
			return nil, false
		}
		var next []position
		for _, successor := range analysis.feasibleSuccessors(resourceFlowState{block: at.block, predecessor: at.predecessor}) {
			next = append(next, position{block: successor, predecessor: at.block})
		}
		return next, true
	})
	return reached
}
