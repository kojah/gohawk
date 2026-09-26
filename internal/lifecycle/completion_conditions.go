package lifecycle

import (
	"go/constant"
	"go/token"

	"github.com/kojah/gohawk/internal/heapmodel"
	"github.com/kojah/gohawk/internal/ssaflow"
	"github.com/kojah/gohawk/internal/syntax"
	"golang.org/x/tools/go/ssa"
)

// Conditional completion preserves a narrow relation between a synchronous
// call's result and its cleanup effect. The condition belongs to that call's
// normal returns, never to every nested helper. Only explicit Boolean and
// error-nil tests are accepted; unresolved results and dispatch stay opaque.
// The condition is an ssaflow.CallCondition with a result test and no
// arguments; OutcomeAny is the ordinary, unconditional search.

// ProveCompletionOnEdge asks whether the call result tested by from establishes
// completion of request.Target on the edge to to. Instruction is derived from
// the test; only synchronous calls and exact target mappings are eligible.
// The result says nothing about the opposite edge or an untested call.
func ProveCompletionOnEdge(from, to *ssa.BasicBlock, request CompletionRequest) ssaflow.CompletionProof {
	call, condition, ok := completionEdgeCondition(from, to)
	if !ok {
		return ssaflow.CompletionProof{Proof: ssaflow.Proof{State: ssaflow.EvidenceUnknown, Reason: ssaflow.EvidenceUnavailable}}
	}
	request.Instruction, request.condition = call, condition
	request.ExactTarget, request.Coverage = true, CoverageEveryReturn
	return ProveCompletion(request)
}

func completionEdgeCondition(from, to *ssa.BasicBlock) (*ssa.Call, ssaflow.CallCondition, bool) {
	if from == nil || len(from.Instrs) == 0 || len(from.Succs) != 2 || from.Succs[0] == from.Succs[1] {
		return nil, ssaflow.CallCondition{}, false
	}
	branch, ok := from.Instrs[len(from.Instrs)-1].(*ssa.If)
	if !ok || to != from.Succs[0] && to != from.Succs[1] {
		return nil, ssaflow.CallCondition{}, false
	}
	value, outcome := completionTest(branch.Cond, to == from.Succs[0])
	call, index, ok := ssaflow.CallResultSource(value)
	if !ok || outcome == ssaflow.OutcomeAny || !ssaflow.InstructionDominates(call, branch) {
		return nil, ssaflow.CallCondition{}, false
	}
	return call, ssaflow.CallCondition{Result: index, Outcome: outcome}, true
}

func completionTest(value ssa.Value, truth bool) (ssa.Value, ssaflow.Outcome) {
	if not, ok := value.(*ssa.UnOp); ok && not.Op == token.NOT {
		return completionTest(not.X, !truth)
	}
	if comparison, ok := value.(*ssa.BinOp); ok {
		return completionComparison(comparison, truth)
	}
	if truth {
		return value, ssaflow.OutcomeTrue
	}
	return value, ssaflow.OutcomeFalse
}

func completionComparison(comparison *ssa.BinOp, truth bool) (ssa.Value, ssaflow.Outcome) {
	if comparison.Op != token.EQL && comparison.Op != token.NEQ {
		return nil, ssaflow.OutcomeAny
	}
	operand, literal := comparison.X, comparison.Y
	if _, ok := literal.(*ssa.Const); !ok {
		operand, literal = literal, operand
	}
	c, ok := literal.(*ssa.Const)
	if !ok {
		return nil, ssaflow.OutcomeAny
	}
	equal := truth == (comparison.Op == token.EQL)
	if c.IsNil() && syntax.IsErrorType(operand.Type()) {
		if equal {
			return operand, ssaflow.OutcomeNil
		}
		return operand, ssaflow.OutcomeNonNil
	}
	if c.Value == nil || c.Value.Kind() != constant.Bool {
		return nil, ssaflow.OutcomeAny
	}
	return completionTest(operand, equal == constant.BoolVal(c.Value))
}

type conditionalCompletionState struct {
	block       *ssa.BasicBlock
	predecessor *ssa.BasicBlock
	completed   bool
}

func (search *completionSearch) conditionalCoverage(
	function *ssa.Function, locals []mappedLocal, target ssa.Value, condition ssaflow.CallCondition,
) bool {
	matched, failed := false, false
	initial := []conditionalCompletionState{{block: function.Blocks[0]}}
	ssaflow.WalkStates(initial, func(state conditionalCompletionState) conditionalCompletionState { return state },
		func(state conditionalCompletionState) ([]conditionalCompletionState, bool) {
			for _, instruction := range state.block.Instrs {
				if !search.budget.Spend() {
					failed = true
					return nil, false
				}
				state.completed = state.completed || search.instructionCompletes(instruction, locals, target)
				if ssaflow.InstructionTerminatesControlFlow(instruction) {
					return nil, true
				}
				if returned, ok := instruction.(*ssa.Return); ok {
					covered, relevant := search.conditionalReturn(returned, locals, condition, state.completed)
					matched = matched || relevant
					failed = failed || relevant && !covered
					return nil, !failed
				}
			}
			var next []conditionalCompletionState
			successors := search.constants.Narrow(ssaflow.FeasibleSuccessors(state.block, state.predecessor), state.block)
			for _, successor := range successors {
				next = append(next, conditionalCompletionState{block: successor, predecessor: state.block, completed: state.completed})
			}
			return next, true
		})
	return matched && !failed && !search.budget.Exhausted()
}

func (search *completionSearch) conditionalReturn(
	returned *ssa.Return, locals []mappedLocal, condition ssaflow.CallCondition, completed bool,
) (covered, relevant bool) {
	if condition.Result >= len(returned.Results) {
		return false, true
	}
	value := returned.Results[condition.Result]
	// Resolve compiler-spilled Boolean results, but do not peel an error's
	// interface box: a typed nil error is not the nil error result.
	boolean := condition.Outcome == ssaflow.OutcomeTrue || condition.Outcome == ssaflow.OutcomeFalse
	if load, ok := value.(*ssa.UnOp); ok && load.Op == token.MUL && boolean {
		if resolved := heapmodel.NewStorage(search.budget).Resolve(value); resolved.Proven() {
			value = resolved.Value
		}
	}
	if holds, known := ssaflow.OutcomeOf(condition.Outcome, value); known && !holds {
		return true, false
	}
	if completed {
		return true, true
	}
	call, index, ok := ssaflow.CallResultSource(value)
	if !ok {
		return false, true
	}
	// A forwarding return composes the same result predicate with the nested
	// callee. Ordinary calls above were searched without this predicate.
	nested := *search
	nested.condition = ssaflow.CallCondition{Result: index, Outcome: condition.Outcome}
	for _, local := range locals {
		if local.kind == localExact {
			if nested.completes(call, local.local).proven {
				return true, true
			}
		}
	}
	return false, true
}
