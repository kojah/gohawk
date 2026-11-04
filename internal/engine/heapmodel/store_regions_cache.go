package heapmodel

import (
	"container/list"
	"sync"

	ssacall "github.com/kojah/gohawk/internal/engine/ssaflow/calls"
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

// regionGraphCacheState is one program's graph cache.
type regionGraphCacheState struct {
	sync.Mutex
	order   *list.List
	entries map[*ssa.Function]*list.Element
	// dependents indexes the cached graphs by the callee summaries they
	// consulted.
	dependents map[*ssa.Function]map[*regionGraphEntry]bool
}

// regionGraphEntry is one cached graph. Its graph is nil while the build
// is in progress, and concurrent lookups wait for done before reading it.
// Graph construction cuts callee-summary call cycles before they can ask
// for the same graph. An evicted entry is stale, and a build that finishes
// after its eviction is not cached.
type regionGraphEntry struct {
	function *ssa.Function
	graph    *regionGraph
	stale    bool
	// done is closed once the build that created the entry finishes, whether
	// it was published or found stale.
	done chan struct{}
}

// finishLocked marks the entry's build finished, waking every lookup that is
// waiting for it. It runs under the cache lock.
func (entry *regionGraphEntry) finishLocked() {
	if entry.done == nil {
		return
	}
	select {
	case <-entry.done:
	default:
		close(entry.done)
	}
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
	regionGraphs := &cachesOf(function).graphs
	for {
		regionGraphs.Lock()
		element, ok := regionGraphs.entries[function]
		if !ok {
			break
		}
		entry := element.Value.(*regionGraphEntry) //nolint:forcetypeassert // The list holds only entries.
		// Publication and eviction both update graph under this lock. Copy
		// the pointer before releasing it; reading entry.graph afterwards
		// races with another pass finishing the same function's build.
		graph, done := entry.graph, entry.done
		regionGraphs.Unlock()
		if graph != nil {
			return graph
		}
		// Another analyzer is building this graph. Answering from an
		// unavailable placeholder meanwhile gave weaker, structural answers
		// whose presence depended on scheduling, so the same package could be
		// reported differently from run to run. Wait for that build instead:
		// a build never asks for its own function's graph, so the builder is
		// always another goroutine. A build found stale is not published, and
		// the loop then builds the graph here.
		if done == nil {
			return &regionGraph{building: true}
		}
		<-done
	}
	entry := &regionGraphEntry{function: function, done: make(chan struct{})}
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
	regionGraphs := &cachesOf(entry.function).graphs
	regionGraphs.Lock()
	defer regionGraphs.Unlock()
	defer entry.finishLocked()
	if entry.stale || !consultedStillCurrent(graph) {
		regionGraphs.removeIndexedEntryLocked(entry)
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
		regionGraphs.evictLocked(oldest.Value.(*regionGraphEntry)) //nolint:forcetypeassert // The list holds only entries.
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
	if resolved := ssacall.ResolvedFunction(callee); resolved != nil && resolved != callee {
		keys = append(keys, resolved)
	}
	return keys
}

// invalidateDependents evicts every cached graph that consulted the
// function's summary, then drops the summaries projected on demand from
// those graphs and evicts their dependents in turn.
func invalidateDependents(function *ssa.Function) {
	regionGraphs := &cachesOf(function).graphs
	pending := []*ssa.Function{function}
	for len(pending) > 0 {
		next := pending[0]
		pending = pending[1:]
		regionGraphs.Lock()
		var evicted []*ssa.Function
		for entry := range regionGraphs.dependents[next] {
			evicted = append(evicted, entry.function)
			regionGraphs.evictLocked(entry)
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

// staleRebuildLimit bounds the rebuilds currentRegionGraph makes while
// registrations keep changing what a build consulted. Registrations are
// finite, so a build soon finds the registry settled.
const staleRebuildLimit = 4

// currentRegionGraph returns the function's graph built against the
// summaries the registry holds now. A build that consulted a summary
// registered while it ran is stale: the cache declines to keep it, but its
// caller still has it, and a summary projected from it is the older answer
// for good, because nothing changes the generation that projection's
// callers consulted. Such a graph is rebuilt instead of projected.
// https://github.com/kojah/gohawk/issues/69
func currentRegionGraph(function *ssa.Function) *regionGraph {
	for attempt := 0; ; attempt++ {
		graph := regionsOfFunction(function)
		// A graph still being built by another analyzer is not available
		// yet. A projection never reaches its own function again on one
		// stack, because a graph applies no summary from its own call
		// cycle, so the summary is taken from a private build instead,
		// which is the same graph.
		if graph.building {
			graph = buildRegionGraph(function)
		}
		if !graph.available || attempt == staleRebuildLimit || consultedStillCurrent(graph) {
			return graph
		}
		evictRegionGraph(function, graph)
	}
}

// evictRegionGraph evicts the function's cached graph if it is the given
// one, so the next request builds afresh. A graph the cache declined to
// keep is not cached, and the next request builds afresh anyway.
func evictRegionGraph(function *ssa.Function, graph *regionGraph) {
	regionGraphs := &cachesOf(function).graphs
	regionGraphs.Lock()
	defer regionGraphs.Unlock()
	if element, ok := regionGraphs.entries[function]; ok {
		if entry := element.Value.(*regionGraphEntry); entry.graph == graph { //nolint:forcetypeassert // The list holds only entries.
			regionGraphs.evictLocked(entry)
		}
	}
}

// evictLocked removes an entry from the cache and its index. The caller
// holds the cache lock.
func (regionGraphs *regionGraphCacheState) evictLocked(entry *regionGraphEntry) {
	regionGraphs.removeIndexedEntryLocked(entry)
	if entry.graph == nil {
		return
	}
	for key := range entry.graph.consulted {
		delete(regionGraphs.dependents[key], entry)
	}
}

// removeIndexedEntryLocked marks an entry stale and unlinks only its own
// cache slot. An older build must not remove its replacement. The caller holds
// the cache lock and separately owns dependency cleanup and build notification:
// eviction does not finish a build that is still running.
func (regionGraphs *regionGraphCacheState) removeIndexedEntryLocked(entry *regionGraphEntry) {
	entry.stale = true
	if element, ok := regionGraphs.entries[entry.function]; ok && element.Value == entry {
		delete(regionGraphs.entries, entry.function)
		regionGraphs.order.Remove(element)
	}
}
