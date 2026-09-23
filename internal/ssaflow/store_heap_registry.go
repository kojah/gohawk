package ssaflow

import (
	"sync"

	"golang.org/x/tools/go/ssa"
)

// The registry hands heap summaries to the graph. The lifecycle pass
// registers the summary of every callee it imports before it builds any
// graph of its own package, and its own summaries once they are computed,
// so a call to a summarized function is applied by substitution instead of
// forgetting every object the caller did not allocate.

// heapSummaryLimit bounds the registry; past it the registry starts over,
// and a later lookup that misses is merely conservative.
const heapSummaryLimit = 8192

// heapEntryState is where a callee's summary stands in the registry. A
// callee being projected right now, which only a call cycle reaches again,
// has no summary yet, so the cycle is cut where it closes. One whose
// projection was not available is remembered so a call site does not ask
// again at every replay.
type heapEntryState uint8

const (
	heapEntryReady heapEntryState = iota
	heapEntryProjecting
	heapEntryMissing
)

type heapEntry struct {
	summary HeapSummary
	state   heapEntryState
}

var heapSummaries = struct {
	sync.Mutex
	entries map[*ssa.Function]heapEntry
}{entries: map[*ssa.Function]heapEntry{}}

// RegisterHeapSummary makes the summary available to every graph built
// afterwards for calls to the function.
func RegisterHeapSummary(function *ssa.Function, summary HeapSummary) {
	if function == nil {
		return
	}
	heapSummaries.Lock()
	defer heapSummaries.Unlock()
	if len(heapSummaries.entries) >= heapSummaryLimit {
		heapSummaries.entries = map[*ssa.Function]heapEntry{}
	}
	heapSummaries.entries[function] = heapEntry{summary: summary}
}

// heapSummaryOf returns the callee's summary: the registered one, or, for a
// callee whose body is in the program, one projected on demand and kept.
// An instantiation of a generic function is looked up as itself first,
// which is how the pass that summarized its package registered it, and
// then as its origin, which is how an importer names it.
func heapSummaryOf(function *ssa.Function) (HeapSummary, bool) {
	for _, candidate := range []*ssa.Function{function, ResolvedFunction(function)} {
		if candidate == nil {
			continue
		}
		if summary, ok := registeredHeapSummary(candidate); ok {
			return summary, true
		}
	}
	return projectHeapOnDemand(function)
}

// RegisteredHeapSummary returns the summary the registry holds for the
// function, for the dump; it never projects one.
func RegisteredHeapSummary(function *ssa.Function) (HeapSummary, bool) {
	heapSummaries.Lock()
	defer heapSummaries.Unlock()
	entry, ok := heapSummaries.entries[function]
	return entry.summary, ok && entry.state == heapEntryReady
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
// keeps the result. A callee already being projected, which only a call
// cycle reaches again, has no summary, so the cycle is cut where it closes.
func projectHeapOnDemand(function *ssa.Function) (HeapSummary, bool) {
	if function == nil || len(function.Blocks) == 0 {
		if resolved := ResolvedFunction(function); resolved != nil && len(resolved.Blocks) != 0 {
			function = resolved
		} else {
			return HeapSummary{}, false
		}
	}
	heapSummaries.Lock()
	if entry, ok := heapSummaries.entries[function]; ok {
		heapSummaries.Unlock()
		return entry.summary, entry.state == heapEntryReady
	}
	heapSummaries.entries[function] = heapEntry{state: heapEntryProjecting}
	heapSummaries.Unlock()
	summary, ok := ProjectHeap(function)
	if ok {
		RegisterHeapSummary(function, summary)
		return summary, true
	}
	heapSummaries.Lock()
	heapSummaries.entries[function] = heapEntry{state: heapEntryMissing}
	heapSummaries.Unlock()
	return summary, false
}
