package path

import (
	"go/types"

	proofs "github.com/kojah/gohawk/internal/proof"
	ssacall "github.com/kojah/gohawk/internal/ssaflow/calls"
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
	Constants  ssacall.FixedValues
	NonNil     ssa.Value
	NonNilType types.Type
}

// Successors returns the successors of block a path arriving from
// predecessor may take.
func (policy SuccessorPolicy) Successors(block, predecessor *ssa.BasicBlock) []*ssa.BasicBlock {
	return policy.SuccessorsWithin(block, predecessor, nil)
}

// SuccessorsWithin shares literal, bound-value and assumption work with budget.
// Custom feasibility hooks may share it too. Cutoff leaves the edge set unknown.
func (policy SuccessorPolicy) SuccessorsWithin(block, predecessor *ssa.BasicBlock, budget *proofs.SearchBudget) []*ssa.BasicBlock {
	var successors []*ssa.BasicBlock
	if policy.Feasible != nil {
		successors = policy.Feasible(block, predecessor)
	} else {
		successors = FeasibleSuccessorsWithin(block, predecessor, budget)
	}
	if budget.Exhausted() {
		return nil
	}
	successors = policy.Constants.NarrowWithin(successors, block, budget)
	if budget.Exhausted() {
		return nil
	}
	return assumedSuccessorsWithin(successors, block, policy.NonNil, policy.NonNilType, budget)
}

// SuccessorEdge is one feasible edge with the path guards extended across it
// and whether the branch contradicts a guard the path already carries.
type SuccessorEdge struct {
	To            *ssa.BasicBlock
	Guards        PathGuards
	Contradiction GuardContradiction
}

// EdgesWithin shares successor selection and guard extension with budget.
// Cutoff returns no complete edge set; callers must check exhaustion.
func (policy SuccessorPolicy) EdgesWithin(block, predecessor *ssa.BasicBlock, guards PathGuards, budget *proofs.SearchBudget) []SuccessorEdge {
	return policy.edgesWithFormats(block, predecessor, guards, budget, nil)
}

func (policy SuccessorPolicy) edgesWithFormats(
	block, predecessor *ssa.BasicBlock, guards PathGuards, budget *proofs.SearchBudget, formats *guardFormats,
) []SuccessorEdge {
	if !budget.Spend() {
		return nil
	}
	successors := policy.SuccessorsWithin(block, predecessor, budget)
	if budget.Exhausted() {
		return nil
	}
	edges := make([]SuccessorEdge, 0, len(successors))
	for _, successor := range successors {
		if !budget.Spend() {
			return nil
		}
		extended, contradiction := guards.extendWithFormats(block, successor, nil, budget, formats)
		if budget.Exhausted() {
			return nil
		}
		edges = append(edges, SuccessorEdge{To: successor, Guards: extended, Contradiction: contradiction})
	}
	return edges
}
