package ssaflow

import "golang.org/x/tools/go/ssa"

// Callback completion asks whether a helper invokes a function argument on
// every normal return, the shape a deferred cleanup helper takes.

func CallInvokesArgumentOnEveryReturn(instruction ssa.Instruction, target ssa.Value) bool {
	search := &callbackSearch{memo: NewCallGraphMemo[callbackKey, bool]()}
	return search.invokes(instruction, target)
}

// callbackSearch answers one callback-completion question. The memo owns the
// cycle guard and the rule that an answer cut short by it is not retained.
type callbackSearch struct {
	memo *CallGraphMemo[callbackKey, bool]
}

type callbackKey struct {
	instruction ssa.Instruction
	target      ssa.Value
}

func (search *callbackSearch) invokes(instruction ssa.Instruction, target ssa.Value) bool {
	return search.memo.Answer(callbackKey{instruction: instruction, target: target}, func() bool {
		return search.searchInvokes(instruction, target)
	})
}

func (search *callbackSearch) searchInvokes(instruction ssa.Instruction, target ssa.Value) bool {
	common := InstructionCall(instruction)
	if common == nil || common.StaticCallee() == nil {
		return false
	}
	callee := common.StaticCallee()
	if !search.memo.Enter(callee) {
		return false
	}
	defer search.memo.Leave(callee)
	return callOwnsArgumentOnEveryReturn(instruction, target, func(candidate ssa.Instruction, parameter ssa.Value) bool {
		common := InstructionCall(candidate)
		return common != nil && SameValue(common.Value, parameter) || search.invokes(candidate, parameter)
	})
}

func callOwnsArgumentOnEveryReturn(instruction ssa.Instruction, target ssa.Value, owns func(ssa.Instruction, ssa.Value) bool) bool {
	common := InstructionCall(instruction)
	if common == nil || common.StaticCallee() == nil {
		return false
	}
	callee := common.StaticCallee()
	if len(callee.Blocks) == 0 {
		return false
	}
	for index, argument := range common.Args {
		if index >= len(callee.Params) || !SameValue(argument, target) && !ValueContainsValue(argument, target) {
			continue
		}
		parameter := callee.Params[index]
		calls := func(candidate ssa.Instruction) bool {
			return owns(candidate, parameter)
		}
		if MethodCallCoverage(callee, calls, CoverageEveryReturn, parameter) {
			return true
		}
	}
	return false
}
