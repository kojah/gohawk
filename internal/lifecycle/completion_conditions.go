package lifecycle

import (
	"go/constant"
	"go/token"

	"github.com/kojah/gohawk/internal/heapmodel"
	proofs "github.com/kojah/gohawk/internal/proof"
	"github.com/kojah/gohawk/internal/ssaflow"
	ssacall "github.com/kojah/gohawk/internal/ssaflow/calls"
	cfg "github.com/kojah/gohawk/internal/ssaflow/cfg"
	ssapath "github.com/kojah/gohawk/internal/ssaflow/path"
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
func ProveCompletionOnEdge(from, to *ssa.BasicBlock, request CompletionRequest) proofs.CompletionProof {
	call, condition, ok := completionEdgeCondition(from, to)
	if !ok {
		return proofs.CompletionProof{Proof: proofs.Proof{State: proofs.EvidenceUnknown, Reason: proofs.EvidenceUnavailable}}
	}
	request.Instruction, request.condition = call, condition
	request.ExactTarget, request.Coverage = true, CoverageEveryReturn
	return ProveCompletion(request)
}

func completionEdgeCondition(from, to *ssa.BasicBlock) (*ssa.Call, ssacall.CallCondition, bool) {
	if from == nil || len(from.Instrs) == 0 || len(from.Succs) != 2 || from.Succs[0] == from.Succs[1] {
		return nil, ssacall.CallCondition{}, false
	}
	branch, ok := from.Instrs[len(from.Instrs)-1].(*ssa.If)
	if !ok || to != from.Succs[0] && to != from.Succs[1] {
		return nil, ssacall.CallCondition{}, false
	}
	value, outcome := completionTest(branch.Cond, to == from.Succs[0])
	call, index, ok := ssacall.CallResultSource(value)
	if !ok || outcome == ssacall.OutcomeAny || !cfg.InstructionDominates(call, branch) {
		return nil, ssacall.CallCondition{}, false
	}
	return call, ssacall.CallCondition{Result: index, Outcome: outcome}, true
}

func completionTest(value ssa.Value, truth bool) (ssa.Value, ssacall.Outcome) {
	value, negated := ssaflow.BooleanNegationSource(value)
	truth = truth != negated
	if comparison, ok := value.(*ssa.BinOp); ok {
		return completionComparison(comparison, truth)
	}
	if truth {
		return value, ssacall.OutcomeTrue
	}
	return value, ssacall.OutcomeFalse
}

func completionComparison(comparison *ssa.BinOp, truth bool) (ssa.Value, ssacall.Outcome) {
	if comparison.Op != token.EQL && comparison.Op != token.NEQ {
		return nil, ssacall.OutcomeAny
	}
	operand, literal := comparison.X, comparison.Y
	if _, ok := literal.(*ssa.Const); !ok {
		operand, literal = literal, operand
	}
	c, ok := literal.(*ssa.Const)
	if !ok {
		return nil, ssacall.OutcomeAny
	}
	equal := truth == (comparison.Op == token.EQL)
	if c.IsNil() && syntax.IsErrorType(operand.Type()) {
		if equal {
			return operand, ssacall.OutcomeNil
		}
		return operand, ssacall.OutcomeNonNil
	}
	if c.Value == nil || c.Value.Kind() != constant.Bool {
		return nil, ssacall.OutcomeAny
	}
	return completionTest(operand, equal == constant.BoolVal(c.Value))
}

type conditionalCompletionState struct {
	block       *ssa.BasicBlock
	predecessor *ssa.BasicBlock
	completed   bool
}

func (search *completionSearch) conditionalCoverage(
	function *ssa.Function, locals []mappedLocal, target ssa.Value, condition ssacall.CallCondition,
) bool {
	matched, failed := false, false
	initial := []conditionalCompletionState{{block: function.Blocks[0]}}
	cfg.WalkStatesWithin(initial, func(state conditionalCompletionState) conditionalCompletionState { return state },
		func(state conditionalCompletionState) ([]conditionalCompletionState, bool) {
			for _, instruction := range state.block.Instrs {
				if !search.budget.Spend() {
					failed = true
					return nil, false
				}
				state.completed = state.completed || search.instructionCompletes(instruction, locals, target)
				if ssapath.InstructionTerminatesControlFlow(instruction) {
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
			successors := ssapath.SuccessorPolicy{Constants: search.constants}.SuccessorsWithin(state.block, state.predecessor, search.budget)
			for _, successor := range successors {
				if !search.budget.Spend() {
					failed = true
					return nil, false
				}
				next = append(next, conditionalCompletionState{block: successor, predecessor: state.block, completed: state.completed})
			}
			return next, true
		}, search.budget)
	return matched && !failed && !search.budget.Exhausted()
}

func (search *completionSearch) conditionalReturn(
	returned *ssa.Return, locals []mappedLocal, condition ssacall.CallCondition, completed bool,
) (covered, relevant bool) {
	if condition.Result >= len(returned.Results) {
		return false, true
	}
	value := returned.Results[condition.Result]
	// Resolve compiler-spilled Boolean results, but do not peel an error's
	// interface box: a typed nil error is not the nil error result.
	boolean := condition.Outcome == ssacall.OutcomeTrue || condition.Outcome == ssacall.OutcomeFalse
	if load, ok := value.(*ssa.UnOp); ok && load.Op == token.MUL && boolean {
		if resolved := heapmodel.NewStorage(search.budget).Resolve(value); resolved.Proven() {
			value = resolved.Value
		}
	}
	if holds, known := ssacall.OutcomeOf(condition.Outcome, value); known && !holds {
		return true, false
	}
	if completed {
		return true, true
	}
	call, index, ok := ssacall.CallResultSource(value)
	if !ok {
		return false, true
	}
	// A forwarding return composes the same result predicate with the nested
	// callee. Ordinary calls above were searched without this predicate.
	nested := *search
	nested.condition = ssacall.CallCondition{Result: index, Outcome: condition.Outcome}
	for _, local := range locals {
		if local.kind == localExact {
			if nested.completes(call, local.local).proven {
				return true, true
			}
		}
	}
	return false, true
}
