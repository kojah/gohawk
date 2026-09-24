package lifecycle

import (
	"github.com/kojah/gohawk/internal/ssaflow"
	"golang.org/x/tools/go/ssa"
)

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
	// paths says where beneath the target a proven completion settled; see
	// completionPaths. It is meaningful only when proven.
	paths completionPaths
}

// completes reports whether the instruction's callees all call method on the
// target with the coverage their launch demands. The answer is unavailable
// when no callee body was available to search.
func (search *completionSearch) completes(instruction ssa.Instruction, target ssa.Value) completionAnswer {
	key := completionKey{
		instruction: instruction, target: target, invokeTarget: search.invokeTarget, bindings: search.bindings, condition: search.condition,
	}
	return search.memo.Compose(key, search.budget, func() completionAnswer {
		return search.searchCompletes(instruction, target)
	}, func(_ ssaflow.SummaryUnavailable, partial completionAnswer) completionAnswer {
		partial.proven = false
		return partial
	})
}

func (search *completionSearch) searchCompletes(instruction ssa.Instruction, target ssa.Value) completionAnswer {
	if kind, proven := search.returnedCallCompletes(instruction, target); proven {
		// A completion routed through a returned callback is proven about
		// the target as a whole; the search does not follow its path.
		return completionAnswer{launch: kind, proven: true, available: true}
	}
	if _, synchronous := instruction.(*ssa.Call); synchronous && search.callContract != nil &&
		search.callContract(instruction, target, search.method, search.invokeTarget, search.condition.predicate()) {
		return completionAnswer{launch: launchCalled, proven: true, available: true}
	}
	callees, ok := search.boundCallees(instruction)
	if !ok || len(callees) == 0 {
		return completionAnswer{launch: launchNone}
	}
	searched := false
	var paths completionPaths
	for _, callee := range callees {
		callee.invocation = instruction
		if callee.environment == nil {
			callee.environment = search.bindings
		}
		if callee.function == nil || len(callee.function.Blocks) == 0 {
			if _, synchronous := instruction.(*ssa.Call); synchronous && search.summarized != nil &&
				search.summarized(instruction, target, search.method, search.invokeTarget, search.condition.predicate()) {
				// A summary settles the target as a whole; where it did so
				// beneath the target is not part of its claim.
				paths.record("", false)
				continue
			}
			return completionAnswer{launch: callee.launch, available: searched}
		}
		answer := search.calleeCompletes(callee, target, instruction)
		if !answer.available {
			return completionAnswer{launch: callee.launch, available: searched}
		}
		searched = true
		if !answer.proven {
			return completionAnswer{launch: callee.launch, available: true}
		}
		paths.merge(answer.paths)
	}
	return completionAnswer{launch: callees[0].launch, proven: true, available: true, paths: paths}
}

func (search *completionSearch) calleeCompletes(callee completionCallee, target ssa.Value, invocation ssa.Instruction) completionAnswer {
	answer := completionAnswer{launch: callee.launch}
	answer.available = search.memo.WithFunction(callee.function, func() {
		// Each body's coverage records its own completing calls, so a
		// nested body's paths do not bleed into the enclosing answer
		// except through the mapping that translates them.
		previous := search.paths
		search.paths = &completionPaths{}
		answer.proven = search.calleeCoverage(callee, target, invocation)
		answer.paths = *search.paths
		search.paths = previous
	})
	return answer
}
