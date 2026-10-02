package heapmodel

import "golang.org/x/tools/go/ssa"

// Containment queries select historical or point-in-time slot contents while
// holding the graph lock. Their shared region search preserves the same depth,
// cycle and unknown-pointee boundaries; it supplies possible containment only.

// contains reports whether the target's object is reachable from the
// owner's objects through what their slots ever held, at any depth the
// bound allows. It is a may-answer over the whole build under the
// structural contract: a value the graph has no pointees for, such as a
// scalar or a call's result tuple, contains nothing and is contained by
// nothing, as the value walk already says.
func (graph *regionGraph) contains(owner, value ssa.Value) bool {
	defer graph.lock()()
	from, ok := graph.pointsToUnlocked(owner)
	if !ok {
		return false
	}
	target, ok := graph.pointsToUnlocked(value)
	if !ok {
		return false
	}
	return containsThroughSlots(graph.history, from, target)
}

// containsAt reports whether the target's object is reachable from the
// owner's objects through the slot contents known when the instruction
// runs. Unlike contains, it does not see what the owner is given later, in
// particular not by the instruction itself: an argument handed to a call
// that stores it does not already contain what the call stores.
func (graph *regionGraph) containsAt(owner, value ssa.Value, at ssa.Instruction) (bool, bool) {
	defer graph.lock()()
	state := graph.stateAt(at)
	if state == nil {
		return false, false
	}
	from, ok := graph.pointsToUnlocked(owner)
	if !ok {
		return false, true
	}
	target, ok := graph.pointsToUnlocked(value)
	if !ok {
		return false, true
	}
	return containsThroughSlots(state.contents, from, target), true
}

// containsThroughSlots follows the selected slot map. History and observation
// must never be mixed: a later store is possible ownership over the whole build,
// but cannot establish containment before the instruction that performs it.
func containsThroughSlots(contents map[slot]pointees, from, target pointees) bool {
	seen := map[*region]bool{}
	queue := make([]*region, 0, len(from))
	for candidate := range from {
		queue = append(queue, candidate.region)
	}
	for depth := 0; len(queue) > 0 && depth < aliasDepth; depth++ {
		var next []*region
		for _, object := range queue {
			if seen[object] {
				continue
			}
			seen[object] = true
			for held, set := range contents {
				if held.region != object {
					continue
				}
				for pointee := range set {
					if _, wanted := target[pointee]; wanted || pointee.region.kind == regionUnknown {
						return true
					}
					next = append(next, pointee.region)
				}
			}
		}
		queue = next
	}
	return false
}
