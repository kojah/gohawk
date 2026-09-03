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
	queue := initial
	expanded := map[K]bool{}
	for len(queue) > 0 {
		state := queue[0]
		queue = queue[1:]
		identity := key(state)
		if expanded[identity] {
			continue
		}
		expanded[identity] = true
		successors, ok := step(state)
		if !ok {
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
	index := InstructionIndex(start)
	if index < 0 {
		return nil
	}
	type location struct {
		block *ssa.BasicBlock
		index int
	}
	var result []ssa.Instruction
	WalkStates([]location{{block: start.Block(), index: index + 1}}, func(at location) location { return at }, func(at location) ([]location, bool) {
		result = append(result, at.block.Instrs[at.index:]...)
		successors := make([]location, 0, len(at.block.Succs))
		for _, successor := range at.block.Succs {
			if successor.Dominates(at.block) {
				continue
			}
			successors = append(successors, location{block: successor})
		}
		return successors, true
	})
	return result
}
