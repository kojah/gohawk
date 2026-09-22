package ssaflow

import "golang.org/x/tools/go/ssa"

// Completion summaries cache an invocation-specific question while guarding
// each possible callee independently. Missing or recursive bodies cannot prove
// completion; the summary driver owns cache invalidation after truncated work.

// completionKey identifies one completion question. The method and coverage
// are fixed for a search, but a callback search shares the parent's guards
// while answering a different question, so invokeTarget belongs in the key.
// Result conditions also belong in the key: true-edge completion must never
// answer a false-edge or unconditional question about the same invocation.
type completionKey struct {
	bindings     *callbackBindings
	instruction  ssa.Instruction
	target       ssa.Value
	invokeTarget bool
	condition    completionCondition
}

type completionAnswer struct {
	launch    launchKind
	proven    bool
	available bool
}

// completes reports whether the instruction's callees all call method on the
// target with the coverage their launch demands. The final result is false
// when no callee body was available to search.
func (search *completionSearch) completes(instruction ssa.Instruction, target ssa.Value) (launchKind, bool, bool) {
	key := completionKey{
		instruction: instruction, target: target, invokeTarget: search.invokeTarget, bindings: search.bindings, condition: search.condition,
	}
	answer := search.memo.Compose(key, search.budget, func() completionAnswer {
		launch, proven, available := search.searchCompletes(instruction, target)
		return completionAnswer{launch: launch, proven: proven, available: available}
	}, func(_ SummaryUnavailable, partial completionAnswer) completionAnswer {
		partial.proven = false
		return partial
	})
	return answer.launch, answer.proven, answer.available
}

func (search *completionSearch) searchCompletes(instruction ssa.Instruction, target ssa.Value) (launchKind, bool, bool) {
	callees, ok := search.boundCallees(instruction)
	if !ok || len(callees) == 0 {
		return launchNone, false, false
	}
	searched := false
	for _, callee := range callees {
		callee.invocation = instruction
		if callee.environment == nil {
			callee.environment = search.bindings
		}
		if callee.function == nil || len(callee.function.Blocks) == 0 {
			return callee.launch, false, searched
		}
		answer := search.calleeCompletes(callee, target, instruction)
		if !answer.available {
			return callee.launch, false, searched
		}
		searched = true
		if !answer.proven {
			return callee.launch, false, true
		}
	}
	return callees[0].launch, true, true
}

func (search *completionSearch) calleeCompletes(callee completionCallee, target ssa.Value, invocation ssa.Instruction) completionAnswer {
	answer := completionAnswer{launch: callee.launch}
	answer.available = search.memo.WithFunction(callee.function, func() {
		answer.proven = search.calleeCoverage(callee, target, invocation)
	})
	return answer
}
