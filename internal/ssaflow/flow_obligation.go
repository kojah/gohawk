package ssaflow

import "golang.org/x/tools/go/ssa"

// Obligation flow is the one return-coverage walk behind every classify-then-
// flow analyzer. The analyzer labels instructions, returns, and CFG edges; this
// file carries those labels along every feasible path to each normal return
// and reports the weakest coverage it found. It decides nothing about what
// counts as a join, transfer, or opaque consumption: that policy stays beside
// the analyzer that supplies the labels.
//
// Uncertainty stays on the path where it occurs. An opaque handoff on one
// branch covers only the returns that branch reaches; an unrelated early
// return with no action at all is still a violation.

// ObligationAction is the label a lifecycle classifier gives one instruction,
// return, or edge with respect to a tracked obligation.
type ObligationAction uint8

const (
	// ObligationNone leaves the obligation untouched on this path.
	ObligationNone ObligationAction = iota
	// ObligationUnknown consumes the obligation in a way the analysis cannot
	// see through. It hides a diagnostic; it never proves settlement.
	ObligationUnknown
	// ObligationExact settles the obligation with exact evidence: a proven
	// join, release, or transfer of the tracked value.
	ObligationExact
)

// ObligationOutcome is the weakest coverage found on any feasible path from
// the obligation to a normal return.
type ObligationOutcome uint8

const (
	// ObligationViolated means some reachable normal return has no action at
	// all before it on a feasible path.
	ObligationViolated ObligationOutcome = iota
	// ObligationUncertain means every return is covered, but at least one only
	// by an opaque action.
	ObligationUncertain
	// ObligationHonored means exact actions cover every reachable return.
	ObligationHonored
)

// ObligationFlow describes one obligation and the classifier that labels the
// code after it. Start is the instruction that created the obligation; the
// walk begins with the instruction after it. NonNil, when set, restricts the
// walk to paths feasible when that value is non-nil. Return and Edge are
// optional; an edge action attaches to that successor's path only.
type ObligationFlow struct {
	Start       ssa.Instruction
	NonNil      ssa.Value
	Instruction func(ssa.Instruction) ObligationAction
	Return      func(*ssa.Return) ObligationAction
	Edge        func(from, to *ssa.BasicBlock) ObligationAction
	// Successors, when set, replaces the default feasibility of a block's
	// successors with the analyzer's richer view, such as one that prunes a
	// branch on a callee's proven result. The NonNil assumption is applied
	// on top of it either way. It must return a subset of the block's
	// successors; it never proves an action.
	Successors func(block, predecessor *ssa.BasicBlock) []*ssa.BasicBlock
}

// feasibleSuccessors applies the caller's feasibility view, or the default
// literal one, and then the non-nil assumption.
func (flow ObligationFlow) feasibleSuccessors(block, predecessor *ssa.BasicBlock) []*ssa.BasicBlock {
	successors := FeasibleSuccessors(block, predecessor)
	if flow.Successors != nil {
		successors = flow.Successors(block, predecessor)
	}
	return nonNilSuccessors(successors, block, flow.NonNil)
}

// EvaluateObligation carries the classifier's labels along every feasible
// path after the obligation and returns the weakest return coverage found. A
// Start outside any block yields Honored: there are no paths to judge.
func EvaluateObligation(flow ObligationFlow) ObligationOutcome {
	index := InstructionIndex(flow.Start)
	if index < 0 {
		return ObligationHonored
	}
	return obligationOutcome([]obligationState{{block: flow.Start.Block(), index: index + 1}}, flow)
}

// obligationState is one path's position and the strongest action seen on it.
type obligationState struct {
	block       *ssa.BasicBlock
	predecessor *ssa.BasicBlock
	index       int
	covered     ObligationAction
}

type obligationKey struct {
	block       int
	predecessor int
	index       int
	covered     ObligationAction
}

func (state obligationState) key() obligationKey {
	predecessor := -1
	if state.predecessor != nil {
		predecessor = state.predecessor.Index
	}
	return obligationKey{block: state.block.Index, predecessor: predecessor, index: state.index, covered: state.covered}
}

// obligationOutcome is the walk shared by EvaluateObligation and the Boolean
// UnownedReturn family. Coverage only strengthens along a path, so a state is
// keyed by its coverage and a block is revisited only under a different one.
// The walk ends at the first violated return; the violation is the answer.
func obligationOutcome(initial []obligationState, flow ObligationFlow) ObligationOutcome {
	outcome := ObligationHonored
	WalkStates(initial, obligationState.key, func(state obligationState) ([]obligationState, bool) {
		for _, instruction := range state.block.Instrs[state.index:] {
			state.covered = max(state.covered, flow.Instruction(instruction))
			if InstructionTerminatesControlFlow(instruction) {
				return nil, true
			}
			returned, ok := instruction.(*ssa.Return)
			if !ok {
				continue
			}
			covered := state.covered
			if flow.Return != nil {
				covered = max(covered, flow.Return(returned))
			}
			switch covered {
			case ObligationNone:
				outcome = ObligationViolated
				return nil, false
			case ObligationUnknown:
				outcome = ObligationUncertain
			case ObligationExact:
			}
		}
		successors := flow.feasibleSuccessors(state.block, state.predecessor)
		next := make([]obligationState, 0, len(successors))
		for _, successor := range successors {
			covered := state.covered
			if flow.Edge != nil {
				covered = max(covered, flow.Edge(state.block, successor))
			}
			next = append(next, obligationState{block: successor, predecessor: state.block, covered: covered})
		}
		return next, true
	})
	return outcome
}

// exactOrNone lifts a Boolean ownership predicate to the two-level lattice the
// UnownedReturn family needs: an owning action is exact, anything else none.
func exactOrNone(owns func(ssa.Instruction) bool) func(ssa.Instruction) ObligationAction {
	return func(instruction ssa.Instruction) ObligationAction {
		if owns(instruction) {
			return ObligationExact
		}
		return ObligationNone
	}
}

func exactOrNoneReturn(allow func(*ssa.Return) bool) func(*ssa.Return) ObligationAction {
	if allow == nil {
		return nil
	}
	return func(returned *ssa.Return) ObligationAction {
		if allow(returned) {
			return ObligationExact
		}
		return ObligationNone
	}
}

func exactOrNoneEdge(owns OwnershipEdge) func(from, to *ssa.BasicBlock) ObligationAction {
	if owns == nil {
		return nil
	}
	return func(from, to *ssa.BasicBlock) ObligationAction {
		if owns(from, to) {
			return ObligationExact
		}
		return ObligationNone
	}
}
