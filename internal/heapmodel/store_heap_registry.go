package heapmodel

import (
	"sync"

	"github.com/kojah/gohawk/internal/ssaflow"
	"golang.org/x/tools/go/ssa"
)

// The registry hands heap summaries to the graph. The lifecycle pass
// registers the summary of every callee it imports before it builds any
// graph of its own package, and its own summaries once they are computed,
// so a call to a summarized function is applied by substitution instead of
// forgetting every object the caller did not allocate.

// heapSummaryLimit bounds the registry. Past it nothing more is registered
// and a later lookup that misses is merely conservative. The registry never
// forgets what it holds: a summary that came and went with the order of
// requests would make the same package analyze differently from one run
// to the next.
const heapSummaryLimit = 131072

// heapEntryState is where a callee's summary stands in the registry. A
// callee being projected right now can be requested by another analyzer;
// that request projects privately instead of treating contention as recursion.
// Graph construction cuts call cycles before requesting a callee summary.
// An unavailable projection is remembered to avoid retrying at every replay.
type heapEntryState uint8

const (
	heapEntryReady heapEntryState = iota
	heapEntryProjecting
	heapEntryMissing
)

// An entry projected on demand came from the function's own graph, and is
// dropped when that graph is found stale; a registered one stands until it
// is registered again.
type heapEntry struct {
	summary  HeapSummary
	state    heapEntryState
	onDemand bool
}

// generations counts, per function, the registrations that changed its
// summary and the on-demand summaries dropped. A graph remembers the
// generation of each summary it consulted; a projection finishing on
// demand, which only fills in what a cycle cut, does not count, so graphs
// around a call cycle are not rebuilt forever.
var heapSummaries = struct {
	sync.Mutex
	entries     map[*ssa.Function]heapEntry
	generations map[*ssa.Function]int
}{entries: map[*ssa.Function]heapEntry{}, generations: map[*ssa.Function]int{}}

// RegisterHeapSummary makes the summary available to every graph built
// afterwards for calls to the function. A summary that differs from what
// the registry held evicts the cached graphs that consulted the old one.
func RegisterHeapSummary(function *ssa.Function, summary HeapSummary) {
	if function == nil {
		return
	}
	heapSummaries.Lock()
	previous, ok := heapSummaries.entries[function]
	if !ok && len(heapSummaries.entries) >= heapSummaryLimit {
		heapSummaries.Unlock()
		return
	}
	heapSummaries.entries[function] = heapEntry{summary: summary}
	changed := !ok || previous.state != heapEntryReady || !heapSummariesEqual(previous.summary, summary)
	if changed {
		heapSummaries.generations[function]++
	}
	heapSummaries.Unlock()
	if changed {
		invalidateDependents(function)
	}
}

// heapSummaryGeneration returns how many times the function's summary has
// been replaced or dropped.
func heapSummaryGeneration(function *ssa.Function) int {
	heapSummaries.Lock()
	defer heapSummaries.Unlock()
	return heapSummaries.generations[function]
}

// forgetOnDemandSummary drops a summary projected from a graph that was
// found stale, and reports whether there was one to drop.
func forgetOnDemandSummary(function *ssa.Function) bool {
	heapSummaries.Lock()
	defer heapSummaries.Unlock()
	entry, ok := heapSummaries.entries[function]
	if !ok || !entry.onDemand {
		return false
	}
	delete(heapSummaries.entries, function)
	heapSummaries.generations[function]++
	return true
}

// heapSummaryOf returns the callee's summary: the registered one, or, for a
// callee whose body is in the program, one projected on demand and kept.
// An instantiation of a generic function is answered from its own body
// whenever it has one, never from its origin: the origin's body is typed
// over type parameters and projects far less, and which of the two was
// registered first must not decide what a caller sees. The origin answers
// only for an instantiation the program did not build.
func heapSummaryOf(function *ssa.Function) (HeapSummary, bool) {
	if function == nil {
		return HeapSummary{}, false
	}
	if summary, ok := registeredHeapSummary(function); ok {
		return summary, true
	}
	if len(function.Blocks) != 0 {
		return projectHeapOnDemand(function)
	}
	resolved := ssaflow.ResolvedFunction(function)
	if summary, ok := registeredHeapSummary(resolved); ok {
		return summary, true
	}
	return projectHeapOnDemand(resolved)
}

// RegisteredHeapSummary returns the registry's ready summary for the function.
// It never projects a summary and reports no answer while projection is in
// progress or when it was unavailable.
func RegisteredHeapSummary(function *ssa.Function) (HeapSummary, bool) {
	return registeredHeapSummary(function)
}

// registeredHeapSummary reports the registry's answer for one function:
// the summary when it is ready, and no answer while it is being projected
// or when its projection was found unavailable.
func registeredHeapSummary(function *ssa.Function) (HeapSummary, bool) {
	heapSummaries.Lock()
	defer heapSummaries.Unlock()
	entry, ok := heapSummaries.entries[function]
	return entry.summary, ok && entry.state == heapEntryReady
}

// projectHeapOnDemand projects a callee whose body is in the program and
// keeps the result. A graph never applies a summary from its own call cycle
// (sameCallCycle), so a callee found mid-projection is being projected by
// another analyzer, not reached again on this stack. Treating that as a cut
// gave the caller no summary whenever two analyzers asked at once, and the
// same package was reported differently from run to run; the callee is
// projected privately instead, which yields the summary the other analyzer
// is about to register.
func projectHeapOnDemand(function *ssa.Function) (HeapSummary, bool) {
	if function == nil || len(function.Blocks) == 0 {
		return HeapSummary{}, false
	}
	heapSummaries.Lock()
	if entry, ok := heapSummaries.entries[function]; ok {
		heapSummaries.Unlock()
		if entry.state == heapEntryProjecting {
			return ProjectHeap(function)
		}
		return entry.summary, entry.state == heapEntryReady
	}
	heapSummaries.entries[function] = heapEntry{state: heapEntryProjecting}
	heapSummaries.Unlock()
	summary, ok := ProjectHeap(function)
	if ok {
		heapSummaries.Lock()
		heapSummaries.entries[function] = heapEntry{summary: summary, onDemand: true}
		heapSummaries.Unlock()
		return summary, true
	}
	heapSummaries.Lock()
	heapSummaries.entries[function] = heapEntry{state: heapEntryMissing}
	heapSummaries.Unlock()
	return summary, false
}
