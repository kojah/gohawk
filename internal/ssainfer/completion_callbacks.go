package ssainfer

import (
	"github.com/kojah/gohawk/internal/ssaflow"
	"golang.org/x/tools/go/ssa"
)

// Callback completion asks whether a helper invokes a function argument on
// every normal return, the shape a deferred cleanup helper takes.

func CallInvokesArgumentOnEveryReturn(instruction ssa.Instruction, target ssa.Value) bool {
	search := &callbackSearch{memo: ssaflow.NewCallGraphMemo[callbackKey, bool]()}
	return search.invokes(instruction, target)
}

// SpawnInvokesArgumentOnEveryReturn reports whether the function launched by
// spawn invokes target synchronously before every normal return. The spawn is
// asynchronous to its caller, but calls made inside its wrapper must not be.
func SpawnInvokesArgumentOnEveryReturn(spawn *ssa.Go, target ssa.Value) bool {
	search := &callbackSearch{memo: ssaflow.NewCallGraphMemo[callbackKey, bool]()}
	// The instruction itself may be a go statement whose callee is the
	// synchronous wrapper under examination. Calls made by that wrapper still
	// have to be synchronous: handing the callback to another goroutine does
	// not make it run before the wrapper returns.
	return search.searchInvokes(spawn, target)
}

// callbackSearch answers one callback-completion question. The memo owns the
// cycle guard and the rule that an answer cut short by it is not retained.
type callbackSearch struct {
	memo *ssaflow.CallGraphMemo[callbackKey, bool]
}

type callbackKey struct {
	instruction ssa.Instruction
	target      ssa.Value
}

func (search *callbackSearch) invokes(instruction ssa.Instruction, target ssa.Value) bool {
	if _, asynchronous := instruction.(*ssa.Go); asynchronous {
		return false
	}
	return search.searchInvokes(instruction, target)
}

func (search *callbackSearch) searchInvokes(instruction ssa.Instruction, target ssa.Value) bool {
	common := ssaflow.InstructionCall(instruction)
	if common == nil || common.StaticCallee() == nil {
		return false
	}
	key := callbackKey{instruction: instruction, target: target}
	return search.memo.Summarize(key, common.StaticCallee(), nil, func() bool {
		return callOwnsArgumentOnEveryReturn(instruction, target, func(candidate ssa.Instruction, parameter ssa.Value) bool {
			if _, asynchronous := candidate.(*ssa.Go); asynchronous {
				return false
			}
			common := ssaflow.InstructionCall(candidate)
			return common != nil && NewStorage(nil).Same(common.Value, parameter).Proven() || search.invokes(candidate, parameter)
		})
	}, func(ssaflow.SummaryUnavailable, bool) bool {
		return false
	})
}

func callOwnsArgumentOnEveryReturn(instruction ssa.Instruction, target ssa.Value, owns func(ssa.Instruction, ssa.Value) bool) bool {
	common := ssaflow.InstructionCall(instruction)
	if common == nil || common.StaticCallee() == nil {
		return false
	}
	callee := common.StaticCallee()
	if len(callee.Blocks) == 0 {
		return false
	}
	for _, binding := range ssaflow.CallBindings(common, callee, nil) {
		if !NewStorage(nil).Same(binding.Supplied, target).Proven() {
			continue
		}
		parameter := binding.Local
		calls := func(candidate ssa.Instruction) bool {
			return owns(candidate, parameter)
		}
		if MethodCallCoverage(callee, calls, CoverageEveryReturn, parameter) {
			return true
		}
	}
	return false
}
