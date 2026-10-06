package heapmodel

import (
	"go/types"

	proofs "github.com/kojah/gohawk/internal/engine/proof"
	"github.com/kojah/gohawk/internal/engine/ssaflow"
	"golang.org/x/tools/go/ssa"
)

// Graph relationship queries expose identity, containment and availability
// with explicit polarity. Public queries and their graph implementation share
// these contracts; construction, mutation and cache lifetimes remain separate.

// ProveMayAlias asks one function's graph whether two values may name the
// same object. An unavailable graph falls back to the structural value walk.
func ProveMayAlias(value, target ssa.Value) proofs.AliasProof {
	graph := regionsOf(value)
	if graph.available && valueFunction(target) == graph.function {
		return graph.aliasProof(value, target)
	}
	return proofs.AliasProof{
		Aliases:    ssaflow.StructurallySame(value, target),
		Reason:     proofs.EvidenceStructuralWalk,
		Provenance: proofs.EvidenceFromLocalSSA,
	}
}

// DefinitelySame reports exact graph identity, including through stable slots.
func DefinitelySame(left, right ssa.Value) bool {
	graph := regionsOf(left)
	return graph.available && valueFunction(right) == graph.function && graph.mustSame(left, right)
}

// ContentValue returns the exact SSA value held by an address at observation.
func ContentValue(address ssa.Value, observation ssa.Instruction) (ssa.Value, bool) {
	return regionsOf(address).contentValue(address, observation)
}

// Contains reports possible object containment in a function's graph.
func Contains(owner, value ssa.Value) bool {
	if !CanHoldReference(owner.Type()) {
		return false
	}
	graph := regionsOf(owner)
	return graph.available && valueFunction(value) == graph.function && graph.contains(owner, value)
}

// ContainsAt reports possible containment at one instruction, with known
// false distinguished from a graph that could not answer.
func ContainsAt(owner, value ssa.Value, at ssa.Instruction) (bool, bool) {
	if !CanHoldReference(owner.Type()) {
		return false, true
	}
	graph := regionsOf(owner)
	if !graph.available || valueFunction(value) != graph.function {
		return false, false
	}
	return graph.containsAt(owner, value, at)
}

// ValueAtPath finds the exact object at a static path beneath root.
func graphValueAtPath(root ssa.Value, path []string, at ssa.Instruction) (ssa.Value, bool) {
	return regionsOf(root).valueAtPath(root, path, at)
}

// ExclusiveAt proves a graph object's caller/local exclusivity at one point.
func ExclusiveAt(value ssa.Value, at ssa.Instruction) (ExclusiveObject, bool) {
	if at == nil || at.Parent() == nil {
		return ExclusiveObject{}, false
	}
	return regionsOfFunction(at.Parent()).exclusiveAt(value, at)
}

// StoredPath finds a path from root to target in the observed graph.
func graphStoredPath(root, target ssa.Value, at ssa.Instruction) ([]string, bool) {
	return regionsOf(root).storedPath(root, target, at)
}

// CanHoldReference reports whether a value of type value can refer to another
// object. A string, a number, or a struct or array made only of them cannot:
// a string's bytes are never an object the program releases. The points-to
// graph can still link such a value to the object it was read from, as a
// string field is to its owner, so containment asks the type first. SSA result
// tuples can retain references only through their component types.
// Real-world form: ForceCLI passes a zip entry's Name to strings.HasPrefix
// while the entry's reader is open,
// https://github.com/ForceCLI/force/blob/662af739b980a568fa55e3a4d7efe65cf2ec15b1/command/fetch.go#L321-L330
func CanHoldReference(value types.Type) bool {
	return anyByValueType(value, func(value types.Type) bool {
		switch value := value.Underlying().(type) {
		case *types.Basic:
			return value.Kind() == types.UnsafePointer || value.Kind() == types.Invalid
		case *types.Struct, *types.Array, *types.Tuple:
			return false
		default:
			return true
		}
	})
}

// GraphEvidence is an observational snapshot, not proof of a relationship.
// An absent or in-progress cached graph is distinct from a completed graph
// with no losses. Call records describe the last transfer at each instruction.
type GraphEvidence struct {
	Cached, Building bool
	BuildReason      GraphBuildReason
	Calls            []CallApplication
	// Widenings counts distinct (slot, instruction) sites, not fixpoint visits.
	Widenings int
	// Escapes counts distinct (slot, escape kind) records, not leaked resources.
	Escapes int
}

// CachedGraphEvidence reads an existing graph without building, publishing, or
// refreshing one. Tracing must not change inference by warming the graph cache.
// Callers should skip this query entirely when observation is disabled.
func CachedGraphEvidence(function *ssa.Function) GraphEvidence {
	regionGraphs.Lock()
	element, found := regionGraphs.entries[function]
	if !found {
		regionGraphs.Unlock()
		return GraphEvidence{}
	}
	entry := element.Value.(*regionGraphEntry) //nolint:forcetypeassert // The list holds only graph entries.
	graph := entry.graph
	regionGraphs.Unlock()
	if graph == nil {
		return GraphEvidence{Cached: true, Building: true}
	}
	defer graph.lock()()
	type wideningSite struct {
		target slot
		at     ssa.Instruction
	}
	seen := make(map[wideningSite]struct{}, len(graph.widened))
	for _, widening := range graph.widened {
		seen[wideningSite{widening.target, widening.at}] = struct{}{}
	}
	return GraphEvidence{
		Cached: true, BuildReason: graph.buildReason, Calls: graph.callApplications(),
		Widenings: len(seen), Escapes: len(graph.escapeOrigins),
	}
}

// GraphBuildReason classifies whether a points-to graph reached its fixpoint.
// The zero value is unavailable, not evidence of a complete empty graph.
type GraphBuildReason uint8

const (
	GraphBuildUnknown GraphBuildReason = iota
	GraphBuildComplete
	GraphBuildNoBody
	GraphBuildBudgetExhausted
	GraphBuildFixpointLimit
	graphBuildReasonCount
)

// String formats the stable reason code, not the optional explanatory detail.
func (reason GraphBuildReason) String() string {
	switch reason {
	case GraphBuildUnknown:
		return "graph-build-unknown"
	case GraphBuildComplete:
		return "graph-build-complete"
	case GraphBuildNoBody:
		return "graph-build-no-body"
	case GraphBuildBudgetExhausted:
		return "graph-build-budget-exhausted"
	case GraphBuildFixpointLimit:
		return "graph-build-fixpoint-limit"
	default:
		return "invalid-graph-build-reason"
	}
}

// Keep the human-readable graph dump stable without using prose as state.
func (graph *regionGraph) buildFailureText() string {
	switch graph.buildReason {
	case GraphBuildNoBody:
		return "no body"
	case GraphBuildBudgetExhausted:
		return "budget exhausted"
	case GraphBuildFixpointLimit:
		return "fixpoint did not settle " + graph.buildDetail
	default:
		return ""
	}
}

// Queries read the graph. Each one states its polarity: a may-answer is
// true whenever the graph cannot rule the relation out, and a must-answer
// is true only when one non-stale slot proves it. An unavailable graph
// answers may with true and must with false.

// pointsTo returns the slots a value may refer to, and whether the graph
// could say.
func (graph *regionGraph) pointsToUnlocked(value ssa.Value) (pointees, bool) {
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
func (graph *regionGraph) aliasProof(left, right ssa.Value) proofs.AliasProof {
	defer graph.lock()()
	if !tracked(left.Type()) || !tracked(right.Type()) || left == right {
		return proofs.AliasProof{
			Aliases: ssaflow.StructurallySame(left, right), Reason: proofs.EvidenceStructuralWalk,
			Provenance: proofs.EvidenceFromLocalSSA,
		}
	}
	a, okA := graph.pointsToUnlocked(left)
	b, okB := graph.pointsToUnlocked(right)
	if !okA || !okB {
		return proofs.AliasProof{Aliases: true, Reason: proofs.EvidenceUnknownPointee, Provenance: proofs.EvidenceFromLocalSSA}
	}
	reason := proofs.EvidenceDisjointObjects
	for x := range a {
		for y := range b {
			if graph.slotsMayAlias(x, y, aliasDepth) {
				return proofs.AliasProof{Aliases: true, Reason: proofs.EvidenceSharedSlot, Provenance: proofs.EvidenceFromLocalSSA}
			}
			switch {
			case x.region == y.region:
				reason = proofs.EvidenceDisjointPaths
			case reason == proofs.EvidenceDisjointObjects && (x.region.kind == regionSite) != (y.region.kind == regionSite):
				reason = proofs.EvidenceUnescapedLocal
			}
		}
	}
	graph.disjoint = append(graph.disjoint, AliasDecision{Value: left, Target: right, Reason: reason})
	return proofs.AliasProof{Aliases: false, Reason: reason, Provenance: proofs.EvidenceFromLocalSSA}
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
	if x.region.kind == regionNil || y.region.kind == regionNil {
		// nil is not an object: two values that may both be nil share
		// nothing a diagnostic could be about.
		return false
	}
	if x.region == y.region {
		return pathsMayAlias(x.path, y.path)
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
// aliases it, ever held an object whose matching projection may be the target.
// Preserve the placeholder's relative path: its field is not the whole owner
// or a sibling field, but may be the same field of an earlier occupant.
// https://github.com/centrifugal/centrifuge-go/blob/080126041ccc71654718bd0601b920ff8b22a8bf/client.go#L1435-L1762
func (graph *regionGraph) everHeld(placeholder, target slot, depth int) bool {
	source := placeholder.region.source
	for held, set := range graph.history {
		if held.region != source.region || !pathsMayAlias(held.path, source.path) {
			continue
		}
		for pointee := range set {
			pointee.path = joinSlotPath(pointee.path, placeholder.path)
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
	a, b := ssaflow.SplitAccessPath(left), ssaflow.SplitAccessPath(right)
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
	defer graph.lock()()
	a, okA := graph.pointsToUnlocked(left)
	b, okB := graph.pointsToUnlocked(right)
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
func (graph *regionGraph) contentAtUnlocked(address ssa.Value, at ssa.Instruction) (pointees, bool) {
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

// everContained reports whether some slot beneath the object was ever given
// one of the target's objects: the aggregate held the target at some point,
// possibly in another iteration of a loop.
func (graph *regionGraph) everContainedUnlocked(object slot, target pointees, budget *proofs.SearchBudget) bool {
	if !budget.Spend() {
		return false
	}
	if object.region.kind == regionUnknown {
		return true
	}
	for held, set := range graph.history {
		if !budget.Spend() {
			return false
		}
		if held.region != object.region || !slotBeneath(held.path, object.path) || held.path == object.path {
			continue
		}
		for pointee := range set {
			if !budget.Spend() {
				return false
			}
			if pointee.region.kind == regionUnknown {
				return true
			}
			if _, ok := target[pointee]; ok {
				return true
			}
		}
	}
	return false
}

// storedPath returns the access path beneath the root's object at which the
// target is stored when the instruction runs: the one slot, at most two
// steps down, whose content is exactly the target's object.
func (graph *regionGraph) storedPath(root, target ssa.Value, at ssa.Instruction) ([]string, bool) {
	defer graph.lock()()
	state := graph.stateAt(at)
	if state == nil {
		return nil, false
	}
	base, ok := singleSlot(graph.pointees(root))
	if !ok {
		return nil, false
	}
	object, ok := graph.pointsToUnlocked(target)
	if !ok {
		return nil, false
	}
	wanted, ok := singleSlot(object)
	if !ok {
		return nil, false
	}
	for _, candidate := range orderedSlots(state.contents) {
		if candidate.region != base.region || !slotBeneath(candidate.path, base.path) || candidate.path == base.path {
			continue
		}
		relative := ssaflow.SplitAccessPath(trimSlash(candidate.path[len(base.path):]))
		if len(relative) > 2 {
			continue
		}
		if held, ok := singleSlot(graph.content(state, candidate)); ok && held == wanted {
			return relative, true
		}
	}
	return nil, false
}

// valueAtPath names the one object stored at path beneath the root's object
// when the instruction runs.
//
//nolint:ireturn // Objects keep their concrete origins.
func (graph *regionGraph) valueAtPath(root ssa.Value, path []string, at ssa.Instruction) (ssa.Value, bool) {
	defer graph.lock()()
	state := graph.stateAt(at)
	if state == nil {
		return nil, false
	}
	base, ok := singleSlot(graph.pointees(root))
	if !ok {
		return nil, false
	}
	target := slot{region: base.region, path: joinSlotPath(base.path, ssaflow.JoinAccessPath(path))}
	held, ok := singleSlot(graph.content(state, target))
	if !ok {
		return nil, false
	}
	return graph.valueOf(held)
}

// contentValue names the one object the addressed slot holds when the
// instruction runs, for a caller that compares objects by SSA identity.
func (graph *regionGraph) contentValue(address ssa.Value, at ssa.Instruction) (ssa.Value, bool) { //nolint:ireturn // Objects keep their concrete origins.
	defer graph.lock()()
	set, ok := graph.contentAtUnlocked(address, at)
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

// lock serializes the queries on one graph. A query replays a block's
// instructions from its entry state, which interns regions and unions
// pointees as the fixpoint did, and graphs of dependency functions are
// queried from several package actions at once.
func (graph *regionGraph) lock() func() {
	graph.mu.Lock()
	return graph.mu.Unlock
}

// pointsTo is pointsToUnlocked for a caller outside the graph's own queries.
func (graph *regionGraph) pointsTo(value ssa.Value) (pointees, bool) {
	defer graph.lock()()
	return graph.pointsToUnlocked(value)
}

// everContainedWithin is everContainedUnlocked for a caller outside the graph's
// own queries.
func (graph *regionGraph) everContainedWithin(object slot, target pointees, budget *proofs.SearchBudget) bool {
	defer graph.lock()()
	return graph.everContainedUnlocked(object, target, budget)
}

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

// AliasDecision records a graph disjointness answer for evidence dumps.
type AliasDecision struct {
	Value, Target ssa.Value
	Reason        proofs.EvidenceReason
}
