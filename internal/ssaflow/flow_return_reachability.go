package ssaflow

import "golang.org/x/tools/go/ssa"

// NormalReturnReachableFrom reports whether block can reach a normal return
// without first invoking a control-flow terminating API.
func NormalReturnReachableFrom(block *ssa.BasicBlock) bool {
	return NormalReturnReachableWith(block, nil)
}

// NormalReturnReachableWith is NormalReturnReachableFrom with the catalog of
// terminating calls extended by a terminator.
func NormalReturnReachableWith(block *ssa.BasicBlock, terminates Terminator) bool {
	return ProveNormalReturnWithin(block, terminates, nil).Proven()
}

// NormalReturnProof distinguishes a reachable return from a completed search
// finding none. Unknown, including cutoff, never proves absence. Witness is
// present only when reachability is proven.
type NormalReturnProof struct {
	Proof
	Witness *ssa.Return
}

// ProveNormalReturnWithin shares queued visits, instruction visits and
// termination queries with budget. It retains the default CFG policy: no
// branch assumptions, and terminating calls stop only their own paths.
// A nil budget leaves the search unbounded; a missing block is unknown.
func ProveNormalReturnWithin(block *ssa.BasicBlock, terminates Terminator, budget *SearchBudget) NormalReturnProof {
	if block == nil {
		return NormalReturnProof{Proof: Proof{Reason: EvidenceUnavailable}}
	}
	var witness *ssa.Return
	WalkStatesWithin([]*ssa.BasicBlock{block}, func(candidate *ssa.BasicBlock) *ssa.BasicBlock { return candidate },
		func(candidate *ssa.BasicBlock) ([]*ssa.BasicBlock, bool) {
			for _, instruction := range candidate.Instrs {
				if !budget.Spend() {
					return nil, false
				}
				terminated := InstructionTerminatesWithin(instruction, terminates, budget)
				if budget.Exhausted() {
					return nil, false
				}
				if terminated {
					return nil, true
				}
				if returned, ok := instruction.(*ssa.Return); ok {
					witness = returned
					return nil, false
				}
			}
			return candidate.Succs, true
		}, budget)
	if budget.Exhausted() {
		return NormalReturnProof{Proof: Proof{Reason: EvidenceBudgetExhausted}}
	}
	if witness != nil {
		return NormalReturnProof{Proof: Proof{
			State: EvidenceProven, Reason: EvidenceStructuralWalk, Provenance: EvidenceFromLocalSSA,
		}, Witness: witness}
	}
	return NormalReturnProof{Proof: Proof{
		State: EvidenceDisproven, Reason: EvidenceNotFound, Provenance: EvidenceFromLocalSSA,
	}}
}
