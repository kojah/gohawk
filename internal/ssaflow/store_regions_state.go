package ssaflow

import (
	"maps"
	"slices"

	"golang.org/x/tools/go/ssa"
)

// The memory state at one program point, and how two states meet at a
// join. The lattice is deliberately small: known contents, backing copies,
// stamps for what the function did not write, clobbers, escapes, and what
// deferred calls were handed.

// regionState is the memory state at one program point.
type regionState struct {
	// contents holds the known pointees of slots.
	contents map[slot]pointees
	// backing records that a slot was filled by copying a snapshot whole,
	// so its sub-slots without their own entry are the snapshot's.
	backing map[slot]*region
	// epoch is the stamp of the last call or write through an unknown
	// pointer, which may have changed any object the function did not
	// allocate; stepEpochs stamps the last write through a foreign pointer
	// to each field or element step. An unwritten foreign slot read before
	// and after such a write is two objects.
	epoch      int
	stepEpochs map[string]int
	// clobbered marks slots whose subtree an unfollowed effect may have
	// changed, with the stamp of that effect; a read beneath one is the
	// content of an object the function did not write, and an exact entry
	// written afterwards still overrides it.
	clobbered map[slot]int
	// escaped marks sites whose address left local control.
	escaped map[*region]bool
	// escapes records how each slot left local control, for the heap
	// projection: any object, not only a site, and every way it went. A
	// slot beneath an object is the address of that field or element; its
	// escape hands on what lies beneath it, not the object above it.
	escapes map[slot]HeapEscape
	// opaque records that a call the graph could not resolve or summarize
	// may have written anything the function did not allocate; the heap
	// projection is then truncated at every root.
	opaque bool
	// deferred holds what deferred calls were handed; their effects apply
	// when the deferred calls run, at the function's RunDefers. calls lists
	// the deferred calls themselves, so one with a summary is applied then
	// and only an unresolved one forgets what it was handed.
	deferred pointees
	calls    []*ssa.Defer
}

func newRegionState() *regionState {
	return &regionState{
		contents:   map[slot]pointees{},
		backing:    map[slot]*region{},
		stepEpochs: map[string]int{},
		clobbered:  map[slot]int{},
		escaped:    map[*region]bool{},
		escapes:    map[slot]HeapEscape{},
		deferred:   pointees{},
	}
}

// stampOf returns the stamp a placeholder for the slot carries now.
func (state *regionState) stampOf(target slot) versionStamp {
	return versionStamp{epoch: state.epoch, step: state.stepEpochs[stepKey(target.path)]}
}

// stepKey names the last step of a path for stamping; an object's own slot
// is a step of its own, distinct from the epoch every call advances.
func stepKey(path string) string {
	if step := lastStep(path); step != "" {
		return step
	}
	return "self"
}

func (state *regionState) clone() *regionState {
	result := &regionState{
		contents:   make(map[slot]pointees, len(state.contents)),
		backing:    make(map[slot]*region, len(state.backing)),
		epoch:      state.epoch,
		stepEpochs: make(map[string]int, len(state.stepEpochs)),
		clobbered:  make(map[slot]int, len(state.clobbered)),
		escaped:    make(map[*region]bool, len(state.escaped)),
		escapes:    make(map[slot]HeapEscape, len(state.escapes)),
		opaque:     state.opaque,
		deferred:   state.deferred.clone(),
		calls:      slices.Clone(state.calls),
	}
	maps.Copy(result.escapes, state.escapes)
	for target, set := range state.contents {
		result.contents[target] = set.clone()
	}
	maps.Copy(result.backing, state.backing)
	maps.Copy(result.stepEpochs, state.stepEpochs)
	maps.Copy(result.clobbered, state.clobbered)
	maps.Copy(result.escaped, state.escaped)
	return result
}

// merge folds another state into this one at a join. Contents union, and a
// slot only one side wrote takes the other side's implicit content too, so
// a field written on one branch is not known after the join. Entries from a
// back edge are marked stale when their object was created inside the loop.
// A slot backed by different snapshots on two paths is clobbered; stamps
// that disagree take the join block's identity; clobbers and escapes
// accumulate. The fixpoint compares whole states, so nothing here reports
// what changed.
func (graph *regionGraph) merge(state, other *regionState, backEdge bool, header *ssa.BasicBlock) {
	graph.mergeContents(state, other, backEdge, header)
	graph.mergeBacking(state, other, header)
	graph.mergeStamps(state, other, header)
	maps.Copy(state.escaped, other.escaped)
	for target, kinds := range other.escapes {
		state.escapes[target] |= kinds
	}
	state.opaque = state.opaque || other.opaque
	state.deferred.union(other.deferred)
	for _, call := range other.calls {
		if !slices.Contains(state.calls, call) {
			state.calls = append(state.calls, call)
		}
	}
}

func (graph *regionGraph) mergeContents(state, other *regionState, backEdge bool, header *ssa.BasicBlock) {
	before := state.clone()
	stale := func(pointee slot, stale bool) bool {
		return stale || backEdge && createdInsideLoop(pointee.region, header)
	}
	for target, set := range other.contents {
		mine, present := state.contents[target]
		if !present {
			mine = graph.content(before, target).clone()
			state.contents[target] = mine
		}
		for pointee, pointeeStale := range set {
			mine.add(pointee, stale(pointee, pointeeStale))
		}
		graph.bound(state, target, nil)
	}
	for target, mine := range state.contents {
		if _, present := other.contents[target]; !present {
			for pointee, pointeeStale := range graph.content(other, target) {
				mine.add(pointee, stale(pointee, pointeeStale))
			}
			graph.bound(state, target, nil)
		}
	}
}

func (graph *regionGraph) mergeBacking(state, other *regionState, header *ssa.BasicBlock) {
	for target, snapshot := range other.backing {
		mine, present := state.backing[target]
		switch {
		case !present:
			state.backing[target] = snapshot
		case mine != snapshot:
			delete(state.backing, target)
			if _, ok := state.clobbered[target]; !ok {
				state.clobbered[target] = graph.blockID(header)
			}
		}
	}
	for target, stamp := range other.clobbered {
		if mine, ok := state.clobbered[target]; ok && mine != stamp {
			stamp = graph.blockID(header)
		}
		state.clobbered[target] = stamp
	}
}

func (graph *regionGraph) mergeStamps(state, other *regionState, header *ssa.BasicBlock) {
	if state.epoch != other.epoch {
		state.epoch = graph.blockID(header)
	}
	for step, stamp := range other.stepEpochs {
		if state.stepEpochs[step] != stamp {
			state.stepEpochs[step] = graph.blockID(header)
		}
	}
	for step := range state.stepEpochs {
		if _, present := other.stepEpochs[step]; !present {
			state.stepEpochs[step] = graph.blockID(header)
		}
	}
}

// createdInsideLoop reports whether the object was created by an instruction
// that the loop header does not dominate, so a pointer to it carried around
// the back edge denotes an earlier iteration's object.
func createdInsideLoop(object *region, header *ssa.BasicBlock) bool {
	switch object.kind {
	case regionNil, regionUnknown, regionExternal:
		return false
	case regionPlaceholder:
		return createdInsideLoop(object.source.region, header)
	case regionSite, regionOpaque, regionSnapshot, regionClosure:
	}
	instruction, ok := object.origin.(ssa.Instruction)
	if !ok || instruction.Block() == nil {
		return false
	}
	return !instruction.Block().Dominates(header) || instruction.Block() == header
}

func (state *regionState) equal(other *regionState) bool {
	return state.epoch == other.epoch &&
		pointeesMapsEqual(state.contents, other.contents) &&
		maps.Equal(state.backing, other.backing) &&
		maps.Equal(state.stepEpochs, other.stepEpochs) &&
		maps.Equal(state.clobbered, other.clobbered) &&
		maps.Equal(state.escaped, other.escaped) &&
		maps.Equal(state.escapes, other.escapes) &&
		state.opaque == other.opaque &&
		maps.Equal(state.deferred, other.deferred) &&
		slices.Equal(state.calls, other.calls)
}

func pointeesMapsEqual(left, right map[slot]pointees) bool {
	if len(left) != len(right) {
		return false
	}
	for target, set := range left {
		theirs, ok := right[target]
		if !ok || !maps.Equal(set, theirs) {
			return false
		}
	}
	return true
}
