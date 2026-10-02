package ssaflow

import "golang.org/x/tools/go/ssa"

// WalkStates drives a keyed work list over path-sensitive states. The caller
// owns the state type and the transfer policy: step consumes one state and
// returns the successor states it produces, or false to end the walk early.
// The driver owns termination: a state is expanded only when its key has not
// been expanded before, which bounds the walk on loops while still letting a
// block be revisited under a different obligation state. The caller's key must
// therefore capture every part of the state that changes what step does.
func WalkStates[S any, K comparable](initial []S, key func(S) K, step func(S) ([]S, bool)) {
	WalkStatesWithin(initial, key, step, nil)
}

// WalkStatesWithin charges queued visits before key construction, including
// revisits. It stops if key or step exhausts the shared allowance, before
// admitting a partial key or successor list. Callers retain cutoff availability;
// a nil budget preserves default expansion and early-stop policy.
func WalkStatesWithin[S any, K comparable](initial []S, key func(S) K, step func(S) ([]S, bool), budget *SearchBudget) {
	queue := initial
	expanded := map[K]bool{}
	for len(queue) > 0 {
		if !budget.Spend() {
			return
		}
		state := queue[0]
		queue = queue[1:]
		identity := key(state)
		if budget.Exhausted() {
			return
		}
		if expanded[identity] {
			continue
		}
		expanded[identity] = true
		successors, ok := step(state)
		if !ok || budget.Exhausted() {
			return
		}
		queue = append(queue, successors...)
	}
}

// InstructionsReachableAfter returns every instruction that control can reach
// after start without crossing a loop back edge, in visiting order. Stopping
// at back edges keeps a loop-carried SSA value from being compared with a
// different runtime value it names on a later iteration, which matters for
// any use-after-X question.
func InstructionsReachableAfter(start ssa.Instruction) []ssa.Instruction {
	return InstructionsReachableAfterWithin(start, nil)
}

// InstructionsReachableAfterWithin charges the instruction and successor
// census to budget. A partial result is usable only with its availability:
// exhaustion never proves that an instruction cannot follow start.
func InstructionsReachableAfterWithin(start ssa.Instruction, budget *SearchBudget) []ssa.Instruction {
	if start == nil || start.Block() == nil {
		return nil
	}
	index := -1
	for i, instruction := range start.Block().Instrs {
		if !budget.Spend() {
			return nil
		}
		if instruction == start {
			index = i
			break
		}
	}
	if index < 0 {
		return nil
	}
	type location struct {
		block *ssa.BasicBlock
		index int
	}
	var result []ssa.Instruction
	WalkStates([]location{{block: start.Block(), index: index + 1}}, func(at location) location { return at }, func(at location) ([]location, bool) {
		for _, instruction := range at.block.Instrs[at.index:] {
			if !budget.Spend() {
				return nil, false
			}
			result = append(result, instruction)
		}
		successors := make([]location, 0, len(at.block.Succs))
		for _, successor := range at.block.Succs {
			if !budget.Spend() {
				return nil, false
			}
			if successor.Dominates(at.block) {
				continue
			}
			successors = append(successors, location{block: successor})
		}
		return successors, true
	})
	return result
}
