package ssaflow

import (
	"slices"
	"sort"
	"strconv"
	"strings"

	"golang.org/x/tools/go/ssa"
)

// A heap summary is the projection of a function's points-to graph onto
// what a caller can name from outside: the parameters, results, globals,
// and captured variables, with a bounded set of paths beneath each. Every
// internal object collapses to fresh, nil, or unknown. The summary says
// where each named slot may point at exit, how each named object left
// local control, which slots the function read before writing, and where
// the projection was cut. It carries no internal structure, so applying it
// at a call site is substitution: the callee's parameter becomes the
// argument's slot, its result the call's, and fresh a new object.

// HeapRootKind names the kinds of object a caller can refer to.
type HeapRootKind uint8

const (
	// HeapParameter is the object parameter Index refers to; the receiver
	// is parameter zero.
	HeapParameter HeapRootKind = iota
	// HeapResult is the object result Index refers to.
	HeapResult
	// HeapGlobal is the package variable Name.
	HeapGlobal
	// HeapFreeVar is the captured variable Index of a literal.
	HeapFreeVar
)

// HeapRoot is one object a caller can name. A global is named by its
// package path and name, so a caller's graph can find the same variable.
type HeapRoot struct {
	Kind    HeapRootKind
	Index   int
	Package string
	Name    string
}

// HeapSlot is a location beneath a root: the root's object itself when
// Path is empty, else the field or element the joined access path selects.
type HeapSlot struct {
	Root HeapRoot
	Path string
}

// HeapTargetKind names what a slot may hold.
type HeapTargetKind uint8

const (
	// HeapTargetSlot is whatever the caller holds at Slot, or the object
	// Slot itself when its path is empty.
	HeapTargetSlot HeapTargetKind = iota
	// HeapTargetFresh is an object the function created, Origin naming the
	// call or literal that produced it when the graph could see one.
	HeapTargetFresh
	// HeapTargetNil is the nil pointer.
	HeapTargetNil
	// HeapTargetUnknown may be anything.
	HeapTargetUnknown
)

// HeapTarget is what a slot may hold. Object numbers a fresh object within
// its summary, so two fresh objects with one origin, such as the two
// results of one call, stay two objects when the summary is applied.
type HeapTarget struct {
	Kind   HeapTargetKind
	Slot   HeapSlot
	Origin string
	Object int
}

// HeapEdge says the slot may hold the target at exit; Must says it does on
// every normal return, and that nothing else does.
type HeapEdge struct {
	From HeapSlot
	To   HeapTarget
	Must bool
}

// HeapEscape is the set of ways an object left local control.
type HeapEscape uint8

const (
	// HeapEscapedGlobal: stored into a package variable.
	HeapEscapedGlobal HeapEscape = 1 << iota
	// HeapEscapedField: stored into an object the caller can reach, a map,
	// or a collection handed on.
	HeapEscapedField
	// HeapEscapedCall: handed to a call the graph could not see through.
	HeapEscapedCall
	// HeapEscapedAsync: handed to a goroutine.
	HeapEscapedAsync
	// HeapEscapedSend: sent on a channel.
	HeapEscapedSend
)

// HeapEffect records what happened to the object at a slot: how it escaped,
// or which lifecycle method released it. Every says the effect holds on
// every normal return.
type HeapEffect struct {
	Slot    HeapSlot
	Escape  HeapEscape
	Release string
	Every   bool
}

// HeapSummary is the projection of one function's heap.
type HeapSummary struct {
	Edges     []HeapEdge
	Effects   []HeapEffect
	Reads     []HeapSlot
	Truncated []HeapSlot
}

// heapPathDepth bounds the paths a summary names beneath a root, and
// heapSlotLimit the slots per root before the root is truncated instead.
const (
	heapPathDepth = 3
	heapSlotLimit = 16
)

// ProjectHeap computes the heap summary of a function from its points-to
// graph. It reports false when the graph is unavailable, or when the
// function has no normal return to project.
func ProjectHeap(function *ssa.Function) (HeapSummary, bool) {
	graph := regionsOfFunction(function)
	if !graph.available {
		return HeapSummary{}, false
	}
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
		Reads:     projection.reads(),
		Truncated: projection.truncated(states),
	}
	return summary, true
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
			if target.path == "" {
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
	case regionSite, regionOpaque, regionSnapshot, regionClosure:
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
	record := func(from HeapSlot, set pointees) {
		entry := entryFor(from)
		entry.returns++
		for pointee, stale := range set {
			target := projection.targetOf(pointee)
			entry.exit[target] = true
			entry.ever[target] = true
			entry.stale = entry.stale || stale
		}
	}
	for index, state := range states {
		projection.projectRoots(state, record)
		projection.projectResults(state, returns[index], record)
	}
	projection.projectHistory(func(from HeapSlot, set pointees) {
		entry := entryFor(from)
		for pointee := range set {
			entry.ever[projection.targetOf(pointee)] = true
		}
	})
	var edges []HeapEdge
	for from, entry := range witnesses {
		must := entry.returns == len(states) && len(entry.exit) == 1 && !entry.stale
		for target := range entry.ever {
			if target.Kind == HeapTargetSlot && target.Slot == from {
				continue
			}
			edges = append(edges, HeapEdge{From: from, To: target, Must: must && entry.exit[target] && target.Kind != HeapTargetUnknown})
		}
	}
	sort.Slice(edges, func(i, j int) bool { return heapEdgeLess(edges[i], edges[j]) })
	return edges
}

// projectRoots records the contents of every slot beneath a root that the
// state knows, within the depth and count bounds.
func (projection *heapProjection) projectRoots(state *regionState, record func(HeapSlot, pointees)) {
	counts := map[HeapRoot]int{}
	for target, set := range state.contents {
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

// projectHistory records everything the function ever stored into a named
// slot, within the depth bound.
func (projection *heapProjection) projectHistory(record func(HeapSlot, pointees)) {
	for target, set := range projection.graph.history {
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
		for target, contents := range state.contents {
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
		for object, kinds := range state.escapes {
			named, ok := projection.rootOf(object)
			if !ok {
				continue
			}
			for kind := HeapEscapedGlobal; kind <= HeapEscapedSend; kind <<= 1 {
				if kinds&kind != 0 {
					counts[HeapEffect{Slot: named, Escape: kind}]++
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

func sortedSlots(set map[HeapSlot]bool) []HeapSlot {
	slots := make([]HeapSlot, 0, len(set))
	for at := range set {
		slots = append(slots, at)
	}
	sort.Slice(slots, func(i, j int) bool { return heapSlotLess(slots[i], slots[j]) })
	return slots
}

func heapSlotLess(left, right HeapSlot) bool {
	if left.Root != right.Root {
		if left.Root.Kind != right.Root.Kind {
			return left.Root.Kind < right.Root.Kind
		}
		if left.Root.Index != right.Root.Index {
			return left.Root.Index < right.Root.Index
		}
		if left.Root.Package != right.Root.Package {
			return left.Root.Package < right.Root.Package
		}
		return left.Root.Name < right.Root.Name
	}
	return left.Path < right.Path
}

func heapEdgeLess(left, right HeapEdge) bool {
	if left.From != right.From {
		return heapSlotLess(left.From, right.From)
	}
	if left.To.Kind != right.To.Kind {
		return left.To.Kind < right.To.Kind
	}
	if left.To.Slot != right.To.Slot {
		return heapSlotLess(left.To.Slot, right.To.Slot)
	}
	return left.To.Origin < right.To.Origin
}

func heapEffectLess(left, right HeapEffect) bool {
	if left.Slot != right.Slot {
		return heapSlotLess(left.Slot, right.Slot)
	}
	if left.Escape != right.Escape {
		return left.Escape < right.Escape
	}
	return left.Release < right.Release
}

// String renders a slot as P0/field:1, R0, G:pkg.name, or F1.
func (at HeapSlot) String() string {
	var root string
	switch at.Root.Kind {
	case HeapParameter:
		root = "P" + strconv.Itoa(at.Root.Index)
	case HeapResult:
		root = "R" + strconv.Itoa(at.Root.Index)
	case HeapGlobal:
		root = "G:" + at.Root.Package + "." + at.Root.Name
	case HeapFreeVar:
		root = "F" + strconv.Itoa(at.Root.Index)
	}
	if at.Path == "" {
		return root
	}
	return root + "/" + at.Path
}

// String renders a target.
func (target HeapTarget) String() string {
	switch target.Kind {
	case HeapTargetSlot:
		return target.Slot.String()
	case HeapTargetFresh:
		return "fresh(" + target.Origin + "#" + strconv.Itoa(target.Object) + ")"
	case HeapTargetNil:
		return "nil"
	case HeapTargetUnknown:
	}
	return "unknown"
}

// String renders the summary one entry per line, for tests and the dump.
func (summary HeapSummary) String() string {
	var lines []string
	for _, edge := range summary.Edges {
		mode := "may"
		if edge.Must {
			mode = "must"
		}
		lines = append(lines, "edge "+edge.From.String()+" -> "+edge.To.String()+" "+mode)
	}
	for _, effect := range summary.Effects {
		mode := "some"
		if effect.Every {
			mode = "every"
		}
		what := "released " + effect.Release
		if effect.Release == "" {
			what = "escaped " + effect.Escape.String()
		}
		lines = append(lines, "effect "+effect.Slot.String()+" "+what+" "+mode)
	}
	for _, at := range summary.Reads {
		lines = append(lines, "read "+at.String())
	}
	for _, at := range summary.Truncated {
		lines = append(lines, "truncated "+at.String())
	}
	return strings.Join(lines, "\n")
}

// String renders the escape kinds.
func (escape HeapEscape) String() string {
	var names []string
	for kind, name := range map[HeapEscape]string{
		HeapEscapedGlobal: "global", HeapEscapedField: "field", HeapEscapedCall: "call", HeapEscapedAsync: "async", HeapEscapedSend: "send",
	} {
		if escape&kind != 0 {
			names = append(names, name)
		}
	}
	slices.Sort(names)
	return strings.Join(names, "+")
}
