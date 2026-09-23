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

var heapSummaries = struct {
	sync.Mutex
	entries map[*ssa.Function]HeapSummary
}{entries: map[*ssa.Function]HeapSummary{}}

// RegisterHeapSummary makes the summary available to every graph built
// afterwards for calls to the function.
func RegisterHeapSummary(function *ssa.Function, summary HeapSummary) {
	if function == nil {
		return
	}
	heapSummaries.Lock()
	defer heapSummaries.Unlock()
	if len(heapSummaries.entries) >= heapSummaryLimit {
		heapSummaries.entries = map[*ssa.Function]HeapSummary{}
	}
	heapSummaries.entries[function] = summary
}

func heapSummaryOf(function *ssa.Function) (HeapSummary, bool) {
	heapSummaries.Lock()
	defer heapSummaries.Unlock()
	summary, ok := heapSummaries.entries[ResolvedFunction(function)]
	return summary, ok
}
