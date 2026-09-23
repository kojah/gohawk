package ssaflow

import (
	"maps"

	"golang.org/x/tools/go/ssa"
)

// Slot contents: what a location holds at a point, how an aggregate is
// copied whole, and how a store settles into the state. These rules decide
// the must-answers, so each one says where it stops being exact.

// content returns what one slot holds in the state: its own entry, a
// wildcard element entry beneath the same array, the backing snapshot's
// entry, nil for an unwritten local slot, or a placeholder for an unwritten
// slot of an object the function did not allocate. A clobbered prefix makes
// the answer unknown unless the slot itself was written since.
func (graph *regionGraph) content(state *regionState, target slot) pointees {
	if target.region.kind == regionUnknown {
		return pointees{target: false}
	}
	if target.region.kind == regionNil {
		return pointees{{region: graph.nilR}: false}
	}
	result := pointees{}
	if set, ok := state.contents[target]; ok {
		result.union(set)
	}
	if isIndexStep(lastStep(target.path)) {
		if lastStep(target.path) == pathStar {
			for other, set := range state.contents {
				if other.region == target.region && parentPath(other.path) == parentPath(target.path) && isIndexStep(lastStep(other.path)) {
					result.union(set)
				}
			}
			// A dynamic read may also hit an element nobody wrote, which
			// the unwritten answer below describes.
			result.union(graph.unwritten(state, target))
			return result
		}
		if set, ok := state.contents[graph.starSlot(target)]; ok {
			result.union(set)
		}
	}
	if len(result) > 0 {
		return result
	}
	return graph.unwritten(state, target)
}

// unwritten returns what a slot the function did not write holds: after an
// effect the graph could not follow, an object stamped by that effect; the
// backing snapshot's content; nil for an untouched local; a placeholder for
// an object the function did not allocate.
func (graph *regionGraph) unwritten(state *regionState, target slot) pointees {
	if stamp, ok := graph.clobberedBeneath(state, target); ok {
		return pointees{{region: graph.placeholder(target, versionStamp{epoch: stamp})}: false}
	}
	if backing, rest, ok := graph.backingOf(state, target); ok {
		return graph.content(state, slot{region: backing, path: rest})
	}
	switch target.region.kind {
	case regionSite:
		return pointees{{region: graph.nilR}: false}
	case regionSnapshot:
		source := target.region.source
		switch {
		case source.region == nil:
			return pointees{{region: graph.unkR}: false}
		case source.region.kind == regionSnapshot:
			return graph.content(state, slot{region: source.region, path: joinSlotPath(source.path, target.path)})
		case source.region.kind == regionSite:
			return pointees{{region: graph.nilR}: false}
		}
		origin := slot{region: source.region, path: joinSlotPath(source.path, target.path)}
		stamp := versionStamp{epoch: target.region.stamp.epoch, step: target.region.stamps[stepKey(origin.path)]}
		return pointees{{region: graph.placeholder(origin, stamp)}: false}
	case regionExternal, regionOpaque, regionPlaceholder, regionClosure:
		return pointees{{region: graph.placeholder(target, state.stampOf(target))}: false}
	case regionNil, regionUnknown:
	}
	return pointees{{region: graph.unkR}: false}
}

func (graph *regionGraph) starSlot(target slot) slot {
	return slot{region: target.region, path: joinSlotPath(parentPath(target.path), pathStar)}
}

func parentPath(path string) string {
	if index := len(path) - len(lastStep(path)) - 1; index > 0 {
		return path[:index]
	}
	return ""
}

// clobberedBeneath returns the stamp of the clobber that reaches the slot:
// the most specific clobbered prefix above it, and the latest stamp among
// clobbers of that prefix, so the placeholder a read produces does not
// depend on the order the clobbers are visited in.
func (graph *regionGraph) clobberedBeneath(state *regionState, target slot) (int, bool) {
	best, found := slot{}, false
	stamp := 0
	for prefix, candidate := range state.clobbered {
		if prefix.region != target.region || !slotBeneath(target.path, prefix.path) {
			continue
		}
		closer := len(prefix.path) > len(best.path) || len(prefix.path) == len(best.path) && candidate > stamp
		if !found || closer {
			best, stamp, found = prefix, candidate, true
		}
	}
	return stamp, found
}

// backingOf finds the nearest slot above target that a snapshot was copied
// into, and the path of target beneath it.
func (graph *regionGraph) backingOf(state *regionState, target slot) (*region, string, bool) {
	path := target.path
	for {
		if backing, ok := state.backing[slot{region: target.region, path: path}]; ok {
			return backing, target.path[len(path):], true
		}
		if path == "" {
			return nil, "", false
		}
		path = parentPath(path)
		if path == "" {
			if backing, ok := state.backing[slot{region: target.region}]; ok {
				rest := target.path
				return backing, rest, true
			}
			return nil, "", false
		}
	}
}

// snapshotOf copies an aggregate out of the addressed slots into a fresh
// snapshot region for the loaded value.
func (graph *regionGraph) snapshotOf(state *regionState, addresses pointees, loaded ssa.Value) pointees {
	snapshot := graph.snapshot(loaded)
	stamp := 0
	if instruction, ok := loaded.(ssa.Instruction); ok {
		stamp = graph.id(instruction)
	}
	if len(addresses) != 1 || addresses.unknown() {
		state.clobbered[slot{region: snapshot}] = stamp
		return pointees{{region: snapshot}: false}
	}
	for address, stale := range addresses {
		if lastStep(address.path) == pathStar {
			state.clobbered[slot{region: snapshot}] = stamp
			return pointees{{region: snapshot}: stale}
		}
		graph.copySubtree(state, address, slot{region: snapshot})
		snapshot.source = address
		snapshot.stamp = versionStamp{epoch: state.epoch}
		snapshot.stamps = maps.Clone(state.stepEpochs)
		if clobber, ok := graph.clobberedBeneath(state, address); ok {
			state.clobbered[slot{region: snapshot}] = clobber
		}
		return pointees{{region: snapshot}: stale}
	}
	return pointees{{region: snapshot}: false}
}

// copySubtree copies every entry beneath source to the same path beneath
// destination and carries the backing snapshot of source along.
func (graph *regionGraph) copySubtree(state *regionState, source, destination slot) {
	for target, set := range state.contents {
		if target.region != source.region || !slotBeneath(target.path, source.path) {
			continue
		}
		rest := target.path[len(source.path):]
		copied := slot{region: destination.region, path: joinSlotPath(destination.path, trimSlash(rest))}
		state.contents[copied] = set.clone()
		graph.remember(copied, set)
	}
	for target, backing := range state.backing {
		if target.region != source.region || !slotBeneath(target.path, source.path) {
			continue
		}
		rest := target.path[len(source.path):]
		state.backing[slot{region: destination.region, path: joinSlotPath(destination.path, trimSlash(rest))}] = backing
	}
	if backing, rest, ok := graph.backingOf(state, source); ok {
		if _, direct := state.backing[destination]; !direct {
			state.backing[destination] = graph.snapshotBeneath(backing, trimSlash(rest))
		}
	}
}

// snapshotBeneath names the sub-aggregate of a snapshot at path as a
// snapshot of its own, so a copy of part of a copy still resolves.
func (graph *regionGraph) snapshotBeneath(snapshot *region, path string) *region {
	if path == "" {
		return snapshot
	}
	nested := graph.intern(regionKey{kind: regionSnapshot, origin: snapshot.origin, source: slot{region: snapshot, path: path}})
	nested.source = slot{region: snapshot, path: path}
	return nested
}

func trimSlash(path string) string {
	if len(path) > 0 && path[0] == '/' {
		return path[1:]
	}
	return path
}

// escapeInto names how a value stored into the object leaves local
// control: through a global, or through an object the caller can reach.
func escapeInto(object *region) HeapEscape {
	if object.kind == regionExternal {
		if _, global := object.origin.(*ssa.Global); global {
			return HeapEscapedGlobal
		}
	}
	return HeapEscapedField
}

// forgetWholeAbove drops the whole-aggregate entries of the slots above a
// sub-slot about to be written: the aggregate no longer equals the value
// stored into it whole.
func (graph *regionGraph) forgetWholeAbove(state *regionState, target slot) {
	if target.path == "" {
		return
	}
	for path := parentPath(target.path); ; path = parentPath(path) {
		delete(state.contents, slot{region: target.region, path: path})
		if path == "" {
			return
		}
	}
}

// storeAggregate records the aggregate value as the slot's whole content
// and copies its sub-slots beneath the target.
func (graph *regionGraph) storeAggregate(state *regionState, target slot, value pointees, strong bool, stamp int) {
	if strong {
		graph.clearSubtree(state, target)
	}
	graph.forgetWholeAbove(state, target)
	if len(value) != 1 || value.unknown() {
		state.clobbered[target] = stamp
		return
	}
	for source := range value {
		if source.region.kind == regionNil {
			// Storing a zero value empties every sub-slot.
			if strong {
				return
			}
			state.clobbered[target] = stamp
			return
		}
		if !strong {
			state.clobbered[target] = stamp
			return
		}
		state.contents[target] = value.clone()
		graph.remember(target, value)
		graph.copySubtree(state, source, target)
		if _, direct := state.backing[target]; !direct {
			state.backing[target] = graph.snapshotBeneath(source.region, source.path)
		}
		if clobber, ok := graph.clobberedBeneath(state, source); ok {
			state.clobbered[target] = clobber
		}
	}
}

// remember records, for the whole build, that the slot was given the value.
func (graph *regionGraph) remember(target slot, value pointees) {
	held, ok := graph.history[target]
	if !ok {
		held = pointees{}
		graph.history[target] = held
	}
	held.union(value)
}

// weakElementStore unions the value into the wildcard element slot and
// every constant element beneath the same array.
func (graph *regionGraph) weakElementStore(state *regionState, target slot, value pointees) {
	graph.remember(target, value)
	star := state.contents[target]
	if star == nil {
		star = pointees{}
		state.contents[target] = star
	}
	star.union(value)
	graph.bound(state, target, nil)
	parent := parentPath(target.path)
	for other, set := range state.contents {
		if other.region == target.region && other.path != target.path && parentPath(other.path) == parent && isIndexStep(lastStep(other.path)) {
			set.union(value)
			graph.bound(state, other, nil)
		}
	}
}

// clearSubtree forgets everything beneath a slot before it is overwritten.
func (graph *regionGraph) clearSubtree(state *regionState, target slot) {
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
	for other := range state.clobbered {
		if other.region == target.region && slotBeneath(other.path, target.path) {
			delete(state.clobbered, other)
		}
	}
}
