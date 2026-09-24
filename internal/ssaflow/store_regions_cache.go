package ssaflow

import (
	"container/list"
	"slices"
	"sync"

	"golang.org/x/tools/go/ssa"
)

// The graph cache keeps each function's points-to graph for the queries
// that follow, and keeps it only while it is still the graph a fresh build
// would produce. A graph applies the heap summaries of its callees, and a
// summary can be registered after the graph was built: the lifecycle pass
// registers imported summaries when it starts, but an analyzer that does not
// depend on it may already have built graphs of the same package. Such a
// graph forgot through a call it would now substitute, and serving it later
// made one run report what another did not. So every cached graph is
// indexed by the callee summaries it consulted, and registering a summary
// that differs from what a graph saw evicts the graph. Evicting a graph also
// drops the summary projected on demand from it, and so on up the callers.
// A caller already holding an evicted graph keeps its answers; the cache
// only guarantees that no one is handed a stale graph afterwards.

// regionGraphCache keeps the most recently built graphs. Analyzers ask many
// questions about one function in a row, and the graph is built once for
// all of them; functions of packages already analyzed fall out of the cache.
const regionGraphCache = 128

var regionGraphs = struct {
	sync.Mutex
	order   *list.List
	entries map[*ssa.Function]*list.Element
	// dependents indexes the cached graphs by the callee summaries they
	// consulted.
	dependents map[*ssa.Function]map[*regionGraphEntry]bool
}{order: list.New(), entries: map[*ssa.Function]*list.Element{}, dependents: map[*ssa.Function]map[*regionGraphEntry]bool{}}

// regionGraphEntry is one cached graph. Its graph is nil while the build
// is in progress: a build that reaches the function again, through a
// callee summary projected on demand around a call cycle, gets an
// unavailable graph instead of recursing, and that summary is truncated
// where the cycle closes. An evicted entry is stale, and a build that
// finishes after its eviction is not cached.
type regionGraphEntry struct {
	function *ssa.Function
	graph    *regionGraph
	stale    bool
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
		entry := element.Value.(*regionGraphEntry) //nolint:forcetypeassert // The list holds only entries.
		// Publication and eviction both update graph under this lock. Copy
		// the pointer before releasing it; reading entry.graph afterwards
		// races with another pass finishing the same function's build.
		graph := entry.graph
		regionGraphs.Unlock()
		if graph == nil {
			return &regionGraph{building: true}
		}
		return graph
	}
	entry := &regionGraphEntry{function: function}
	regionGraphs.entries[function] = regionGraphs.order.PushFront(entry)
	regionGraphs.Unlock()
	graph := buildRegionGraph(function)
	cacheRegionGraph(entry, graph)
	return graph
}

// cacheRegionGraph stores a finished build unless it is already stale: its
// entry was evicted while it ran, or a summary it consulted has changed
// since. The check runs under the cache lock, and registration updates the
// registry before it takes that lock to evict, so a registration either
// happened before the check and is seen here, or finds the entry indexed
// and evicts it.
func cacheRegionGraph(entry *regionGraphEntry, graph *regionGraph) {
	regionGraphs.Lock()
	defer regionGraphs.Unlock()
	if entry.stale || !consultedStillCurrent(graph) {
		entry.stale = true
		if element, ok := regionGraphs.entries[entry.function]; ok && element.Value == entry {
			delete(regionGraphs.entries, entry.function)
			regionGraphs.order.Remove(element)
		}
		return
	}
	entry.graph = graph
	if element, ok := regionGraphs.entries[entry.function]; ok {
		regionGraphs.order.MoveToFront(element)
	} else {
		// The build outlived the cache's capacity; keep the graph anyway,
		// the caller is about to query it.
		regionGraphs.entries[entry.function] = regionGraphs.order.PushFront(entry)
	}
	for key := range graph.consulted {
		if regionGraphs.dependents[key] == nil {
			regionGraphs.dependents[key] = map[*regionGraphEntry]bool{}
		}
		regionGraphs.dependents[key][entry] = true
	}
	for regionGraphs.order.Len() > regionGraphCache {
		oldest := regionGraphs.order.Back()
		if oldest.Value.(*regionGraphEntry).graph == nil { //nolint:forcetypeassert // The list holds only entries.
			break
		}
		evictLocked(oldest.Value.(*regionGraphEntry)) //nolint:forcetypeassert // The list holds only entries.
	}
}

// consultedStillCurrent reports whether no summary the build consulted has
// been replaced or dropped since.
func consultedStillCurrent(graph *regionGraph) bool {
	for callee, generation := range graph.consulted {
		if heapSummaryGeneration(callee) != generation {
			return false
		}
	}
	return true
}

// summaryKeys names the registry entries a lookup of the callee can read:
// the callee's own, and for a bodiless instantiation its origin's.
func summaryKeys(callee *ssa.Function) []*ssa.Function {
	keys := []*ssa.Function{callee}
	if resolved := ResolvedFunction(callee); resolved != nil && resolved != callee {
		keys = append(keys, resolved)
	}
	return keys
}

// invalidateDependents evicts every cached graph that consulted the
// function's summary, then drops the summaries projected on demand from
// those graphs and evicts their dependents in turn.
func invalidateDependents(function *ssa.Function) {
	pending := []*ssa.Function{function}
	for len(pending) > 0 {
		next := pending[0]
		pending = pending[1:]
		regionGraphs.Lock()
		var evicted []*ssa.Function
		for entry := range regionGraphs.dependents[next] {
			evicted = append(evicted, entry.function)
			evictLocked(entry)
		}
		delete(regionGraphs.dependents, next)
		regionGraphs.Unlock()
		for _, caller := range evicted {
			if forgetOnDemandSummary(caller) {
				pending = append(pending, caller)
			}
		}
	}
}

// evictLocked removes an entry from the cache and its index. The caller
// holds the cache lock.
func evictLocked(entry *regionGraphEntry) {
	entry.stale = true
	if element, ok := regionGraphs.entries[entry.function]; ok && element.Value == entry {
		delete(regionGraphs.entries, entry.function)
		regionGraphs.order.Remove(element)
	}
	if entry.graph == nil {
		return
	}
	for key := range entry.graph.consulted {
		delete(regionGraphs.dependents[key], entry)
	}
}

// heapSummariesEqual reports whether two summaries say the same thing.
func heapSummariesEqual(left, right HeapSummary) bool {
	return slices.Equal(left.Edges, right.Edges) && slices.Equal(left.Effects, right.Effects) &&
		slices.Equal(left.Holds, right.Holds) && slices.Equal(left.Reads, right.Reads) &&
		slices.Equal(left.Requires, right.Requires) && slices.Equal(left.Truncated, right.Truncated)
}
