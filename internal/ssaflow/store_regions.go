package ssaflow

import (
	"container/list"
	"maps"
	"strings"
	"sync"

	"golang.org/x/tools/go/ssa"
)

// A region graph is the one place the package models memory. It maps every
// pointer-valued expression of a function to the abstract objects it may
// refer to and records, at each program point, what those objects hold. The
// identity, containment, and storage questions the analyzers ask are then
// lookups in that structure rather than separate walks over the SSA value
// graph, so a copy, a merge, or a captured cell is understood once. The
// graph is flow-sensitive within one function and knows nothing about other
// functions beyond the call-effect proof it consults; a body it cannot finish
// within its budget is unavailable, and every answer about it is unknown.
// See docs/development/points-to-model.md for the model.

// regionKind classifies an abstract object by where the graph learned of it.
type regionKind uint8

const (
	// regionNil is the nil pointer.
	regionNil regionKind = iota
	// regionUnknown may be any object; it never supports a must-answer.
	regionUnknown
	// regionSite is a local allocation, one per Alloc instruction.
	regionSite
	// regionExternal is the object a parameter, free variable, or global
	// refers to, which existed before the function ran.
	regionExternal
	// regionOpaque is an object produced by an instruction the graph cannot
	// see through, such as a call result.
	regionOpaque
	// regionPlaceholder is the content of a slot the function never wrote,
	// identified by the slot and the write version of its region.
	regionPlaceholder
	// regionSnapshot is an aggregate value copied at one point.
	regionSnapshot
	// regionClosure holds the bindings of a function literal.
	regionClosure
)

// region is one abstract object. Regions are interned per graph, so identity
// is pointer identity.
type region struct {
	kind   regionKind
	origin ssa.Value
	// source and stamp identify a placeholder: the slot it stands for and
	// the write stamp of that slot when it was read. A snapshot keeps the
	// stamps of its source at the copy, so a sub-slot the copy did not list
	// resolves to the placeholder a load at that moment would have produced.
	source slot
	stamp  versionStamp
	stamps map[string]int
}

// versionStamp identifies the last unfollowed write that may have changed a
// slot: the epoch is the last call or write through an unknown pointer, the
// step the last write through a foreign pointer to a field or element with
// the same last step. Both are instruction or block identities, so a
// fixpoint that revisits a write produces the same stamp again.
type versionStamp struct {
	epoch int
	step  int
}

// slot is one abstract location: a region and an access path beneath it in
// JoinAccessPath form, empty for the object itself.
type slot struct {
	region *region
	path   string
}

// pathStar is the element step of an array or slice indexed dynamically; it
// overlaps every constant element step beneath the same prefix.
const pathStar = "index:*"

// pointees is the set of slots a value may refer to, or a slot may hold. A
// stale entry was carried around a back edge from an earlier iteration and
// supports may-answers only.
type pointees map[slot]bool

// add records a slot, keeping an entry stale once any addition was.
func (set pointees) add(target slot, stale bool) {
	set[target] = set[target] || stale
}

func (set pointees) union(other pointees) {
	for target, stale := range other {
		set.add(target, stale)
	}
}

func (set pointees) clone() pointees {
	result := make(pointees, len(set))
	maps.Copy(result, set)
	return result
}

// unknown reports whether the set admits any object.
func (set pointees) unknown() bool {
	for target := range set {
		if target.region.kind == regionUnknown {
			return true
		}
	}
	return false
}

// regionKey interns regions.
type regionKey struct {
	kind   regionKind
	origin ssa.Value
	source slot
	stamp  versionStamp
}

// regionGraph is the points-to graph of one function.
type regionGraph struct {
	function  *ssa.Function
	available bool
	regions   map[regionKey]*region
	// values maps each pointer-valued expression to its pointees. SSA
	// values are immutable, so the map only grows during the fixpoint.
	values map[ssa.Value]pointees
	// views records constant slice windows over local arrays so an element
	// selected through a slice names the array's own element.
	views map[ssa.Value]sliceView
	// entry holds the state at the start of each block after the fixpoint,
	// and exit the state at its end.
	entry map[*ssa.BasicBlock]*regionState
	exit  map[*ssa.BasicBlock]*regionState
	// history unions, over the whole build, what each slot was ever given;
	// see slotsMayAlias. A later effect may forget a slot's content in the
	// state, but not that the function once put the object there.
	history map[slot]pointees
	// order is the reverse postorder the fixpoint used.
	order  []*ssa.BasicBlock
	budget *SearchBudget
	nilR   *region
	unkR   *region
	// ids number instructions and blocks so stamps are stable.
	ids map[ssa.Instruction]int
	// disjoint records every disjointness answer, for attribution.
	disjoint []AliasDecision
}

// sliceView is a constant window over a local array.
type sliceView struct {
	offset, size, capacity int64
}

// regionBuildBudget bounds the instruction transfers one graph may spend,
// across every fixpoint round. A function past it is unavailable rather
// than half-modeled.
const regionBuildBudget = 200_000

// regionFixpointRounds bounds the rounds over the blocks. Ordinary loops
// settle in two or three; a body that has not settled by then is unavailable.
const regionFixpointRounds = 6

// regionGraphCache keeps the most recently built graphs. Analyzers ask many
// questions about one function in a row, and the graph is built once for
// all of them; functions of packages already analyzed fall out of the cache.
const regionGraphCache = 128

var regionGraphs = struct {
	sync.Mutex
	order   *list.List
	entries map[*ssa.Function]*list.Element
}{order: list.New(), entries: map[*ssa.Function]*list.Element{}}

type regionGraphEntry struct {
	function *ssa.Function
	graph    *regionGraph
}

// regionsOf returns the points-to graph of the function that owns value,
// building it on first use. A value with no function, or a function with no
// body, has an unavailable graph.
func regionsOf(value ssa.Value) *regionGraph {
	return regionsOfFunction(valueFunction(value))
}

// regionsOfFunction returns the function's points-to graph, building it on
// first use.
func regionsOfFunction(function *ssa.Function) *regionGraph {
	if function == nil || len(function.Blocks) == 0 {
		return &regionGraph{}
	}
	regionGraphs.Lock()
	if element, ok := regionGraphs.entries[function]; ok {
		regionGraphs.order.MoveToFront(element)
		graph := element.Value.(*regionGraphEntry).graph //nolint:forcetypeassert // The list holds only entries.
		regionGraphs.Unlock()
		return graph
	}
	regionGraphs.Unlock()
	graph := buildRegionGraph(function)
	regionGraphs.Lock()
	defer regionGraphs.Unlock()
	if element, ok := regionGraphs.entries[function]; ok {
		return element.Value.(*regionGraphEntry).graph //nolint:forcetypeassert // The list holds only entries.
	}
	regionGraphs.entries[function] = regionGraphs.order.PushFront(&regionGraphEntry{function: function, graph: graph})
	for regionGraphs.order.Len() > regionGraphCache {
		oldest := regionGraphs.order.Back()
		delete(regionGraphs.entries, oldest.Value.(*regionGraphEntry).function) //nolint:forcetypeassert // The list holds only entries.
		regionGraphs.order.Remove(oldest)
	}
	return graph
}

func valueFunction(value ssa.Value) *ssa.Function {
	switch typed := value.(type) {
	case nil:
		return nil
	case ssa.Instruction:
		// An instruction outside any block, such as one a test built by
		// hand, belongs to no function.
		if typed.Block() == nil {
			return nil
		}
		return typed.Parent()
	case *ssa.Parameter:
		return typed.Parent()
	case *ssa.FreeVar:
		return typed.Parent()
	}
	return nil
}

func buildRegionGraph(function *ssa.Function) *regionGraph {
	graph := &regionGraph{
		function: function,
		regions:  map[regionKey]*region{},
		values:   map[ssa.Value]pointees{},
		views:    map[ssa.Value]sliceView{},
		entry:    map[*ssa.BasicBlock]*regionState{},
		history:  map[slot]pointees{},
		budget:   NewSearchBudget(regionBuildBudget),
		ids:      map[ssa.Instruction]int{},
	}
	graph.nilR = graph.intern(regionKey{kind: regionNil})
	graph.unkR = graph.intern(regionKey{kind: regionUnknown})
	graph.order = reversePostorder(function)
	graph.available = graph.fixpoint()
	if !graph.available {
		graph.entry = nil
		graph.exit = nil
	}
	return graph
}

func (graph *regionGraph) intern(key regionKey) *region {
	if existing, ok := graph.regions[key]; ok {
		return existing
	}
	created := &region{kind: key.kind, origin: key.origin, source: key.source, stamp: key.stamp}
	graph.regions[key] = created
	return created
}

func (graph *regionGraph) site(origin ssa.Value) *region {
	return graph.intern(regionKey{kind: regionSite, origin: origin})
}

func (graph *regionGraph) external(origin ssa.Value) *region {
	return graph.intern(regionKey{kind: regionExternal, origin: origin})
}

func (graph *regionGraph) opaque(origin ssa.Value) *region {
	return graph.intern(regionKey{kind: regionOpaque, origin: origin})
}

func (graph *regionGraph) snapshot(origin ssa.Value) *region {
	return graph.intern(regionKey{kind: regionSnapshot, origin: origin})
}

func (graph *regionGraph) closure(origin ssa.Value) *region {
	return graph.intern(regionKey{kind: regionClosure, origin: origin})
}

func (graph *regionGraph) placeholder(source slot, stamp versionStamp) *region {
	return graph.intern(regionKey{kind: regionPlaceholder, source: source, stamp: stamp})
}

// id numbers an instruction stably for stamps.
func (graph *regionGraph) id(instruction ssa.Instruction) int {
	if id, ok := graph.ids[instruction]; ok {
		return id
	}
	id := len(graph.ids) + 1
	graph.ids[instruction] = id
	return id
}

// blockID numbers a join block for stamps, apart from instruction ids.
func (graph *regionGraph) blockID(block *ssa.BasicBlock) int {
	return -(block.Index + 1)
}

// fixpoint runs the transfer over the blocks in reverse postorder until the
// entry states settle, and reports whether they did within the bounds.
func (graph *regionGraph) fixpoint() bool {
	function := graph.function
	if len(function.Blocks) == 0 {
		return false
	}
	graph.entry[function.Blocks[0]] = newRegionState()
	out := map[*ssa.BasicBlock]*regionState{}
	for range regionFixpointRounds {
		changed := false
		for _, block := range graph.order {
			in := graph.entryState(block, out)
			if in == nil {
				continue
			}
			state := in.clone()
			for _, instruction := range block.Instrs {
				if !graph.budget.Spend() {
					return false
				}
				graph.transfer(state, instruction)
			}
			if previous, ok := out[block]; !ok || !previous.equal(state) {
				changed = true
			}
			out[block] = state
		}
		if !changed {
			graph.exit = out
			return true
		}
	}
	return false
}

// entryState merges the predecessors' out states into the block's entry
// state, marking back edges, and returns nil for a block nothing reaches yet.
func (graph *regionGraph) entryState(block *ssa.BasicBlock, out map[*ssa.BasicBlock]*regionState) *regionState {
	if block == graph.function.Blocks[0] {
		return graph.entry[block]
	}
	var merged *regionState
	for _, predecessor := range block.Preds {
		state, ok := out[predecessor]
		if !ok {
			continue
		}
		if merged == nil {
			merged = state.clone()
			if block.Dominates(predecessor) {
				graph.merge(merged, state, true, block)
			}
			continue
		}
		graph.merge(merged, state, block.Dominates(predecessor), block)
	}
	if merged == nil {
		return nil
	}
	graph.entry[block] = merged
	return merged
}

// stateAt replays the block's entry state up to, but not including, the
// instruction, so a query sees memory as it was when the instruction ran.
func (graph *regionGraph) stateAt(instruction ssa.Instruction) *regionState {
	if !graph.available || instruction == nil || instruction.Parent() != graph.function {
		return nil
	}
	entry, ok := graph.entry[instruction.Block()]
	if !ok {
		return nil
	}
	state := entry.clone()
	for _, candidate := range instruction.Block().Instrs {
		if candidate == instruction {
			return state
		}
		graph.transfer(state, candidate)
	}
	return nil
}

func reversePostorder(function *ssa.Function) []*ssa.BasicBlock {
	seen := map[*ssa.BasicBlock]bool{}
	var order []*ssa.BasicBlock
	var visit func(block *ssa.BasicBlock)
	visit = func(block *ssa.BasicBlock) {
		if seen[block] {
			return
		}
		seen[block] = true
		for _, successor := range block.Succs {
			visit(successor)
		}
		order = append(order, block)
	}
	visit(function.Blocks[0])
	for i, j := 0, len(order)-1; i < j; i, j = i+1, j-1 {
		order[i], order[j] = order[j], order[i]
	}
	return order
}

// joinSlotPath appends a step to a slot path.
func joinSlotPath(path, step string) string {
	if path == "" {
		return step
	}
	return path + "/" + step
}

// slotBeneath reports whether path lies beneath prefix, or is it.
func slotBeneath(path, prefix string) bool {
	return prefix == "" || path == prefix || strings.HasPrefix(path, prefix+"/")
}

// lastStep returns the final step of a path.
func lastStep(path string) string {
	if index := strings.LastIndex(path, "/"); index >= 0 {
		return path[index+1:]
	}
	return path
}

func isIndexStep(step string) bool {
	return strings.HasPrefix(step, "index:")
}
