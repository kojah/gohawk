package ssaflow

import "golang.org/x/tools/go/ssa"

// Queries read the graph. Each one states its polarity: a may-answer is
// true whenever the graph cannot rule the relation out, and a must-answer
// is true only when one non-stale slot proves it. An unavailable graph
// answers may with true and must with false.

// pointsTo returns the slots a value may refer to, and whether the graph
// could say.
func (graph *regionGraph) pointsTo(value ssa.Value) (pointees, bool) {
	if !graph.available || value == nil {
		return nil, false
	}
	if set, ok := graph.values[value]; ok {
		return set, true
	}
	set := graph.pointees(value)
	return set, len(set) > 0
}

// aliasProof decides whether two values may refer to the same object and
// names the rule. A value the graph does not track, such as a call's whole
// result tuple, keeps the structural answer: it is not unknown, it is not a
// pointer. A value with no pointees is unknown and may alias anything. A
// disjointness answer is recorded on the graph.
func (graph *regionGraph) aliasProof(left, right ssa.Value) AliasProof {
	if !tracked(left.Type()) || !tracked(right.Type()) {
		return AliasProof{Aliases: structurallySame(left, right), Reason: EvidenceStructuralWalk, Provenance: EvidenceFromLocalSSA}
	}
	a, okA := graph.pointsTo(left)
	b, okB := graph.pointsTo(right)
	if !okA || !okB {
		return AliasProof{Aliases: true, Reason: EvidenceUnknownPointee, Provenance: EvidenceFromLocalSSA}
	}
	reason := EvidenceDisjointObjects
	for x := range a {
		for y := range b {
			if graph.slotsMayAlias(x, y, aliasDepth) {
				return AliasProof{Aliases: true, Reason: EvidenceSharedSlot, Provenance: EvidenceFromLocalSSA}
			}
			switch {
			case x.region == y.region:
				reason = EvidenceDisjointPaths
			case reason == EvidenceDisjointObjects && (x.region.kind == regionSite) != (y.region.kind == regionSite):
				reason = EvidenceUnescapedLocal
			}
		}
	}
	graph.disjoint = append(graph.disjoint, AliasDecision{Value: left, Target: right, Reason: reason})
	return AliasProof{Aliases: false, Reason: reason, Provenance: EvidenceFromLocalSSA}
}

// aliasDepth bounds the placeholder chase: a placeholder may have been
// stored into the slot it stands for, so the walk answers conservatively
// past this depth instead of following itself.
const aliasDepth = 8

// slotsMayAlias reports whether two slots may be one location under the
// structural contract every consumer relies on: two objects are the same
// only when the function's own flow connects them. Two parameters, or two
// call results, are distinct objects even though at run time they might
// not be; a diagnostic about one is not a claim about the other. Unknown
// admits anything. Two slots of one object alias when their paths agree, a
// dynamic element step standing for any element. A placeholder, the unread
// content of a foreign slot, may be whatever was ever stored into a slot
// that aliases its source, and two placeholders of aliasing sources may be
// one object whatever writes lay between them.
func (graph *regionGraph) slotsMayAlias(x, y slot, depth int) bool {
	if depth == 0 || x.region.kind == regionUnknown || y.region.kind == regionUnknown {
		return true
	}
	if x.region == y.region {
		return pathsMayAlias(x.path, y.path)
	}
	if x.region.kind == regionNil || y.region.kind == regionNil {
		return false
	}
	if x.region.kind == regionPlaceholder && y.region.kind == regionPlaceholder {
		return pathsMayAlias(x.path, y.path) && graph.slotsMayAlias(x.region.source, y.region.source, depth-1)
	}
	if x.region.kind == regionPlaceholder {
		return graph.everHeld(x, y, depth-1)
	}
	if y.region.kind == regionPlaceholder {
		return graph.everHeld(y, x, depth-1)
	}
	return false
}

// everHeld reports whether the placeholder's source slot, or a slot that
// aliases it, ever held an object that may be the target. Only the
// placeholder object itself can be such content; a slot beneath it is an
// address inside that object.
func (graph *regionGraph) everHeld(placeholder, target slot, depth int) bool {
	if placeholder.path != "" {
		return false
	}
	source := placeholder.region.source
	for held, set := range graph.history {
		if held.region != source.region || !pathsMayAlias(held.path, source.path) {
			continue
		}
		for pointee := range set {
			if pointee.region.kind == regionUnknown {
				return true
			}
			if pointee.region == target.region && pathsMayAlias(pointee.path, target.path) {
				return true
			}
			// A slot given back its own earlier content, as an append to a
			// global does, adds nothing and would otherwise chase itself.
			if pointee.region.kind == regionPlaceholder && pointee.region.source != source && graph.slotsMayAlias(pointee, target, depth) {
				return true
			}
		}
	}
	return false
}

func pathsMayAlias(left, right string) bool {
	if left == right {
		return true
	}
	a, b := SplitAccessPath(left), SplitAccessPath(right)
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] && (!isIndexStep(a[i]) || !isIndexStep(b[i]) || a[i] != pathStar && b[i] != pathStar) {
			return false
		}
	}
	return true
}

// mustSame reports whether two values certainly refer to one object.
func (graph *regionGraph) mustSame(left, right ssa.Value) bool {
	a, okA := graph.pointsTo(left)
	b, okB := graph.pointsTo(right)
	if !okA || !okB {
		return false
	}
	x, okX := singleSlot(a)
	y, okY := singleSlot(b)
	return okX && okY && x == y
}

// singleSlot returns the one non-stale, non-unknown slot of a set.
func singleSlot(set pointees) (slot, bool) {
	if len(set) != 1 {
		return slot{}, false
	}
	for target, stale := range set {
		if stale || target.region.kind == regionUnknown || lastStep(target.path) == pathStar {
			return slot{}, false
		}
		return target, true
	}
	return slot{}, false
}

// contentAt returns what the addressed slots hold when the instruction runs.
func (graph *regionGraph) contentAt(address ssa.Value, at ssa.Instruction) (pointees, bool) {
	state := graph.stateAt(at)
	if state == nil {
		return nil, false
	}
	addresses := graph.pointees(address)
	if len(addresses) == 0 {
		return nil, false
	}
	return graph.load(state, addresses, address), true
}

// AddressIsUnescapedLocal reports whether every object the address may
// select from is a local allocation whose address never leaves the
// function: not stored anywhere the function does not own, not handed to a
// call it cannot see through, not captured by a closure that does either.
// What such an aggregate holds lives no longer than the aggregate itself.
func AddressIsUnescapedLocal(address ssa.Value) bool {
	graph := regionsOf(address)
	set, ok := graph.pointsTo(address)
	if !ok {
		return false
	}
	for target := range set {
		if target.region.kind != regionSite || graph.everEscaped(target.region) {
			return false
		}
	}
	return true
}

// everEscaped reports whether a site's address escaped at any point.
func (graph *regionGraph) everEscaped(site *region) bool {
	for _, states := range []map[*ssa.BasicBlock]*regionState{graph.entry, graph.exit} {
		for _, state := range states {
			if state.escaped[site] {
				return true
			}
		}
	}
	return false
}

// contentValue names the one object the addressed slot holds when the
// instruction runs, for a caller that compares objects by SSA identity.
func (graph *regionGraph) contentValue(address ssa.Value, at ssa.Instruction) (ssa.Value, bool) { //nolint:ireturn // Objects keep their concrete origins.
	set, ok := graph.contentAt(address, at)
	if !ok {
		return nil, false
	}
	target, ok := singleSlot(set)
	if !ok {
		return nil, false
	}
	return graph.valueOf(target)
}

// valueOf names the SSA value that produced the object a slot refers to,
// for callers that compare objects by their SSA identity.
func (graph *regionGraph) valueOf(target slot) (ssa.Value, bool) { //nolint:ireturn // Objects keep their concrete origins.
	if target.path != "" || target.region.origin == nil {
		return nil, false
	}
	switch target.region.kind {
	case regionSite, regionExternal, regionOpaque, regionSnapshot, regionClosure:
		return target.region.origin, true
	case regionPlaceholder, regionNil, regionUnknown:
		// A placeholder is the unread content of a slot: nothing wrote it,
		// so no SSA value is it. Naming the load that first read it would
		// make a later load resolve to an earlier one.
	}
	return nil, false
}
