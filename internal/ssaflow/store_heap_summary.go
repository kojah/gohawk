package ssaflow

import (
	"sort"

	"golang.org/x/tools/go/ssa"
)

// The projection reads one function's graph at its returns and names what
// a caller can see: the contents of every named slot, how each escaped,
// which results hold which parameters, what was read before being written,
// and where the projection had to stop. Nothing here is applied; the
// substitution at call sites lives beside the registry.

// ProjectHeap computes the heap summary of a function from its points-to
// graph. It reports false when the graph is unavailable, or when the
// function has no normal return to project.
func ProjectHeap(function *ssa.Function) (HeapSummary, bool) {
	graph := regionsOfFunction(function)
	if !graph.available {
		return HeapSummary{}, false
	}
	// A graph already locked is being queried or projected higher on this
	// stack, which a recursive call reaches; the projection is then not
	// available here, and the call that asked for it stays unresolved.
	if !graph.mu.TryLock() {
		return HeapSummary{}, false
	}
	defer graph.mu.Unlock()
	projection := &heapProjection{graph: graph, roots: map[*region]HeapRoot{}}
	projection.nameRoots()
	returns := InstructionsOf[*ssa.Return](function)
	var states []*regionState
	for _, returned := range returns {
		if state := graph.stateAt(returned); state != nil {
			projection.nameResults(returned)
			states = append(states, state)
		}
	}
	if len(states) == 0 {
		return HeapSummary{}, false
	}
	summary := HeapSummary{
		Edges:     projection.edges(states, returns),
		Effects:   projection.escapes(states),
		Holds:     projection.holds(states, returns),
		Reads:     projection.reads(),
		Truncated: projection.truncated(states),
	}
	// Requirements are named after the cuts are known, so a slot the
	// projection could not describe is never one the caller is held to.
	summary.Requires = projection.requirements()
	return summary, true
}

// holds decides, per return, whether each result holds each parameter's
// object, itself or beneath, and joins the answers.
func (projection *heapProjection) holds(states []*regionState, returns []*ssa.Return) []HeapHold {
	type key struct{ result, parameter int }
	counts := map[key]int{}
	relevant := map[int]int{}
	for index, state := range states {
		for resultIndex, result := range returns[index].Results {
			set := projection.graph.pointees(result)
			if isNilSet(set) || !tracked(result.Type()) {
				continue
			}
			relevant[resultIndex]++
			for parameter := range projection.graph.function.Params {
				if projection.resultHolds(state, set, parameter) {
					counts[key{resultIndex, parameter}]++
				}
			}
		}
	}
	var holds []HeapHold
	for at, count := range counts {
		holds = append(holds, HeapHold{Result: at.result, Parameter: at.parameter, Must: count == relevant[at.result] && count > 0})
	}
	sort.Slice(holds, func(i, j int) bool {
		if holds[i].Result != holds[j].Result {
			return holds[i].Result < holds[j].Result
		}
		return holds[i].Parameter < holds[j].Parameter
	})
	return holds
}

// resultHolds reports whether, in the state, every object the result refers
// to is the parameter's object or holds it beneath a bounded path; nil
// alternatives do not count against it.
func (projection *heapProjection) resultHolds(state *regionState, set pointees, parameter int) bool {
	wanted := projection.graph.external(projection.graph.function.Params[parameter])
	found := false
	for target, stale := range set {
		if target.region.kind == regionNil {
			continue
		}
		if stale || target.path != "" {
			return false
		}
		if target.region == wanted {
			found = true
			continue
		}
		if !projection.objectHolds(state, target.region, wanted, heapPathDepth) {
			return false
		}
		found = true
	}
	return found
}

// objectHolds reports whether some slot beneath the object holds exactly
// the wanted object, within depth.
func (projection *heapProjection) objectHolds(state *regionState, object, wanted *region, depth int) bool {
	if depth == 0 {
		return false
	}
	for held, set := range state.contents {
		if held.region != object {
			continue
		}
		content, ok := singleSlot(set)
		if !ok {
			continue
		}
		if content.region == wanted && content.path == "" {
			return true
		}
		if content.path == "" && projection.objectHolds(state, content.region, wanted, depth-1) {
			return true
		}
	}
	return false
}

type heapProjection struct {
	graph *regionGraph
	// roots maps each region a caller can name to its root. Result regions
	// are named per return; when returns disagree the result is projected
	// as its targets rather than as an object with slots.
	roots   map[*region]HeapRoot
	results map[int]map[*region]bool
	cuts    map[HeapSlot]bool
	// objects numbers the fresh objects the summary names.
	objects map[*region]int
}

func (projection *heapProjection) nameRoots() {
	function := projection.graph.function
	for index, parameter := range function.Params {
		projection.roots[projection.graph.external(parameter)] = HeapRoot{Kind: HeapParameter, Index: index}
	}
	for index, free := range function.FreeVars {
		projection.roots[projection.graph.external(free)] = HeapRoot{Kind: HeapFreeVar, Index: index}
	}
	for key, object := range projection.graph.regions {
		if key.kind != regionExternal {
			continue
		}
		if global, ok := object.origin.(*ssa.Global); ok {
			projection.roots[object] = HeapRoot{Kind: HeapGlobal, Package: global.Pkg.Pkg.Path(), Name: global.Name()}
		}
	}
}

// nameResults records which objects each result refers to at a return.
func (projection *heapProjection) nameResults(returned *ssa.Return) {
	if projection.results == nil {
		projection.results = map[int]map[*region]bool{}
	}
	for index, result := range returned.Results {
		set := projection.graph.pointees(result)
		if projection.results[index] == nil {
			projection.results[index] = map[*region]bool{}
		}
		for target := range set {
			if target.path == "" && target.region.kind != regionNil {
				projection.results[index][target.region] = true
			}
		}
	}
}

// rootOf names the root an object corresponds to, with the path beneath it
// for a placeholder standing for a slot's unwritten content.
func (projection *heapProjection) rootOf(object *region) (HeapSlot, bool) {
	if root, ok := projection.roots[object]; ok {
		return HeapSlot{Root: root}, true
	}
	if object.kind == regionPlaceholder {
		source, ok := projection.rootOf(object.source.region)
		if !ok {
			return HeapSlot{}, false
		}
		return HeapSlot{Root: source.Root, Path: joinSlotPath(source.Path, object.source.path)}, true
	}
	return HeapSlot{}, false
}

// targetOf names what a pointee stands for from outside.
func (projection *heapProjection) targetOf(pointee slot) HeapTarget {
	switch pointee.region.kind {
	case regionNil:
		return HeapTarget{Kind: HeapTargetNil}
	case regionUnknown:
		return HeapTarget{Kind: HeapTargetUnknown}
	case regionSnapshot:
		// A copy of a named object's value is that value from outside: the
		// caller holds it at the slot the copy was taken from.
		if pointee.path == "" && pointee.region.source.region != nil {
			if named, ok := projection.rootOf(pointee.region.source.region); ok {
				return HeapTarget{Kind: HeapTargetSlot, Slot: HeapSlot{Root: named.Root, Path: joinSlotPath(named.Path, pointee.region.source.path)}}
			}
		}
		fallthrough
	case regionSite, regionOpaque, regionClosure:
		if pointee.path != "" {
			// An address inside a fresh object has no name outside.
			return HeapTarget{Kind: HeapTargetUnknown}
		}
		if projection.objects == nil {
			projection.objects = map[*region]int{}
		}
		number, ok := projection.objects[pointee.region]
		if !ok {
			number = len(projection.objects) + 1
			projection.objects[pointee.region] = number
		}
		return HeapTarget{Kind: HeapTargetFresh, Origin: freshOrigin(pointee.region), Object: number}
	case regionExternal, regionPlaceholder:
		named, ok := projection.rootOf(pointee.region)
		if !ok {
			return HeapTarget{Kind: HeapTargetUnknown}
		}
		return HeapTarget{Kind: HeapTargetSlot, Slot: HeapSlot{Root: named.Root, Path: joinSlotPath(named.Path, pointee.path)}}
	}
	return HeapTarget{Kind: HeapTargetUnknown}
}

// freshOrigin names the call or literal that produced a fresh object.
func freshOrigin(object *region) string {
	switch origin := object.origin.(type) {
	case *ssa.Call:
		if callee := origin.Common().StaticCallee(); callee != nil {
			return callee.String()
		}
		return "call"
	case *ssa.Extract:
		if call, ok := origin.Tuple.(*ssa.Call); ok {
			if callee := call.Common().StaticCallee(); callee != nil {
				return callee.String()
			}
		}
		return "call"
	case *ssa.Alloc:
		return "new"
	case *ssa.MakeClosure:
		return "closure"
	}
	return ""
}

// edges projects every named slot's contents. A must edge comes from the
// exit states alone: every return agrees on one non-stale target. May edges
// also come from the whole build's history, because a write the function
// made stays a write the caller must expect even after a later call made
// the graph forget the slot's exact content.
func (projection *heapProjection) edges(states []*regionState, returns []*ssa.Return) []HeapEdge {
	type witness struct {
		exit    map[HeapTarget]bool
		ever    map[HeapTarget]bool
		stale   bool
		returns int
	}
	witnesses := map[HeapSlot]*witness{}
	entryFor := func(from HeapSlot) *witness {
		entry := witnesses[from]
		if entry == nil {
			entry = &witness{exit: map[HeapTarget]bool{}, ever: map[HeapTarget]bool{}}
			witnesses[from] = entry
		}
		return entry
	}
	// A result root is judged only on the returns where the result is not
	// nil: a constructor that returns nil beside an error on its failure
	// path still holds its parameter in its result on every return that
	// has one, which is the claim a returned owner makes.
	relevant := map[HeapRoot]int{}
	record := func(from HeapSlot, set pointees) {
		entry := entryFor(from)
		entry.returns++
		for _, pointee := range orderedSlots(set) {
			stale := set[pointee]
			target := projection.targetOf(pointee)
			if from.Root.Kind == HeapResult && target.Kind == HeapTargetNil {
				entry.ever[target] = true
				continue
			}
			entry.exit[target] = true
			entry.ever[target] = true
			entry.stale = entry.stale || stale
		}
	}
	for index, state := range states {
		projection.projectRoots(state, record)
		projection.projectResults(state, returns[index], record)
		for _, root := range projection.roots {
			relevant[root]++
		}
		for resultIndex, result := range returns[index].Results {
			if !isNilSet(projection.graph.pointees(result)) {
				relevant[HeapRoot{Kind: HeapResult, Index: resultIndex}]++
			}
		}
	}
	projection.projectHistory(func(from HeapSlot, set pointees) {
		entry := entryFor(from)
		for _, pointee := range orderedSlots(set) {
			entry.ever[projection.targetOf(pointee)] = true
		}
	})
	var edges []HeapEdge
	for from, entry := range witnesses {
		must := entry.returns == relevant[from.Root] && len(entry.exit) == 1 && !entry.stale
		for target := range entry.ever {
			edge := HeapEdge{From: from, To: target, Must: must && entry.exit[target] && target.Kind != HeapTargetUnknown}
			if edgeWorthListing(edge, entry.exit) {
				edges = append(edges, edge)
			}
		}
	}
	sort.Slice(edges, func(i, j int) bool { return heapEdgeLess(edges[i], edges[j]) })
	return edges
}

// orderedSlots lists a slot map's keys in a fixed order, so the slots a
// bound keeps, and the numbers fresh objects receive, do not depend on
// map iteration and the summary is the same on every run.
func orderedSlots[Value any](entries map[slot]Value) []slot {
	slots := make([]slot, 0, len(entries))
	for target := range entries {
		slots = append(slots, target)
	}
	sort.Slice(slots, func(i, j int) bool {
		if slots[i].region != slots[j].region {
			return slots[i].region.serial < slots[j].region.serial
		}
		return slots[i].path < slots[j].path
	})
	return slots
}

// projectRoots records the contents of every slot beneath a root that the
// state knows, within the depth and count bounds.
func (projection *heapProjection) projectRoots(state *regionState, record func(HeapSlot, pointees)) {
	counts := map[HeapRoot]int{}
	for _, target := range orderedSlots(state.contents) {
		set := state.contents[target]
		named, ok := projection.rootOf(target.region)
		if !ok {
			continue
		}
		path := joinSlotPath(named.Path, target.path)
		if len(SplitAccessPath(path)) > heapPathDepth {
			continue
		}
		counts[named.Root]++
		if counts[named.Root] > heapSlotLimit {
			projection.truncate(HeapSlot{Root: named.Root})
			continue
		}
		record(HeapSlot{Root: named.Root, Path: path}, set)
	}
}

// isNilSet reports a set that holds only nil.
func isNilSet(set pointees) bool {
	for target := range set {
		if target.region.kind != regionNil {
			return false
		}
	}
	return len(set) > 0
}

// edgeWorthListing drops the edges a summary need not carry: a slot that
// holds itself, and what a caller assumes anyway. A result that may be
// fresh is only assumed when nothing else is said about it. Beside another
// target the fresh alternative is what keeps that target a may edge when
// the summary is applied: bufio.NewReaderSize returns its argument or a new
// Reader, and a caller that saw only the argument would hold it as the one
// result.
func edgeWorthListing(edge HeapEdge, exit map[HeapTarget]bool) bool {
	if edge.To.Kind == HeapTargetSlot && edge.To.Slot == edge.From {
		return false
	}
	if !assumedByDefault(edge) {
		return true
	}
	if edge.To.Kind != HeapTargetFresh {
		return false
	}
	for target := range exit {
		if target.Kind != HeapTargetFresh && target.Kind != HeapTargetNil {
			return true
		}
	}
	return false
}

// assumedByDefault reports an edge a caller applying the summary assumes
// without being told: a result that may be nil, or that may be a fresh
// object, which is what an undescribed result becomes anyway. A must edge
// to a fresh object is kept, because it carries the object's origin.
func assumedByDefault(edge HeapEdge) bool {
	if edge.From.Root.Kind != HeapResult || edge.From.Path != "" {
		return false
	}
	switch edge.To.Kind {
	case HeapTargetNil:
		return true
	case HeapTargetFresh:
		return !edge.Must
	case HeapTargetSlot, HeapTargetUnknown:
	}
	return false
}

// projectHistory records everything the function ever stored into a named
// slot, within the depth bound.
func (projection *heapProjection) projectHistory(record func(HeapSlot, pointees)) {
	for _, target := range orderedSlots(projection.graph.history) {
		set := projection.graph.history[target]
		named, ok := projection.rootOf(target.region)
		if !ok {
			continue
		}
		path := joinSlotPath(named.Path, target.path)
		if len(SplitAccessPath(path)) > heapPathDepth {
			continue
		}
		record(HeapSlot{Root: named.Root, Path: path}, set)
	}
}

// projectResults records what each result refers to and, when it refers to
// one fresh object, that object's slots as slots beneath the result.
func (projection *heapProjection) projectResults(state *regionState, returned *ssa.Return, record func(HeapSlot, pointees)) {
	for index, result := range returned.Results {
		if !tracked(result.Type()) {
			continue
		}
		root := HeapRoot{Kind: HeapResult, Index: index}
		set := projection.graph.pointees(result)
		record(HeapSlot{Root: root}, set)
		object, ok := singleSlot(set)
		if !ok || len(projection.results[index]) != 1 {
			continue
		}
		if _, named := projection.roots[object.region]; named {
			continue
		}
		count := 0
		for _, target := range orderedSlots(state.contents) {
			contents := state.contents[target]
			if target.region != object.region || target.path == "" || len(SplitAccessPath(target.path)) > heapPathDepth {
				continue
			}
			count++
			if count > heapSlotLimit {
				projection.truncate(HeapSlot{Root: root})
				break
			}
			record(HeapSlot{Root: root, Path: target.path}, contents)
		}
	}
}

func (projection *heapProjection) truncate(at HeapSlot) {
	if projection.cuts == nil {
		projection.cuts = map[HeapSlot]bool{}
	}
	projection.cuts[at] = true
}

// escapes projects how each named object left local control, and whether
// it did so on every return.
func (projection *heapProjection) escapes(states []*regionState) []HeapEffect {
	counts := map[HeapEffect]int{}
	for _, state := range states {
		for target, kinds := range state.escapes {
			named, ok := projection.rootOf(target.region)
			if !ok {
				continue
			}
			path := joinSlotPath(named.Path, target.path)
			if len(SplitAccessPath(path)) > heapPathDepth {
				continue
			}
			at := HeapSlot{Root: named.Root, Path: path}
			for kind := HeapEscapedGlobal; kind <= HeapEscapedSend; kind <<= 1 {
				if kinds&kind != 0 {
					counts[HeapEffect{Slot: at, Escape: kind}]++
				}
			}
		}
	}
	var effects []HeapEffect
	for effect, count := range counts {
		effect.Every = count == len(states)
		effects = append(effects, effect)
	}
	sort.Slice(effects, func(i, j int) bool { return heapEffectLess(effects[i], effects[j]) })
	return effects
}

// reads lists the named slots the function read before writing: every
// placeholder that stands for a root's slot.
func (projection *heapProjection) reads() []HeapSlot {
	seen := map[HeapSlot]bool{}
	for key, object := range projection.graph.regions {
		if key.kind != regionPlaceholder {
			continue
		}
		if named, ok := projection.rootOf(object); ok {
			seen[named] = true
		}
	}
	return sortedSlots(seen)
}

// truncated lists where the projection stops being exact: every root when
// an unresolved call may have written anything, and the roots whose slots
// exceeded the bound.
func (projection *heapProjection) truncated(states []*regionState) []HeapSlot {
	seen := map[HeapSlot]bool{}
	for at := range projection.cuts {
		seen[at] = true
	}
	for _, state := range states {
		if state.opaque {
			for _, root := range projection.roots {
				seen[HeapSlot{Root: root}] = true
			}
			for index := range projection.results {
				seen[HeapSlot{Root: HeapRoot{Kind: HeapResult, Index: index}}] = true
			}
		}
	}
	return sortedSlots(seen)
}
