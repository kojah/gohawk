package heapmodel

import (
	"maps"
	"slices"
	"strings"

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
	// epoch is the stamp of the last write through an unknown pointer,
	// which may have changed any object the function did not allocate;
	// reachEpoch is the stamp of the last such write or unresolved call,
	// which may have changed any object within its reach; stepEpochs
	// stamps the last write through a foreign pointer to each field or
	// element step. An unwritten slot read before and after an effect that
	// could reach it is two objects; one the effect could not reach is
	// read as the same object on both sides.
	epoch      int
	reachEpoch int
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
	// ran marks function values that code the graph cannot see invoked.
	// Running a callback is not keeping it, so it is no escape; but what
	// the callback captured may have been written, so the projection cuts
	// a root that ran.
	ran map[*region]bool
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
		ran:        map[*region]bool{},
		escapes:    map[slot]HeapEscape{},
		deferred:   pointees{},
	}
}

// stampOf returns the stamp a placeholder for the slot carries now: the
// last effect that could have reached the slot's object.
func (graph *regionGraph) stampOf(state *regionState, target slot) versionStamp {
	epoch := state.epoch
	if graph.foreign(state, target.region, reachEscaped) {
		epoch = state.reachEpoch
	}
	return versionStamp{epoch: epoch, step: state.stepEpochs[stepKey(target.path)]}
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
		backing:    cloneRegionMap(state.backing),
		epoch:      state.epoch,
		reachEpoch: state.reachEpoch,
		stepEpochs: cloneRegionMap(state.stepEpochs),
		clobbered:  cloneRegionMap(state.clobbered),
		escaped:    cloneRegionMap(state.escaped),
		ran:        maps.Clone(state.ran),
		escapes:    cloneRegionMap(state.escapes),
		opaque:     state.opaque,
		deferred:   cloneRegionMap(state.deferred),
		calls:      slices.Clone(state.calls),
	}
	for target, set := range state.contents {
		result.contents[target] = cloneRegionMap(set)
	}
	return result
}

// cloneRegionMap copies flat state maps through the runtime's map clone rather
// than hashing each entry again. A zero state still needs writable maps.
func cloneRegionMap[K comparable, V any](source map[K]V) map[K]V {
	if source == nil {
		return map[K]V{}
	}
	return maps.Clone(source)
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
	maps.Copy(state.ran, other.ran)
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
	// Missing slots can read backing copies or wildcard elements that this
	// same join updates. Preserve the original read order when those reads
	// are needed; otherwise the snapshot would never be used.
	var before *regionState
	for target := range other.contents {
		if _, present := state.contents[target]; !present {
			before = state.clone()
			break
		}
	}
	stale := func(pointee slot, stale bool) bool {
		return stale || backEdge && createdInsideLoop(pointee.region, header)
	}
	for target, set := range other.contents {
		mine, present := state.contents[target]
		if !present {
			mine = graph.content(before, target)
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
	if state.reachEpoch != other.reachEpoch {
		state.reachEpoch = graph.blockID(header)
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
	return state.epoch == other.epoch && state.reachEpoch == other.reachEpoch &&
		pointeesMapsEqual(state.contents, other.contents) &&
		maps.Equal(state.backing, other.backing) &&
		maps.Equal(state.stepEpochs, other.stepEpochs) &&
		maps.Equal(state.clobbered, other.clobbered) &&
		maps.Equal(state.escaped, other.escaped) &&
		maps.Equal(state.ran, other.ran) &&
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

// difference names the first thing that distinguishes the other state from
// this one, for the dump of a fixpoint that did not settle: a slot whose
// contents differ, then a stamp, an escape, or a flag.
func (state *regionState) difference(other *regionState) string {
	for _, target := range orderedSlots(other.contents) {
		mine, ok := state.contents[target]
		if !ok || !maps.Equal(mine, other.contents[target]) {
			return slotName(target) + " holds " + renderPointees(other.contents[target]) + ", held " + renderPointees(mine)
		}
	}
	for target := range state.contents {
		if _, ok := other.contents[target]; !ok {
			return slotName(target) + " forgotten"
		}
	}
	switch {
	case state.epoch != other.epoch:
		return "epoch"
	case state.reachEpoch != other.reachEpoch:
		return "reach epoch"
	case !maps.Equal(state.stepEpochs, other.stepEpochs):
		return "step epochs"
	case !maps.Equal(state.clobbered, other.clobbered):
		return "clobbers"
	case !maps.Equal(state.escapes, other.escapes):
		return "escapes"
	case !maps.Equal(state.ran, other.ran):
		return "callbacks run"
	case !maps.Equal(state.backing, other.backing):
		return "backing"
	case state.opaque != other.opaque:
		return "opacity"
	}
	return "deferred calls"
}

// renderPointees names a set's slots for the dump.
func renderPointees(set pointees) string {
	names := make([]string, 0, len(set))
	for _, target := range orderedSlots(set) {
		name := slotName(target)
		if set[target] {
			name += " (stale)"
		}
		names = append(names, name)
	}
	if len(names) == 0 {
		return "nothing"
	}
	return strings.Join(names, ", ")
}

// forgetStoredSubtree removes concrete contents and backing copies. It retains
// clobber stamps: forgetting after an opaque effect must not restore zero-value
// certainty. A full overwrite or allocation reset clears stamps separately.
func (state *regionState) forgetStoredSubtree(target slot) {
	for other := range state.contents {
		if other.region == target.region && slotBeneath(other.path, target.path) {
			delete(state.contents, other)
		}
	}
	for other := range state.backing {
		if other.region == target.region && slotBeneath(other.path, target.path) {
			delete(state.backing, other)
		}
	}
}
