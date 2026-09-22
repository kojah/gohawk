package ssaflow

import (
	"go/constant"
	"go/token"

	"github.com/kojah/gohawk/internal/syntax"

	"golang.org/x/tools/go/ssa"
)

// Conditional completion preserves a narrow relation between a synchronous
// call's result and its cleanup effect. The predicate belongs to that call's
// normal returns, never to every nested helper. Only explicit Boolean and
// error-nil tests are accepted; unresolved results and dispatch stay opaque.
type completionCondition struct {
	result int
	kind   completionConditionKind
}

type completionConditionKind uint8

const (
	completionUnconditional completionConditionKind = iota
	completionTrue
	completionFalse
	completionNil
	completionNonNil
)

// ProveCompletionOnEdge asks whether the call result tested by from establishes
// completion of request.Target on the edge to to. Instruction is derived from
// the test; only synchronous calls and exact target mappings are eligible.
// The result says nothing about the opposite edge or an untested call.
func ProveCompletionOnEdge(from, to *ssa.BasicBlock, request CompletionRequest) CompletionProof {
	call, condition, ok := completionEdgeCondition(from, to)
	if !ok {
		return CompletionProof{Proof{State: EvidenceUnknown, Reason: EvidenceUnavailable}}
	}
	request.Instruction, request.condition = call, condition
	request.ExactTarget, request.Coverage = true, CoverageEveryReturn
	return ProveCompletion(request)
}

func completionEdgeCondition(from, to *ssa.BasicBlock) (*ssa.Call, completionCondition, bool) {
	if from == nil || len(from.Instrs) == 0 || len(from.Succs) != 2 || from.Succs[0] == from.Succs[1] {
		return nil, completionCondition{}, false
	}
	branch, ok := from.Instrs[len(from.Instrs)-1].(*ssa.If)
	if !ok || to != from.Succs[0] && to != from.Succs[1] {
		return nil, completionCondition{}, false
	}
	value, kind := completionTest(branch.Cond, to == from.Succs[0])
	call, index, ok := completionResultCall(value)
	if !ok || kind == completionUnconditional || !InstructionDominates(call, branch) {
		return nil, completionCondition{}, false
	}
	return call, completionCondition{result: index, kind: kind}, true
}

func completionTest(value ssa.Value, truth bool) (ssa.Value, completionConditionKind) {
	if not, ok := value.(*ssa.UnOp); ok && not.Op == token.NOT {
		return completionTest(not.X, !truth)
	}
	if comparison, ok := value.(*ssa.BinOp); ok {
		return completionComparison(comparison, truth)
	}
	if truth {
		return value, completionTrue
	}
	return value, completionFalse
}

func completionComparison(comparison *ssa.BinOp, truth bool) (ssa.Value, completionConditionKind) {
	if comparison.Op != token.EQL && comparison.Op != token.NEQ {
		return nil, completionUnconditional
	}
	operand, literal := comparison.X, comparison.Y
	if _, ok := literal.(*ssa.Const); !ok {
		operand, literal = literal, operand
	}
	c, ok := literal.(*ssa.Const)
	if !ok {
		return nil, completionUnconditional
	}
	equal := truth == (comparison.Op == token.EQL)
	if c.IsNil() && syntax.IsErrorType(operand.Type()) {
		if equal {
			return operand, completionNil
		}
		return operand, completionNonNil
	}
	if c.Value == nil || c.Value.Kind() != constant.Bool {
		return nil, completionUnconditional
	}
	return completionTest(operand, equal == constant.BoolVal(c.Value))
}

func completionResultCall(value ssa.Value) (*ssa.Call, int, bool) {
	if extract, ok := value.(*ssa.Extract); ok {
		call, ok := extract.Tuple.(*ssa.Call)
		return call, extract.Index, ok
	}
	call, ok := value.(*ssa.Call)
	return call, 0, ok
}

func (condition completionCondition) matches(value ssa.Value) (matches, known bool) {
	if _, boxed := value.(*ssa.MakeInterface); boxed && (condition.kind == completionNil || condition.kind == completionNonNil) {
		// Even a boxed nil pointer has a dynamic type and is not a nil interface.
		return condition.kind == completionNonNil, true
	}
	c, ok := value.(*ssa.Const)
	if !ok {
		return false, false
	}
	switch condition.kind {
	case completionTrue, completionFalse:
		if c.Value != nil && c.Value.Kind() == constant.Bool {
			return constant.BoolVal(c.Value) == (condition.kind == completionTrue), true
		}
	case completionNil, completionNonNil:
		if c.IsNil() {
			return condition.kind == completionNil, true
		}
	case completionUnconditional:
	}
	return false, false
}

type conditionalCompletionState struct {
	block       *ssa.BasicBlock
	predecessor *ssa.BasicBlock
	completed   bool
}

func (search *completionSearch) conditionalCoverage(
	function *ssa.Function, locals []mappedLocal, target ssa.Value, condition completionCondition,
) bool {
	matched, failed := false, false
	initial := []conditionalCompletionState{{block: function.Blocks[0]}}
	WalkStates(initial, func(state conditionalCompletionState) conditionalCompletionState { return state },
		func(state conditionalCompletionState) ([]conditionalCompletionState, bool) {
			for _, instruction := range state.block.Instrs {
				if !search.budget.Spend() {
					failed = true
					return nil, false
				}
				state.completed = state.completed || search.instructionCompletes(instruction, locals, target)
				if InstructionTerminatesControlFlow(instruction) {
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
			for _, successor := range FeasibleSuccessors(state.block, state.predecessor) {
				next = append(next, conditionalCompletionState{block: successor, predecessor: state.block, completed: state.completed})
			}
			return next, true
		})
	return matched && !failed && !search.budget.Exhausted()
}

func (search *completionSearch) conditionalReturn(
	returned *ssa.Return, locals []mappedLocal, condition completionCondition, completed bool,
) (covered, relevant bool) {
	if condition.result >= len(returned.Results) {
		return false, true
	}
	value := returned.Results[condition.result]
	// Resolve compiler-spilled Boolean results, but do not peel an error's
	// interface box: a typed nil error is not the nil error result.
	boolean := condition.kind == completionTrue || condition.kind == completionFalse
	if load, ok := value.(*ssa.UnOp); ok && load.Op == token.MUL && boolean {
		if resolved := NewStorage(search.budget).Resolve(value); resolved.Proven() {
			value = resolved.Value
		}
	}
	if matches, known := condition.matches(value); known && !matches {
		return true, false
	}
	if completed {
		return true, true
	}
	call, index, ok := completionResultCall(value)
	if !ok {
		return false, true
	}
	// A forwarding return composes the same result predicate with the nested
	// callee. Ordinary calls above were searched without this predicate.
	nested := *search
	nested.condition = completionCondition{result: index, kind: condition.kind}
	for _, local := range locals {
		if local.kind == localExact {
			if _, proven, _ := nested.completes(call, local.local); proven {
				return true, true
			}
		}
	}
	return false, true
}
