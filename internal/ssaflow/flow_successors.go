package ssaflow

import (
	"go/types"

	"golang.org/x/tools/go/ssa"
)

// Three sources decide which successors of a block a path may take: the
// literal view (FeasibleSuccessors, from constant conditions and the phi
// edge the path arrived by), Boolean parameters or captures bound by a call
// (FixedValues), and an assumption about one value, such as a cleanup a
// constructor guarantees non-nil. A walk that needs a richer literal view,
// such as one that reads a callee's proven result, supplies it as Feasible.
// Every walk takes its successors from one SuccessorPolicy so each applies
// the three in the same order with the same meaning. Path guards, the branch
// history of one path, are extended per edge by Edges; what a contradiction
// means, a skipped edge or an uncertain one, stays the walk's own policy.

// SuccessorPolicy chooses the feasible successors of a block.
type SuccessorPolicy struct {
	// Feasible, when set, replaces the literal view; it must return a subset
	// of the block's successors.
	Feasible   func(block, predecessor *ssa.BasicBlock) []*ssa.BasicBlock
	Constants  FixedValues
	NonNil     ssa.Value
	NonNilType types.Type
}

// Successors returns the successors of block a path arriving from
// predecessor may take.
func (policy SuccessorPolicy) Successors(block, predecessor *ssa.BasicBlock) []*ssa.BasicBlock {
	var successors []*ssa.BasicBlock
	if policy.Feasible != nil {
		successors = policy.Feasible(block, predecessor)
	} else {
		successors = FeasibleSuccessors(block, predecessor)
	}
	successors = policy.Constants.Narrow(successors, block)
	return assumedSuccessors(successors, block, policy.NonNil, policy.NonNilType)
}

// SuccessorEdge is one feasible edge with the path guards extended across it
// and whether the branch contradicts a guard the path already carries.
type SuccessorEdge struct {
	To            *ssa.BasicBlock
	Guards        PathGuards
	Contradiction GuardContradiction
}

// Edges returns the feasible edges out of block for a path that arrived from
// predecessor carrying guards.
func (policy SuccessorPolicy) Edges(block, predecessor *ssa.BasicBlock, guards PathGuards) []SuccessorEdge {
	return policy.edgesWithin(block, predecessor, guards, nil)
}

func (policy SuccessorPolicy) edgesWithin(block, predecessor *ssa.BasicBlock, guards PathGuards, budget *SearchBudget) []SuccessorEdge {
	if !budget.Spend() {
		return nil
	}
	successors := policy.Successors(block, predecessor)
	if budget.Exhausted() {
		return nil
	}
	edges := make([]SuccessorEdge, 0, len(successors))
	for _, successor := range successors {
		if !budget.Spend() {
			return nil
		}
		extended, contradiction := guards.extendWithin(block, successor, nil, budget)
		if budget.Exhausted() {
			return nil
		}
		edges = append(edges, SuccessorEdge{To: successor, Guards: extended, Contradiction: contradiction})
	}
	return edges
}
