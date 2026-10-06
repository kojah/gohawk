package heapmodel

import (
	"sync"
	"testing"
	"time"

	"golang.org/x/tools/go/ssa"
)

// A result or lifecycle pass can finish a graph while another pass reads the
// same cache entry. Both the lookup and the published pointer must use the
// cache lock; returning a graph does not require holding the lock afterwards.
func TestRegionGraphCacheConcurrentPublication(t *testing.T) {
	function := &ssa.Function{Blocks: []*ssa.BasicBlock{{}}}
	entry := &regionGraphEntry{function: function, graph: &regionGraph{available: true}}
	regionGraphs.Lock()
	regionGraphs.entries[function] = regionGraphs.order.PushFront(entry)
	regionGraphs.Unlock()
	t.Cleanup(func() {
		regionGraphs.Lock()
		evictLocked(entry)
		regionGraphs.Unlock()
	})

	start := make(chan struct{})
	var workers sync.WaitGroup
	workers.Add(2)
	go func() {
		defer workers.Done()
		<-start
		for range 10000 {
			if graph := regionsOfFunction(function); graph == nil || !graph.available {
				t.Error("cache lookup returned no published graph")
				return
			}
		}
	}()
	go func() {
		defer workers.Done()
		<-start
		for range 10000 {
			cacheRegionGraph(entry, &regionGraph{available: true})
		}
	}()
	close(start)
	workers.Wait()
}

// A lookup that finds another analyzer's build in progress waits for it and
// returns the published graph, never an unavailable placeholder: answering
// from the placeholder made results depend on scheduling.
func TestRegionGraphLookupWaitsForBuild(t *testing.T) {
	function := &ssa.Function{Blocks: []*ssa.BasicBlock{{}}}
	entry := &regionGraphEntry{function: function, done: make(chan struct{})}
	regionGraphs.Lock()
	regionGraphs.entries[function] = regionGraphs.order.PushFront(entry)
	regionGraphs.Unlock()
	t.Cleanup(func() {
		regionGraphs.Lock()
		if element, ok := regionGraphs.entries[function]; ok {
			delete(regionGraphs.entries, function)
			regionGraphs.order.Remove(element)
		}
		regionGraphs.Unlock()
	})
	published := &regionGraph{available: true}
	looked := make(chan *regionGraph, 1)
	go func() { looked <- regionsOfFunction(function) }()
	select {
	case graph := <-looked:
		t.Fatalf("lookup returned %+v before the build finished", graph)
	case <-time.After(20 * time.Millisecond):
	}
	cacheRegionGraph(entry, published)
	if graph := <-looked; graph != published {
		t.Fatalf("lookup returned %+v, want the published graph", graph)
	}
}

func TestRegionGraphStaleBuildKeepsReplacement(t *testing.T) {
	function := &ssa.Function{Blocks: []*ssa.BasicBlock{{}}}
	entry := &regionGraphEntry{function: function, done: make(chan struct{})}
	regionGraphs.Lock()
	regionGraphs.entries[function] = regionGraphs.order.PushFront(entry)
	evictLocked(entry)
	regionGraphs.Unlock()
	select {
	case <-entry.done:
		t.Fatal("eviction finished a build that is still running")
	default:
	}
	replacement := &regionGraphEntry{function: function, graph: &regionGraph{available: true}}
	regionGraphs.Lock()
	element := regionGraphs.order.PushFront(replacement)
	regionGraphs.entries[function] = element
	regionGraphs.Unlock()
	t.Cleanup(func() {
		regionGraphs.Lock()
		evictLocked(replacement)
		regionGraphs.Unlock()
	})
	cacheRegionGraph(entry, &regionGraph{available: true})
	select {
	case <-entry.done:
	default:
		t.Fatal("rejected publication did not finish its build")
	}
	regionGraphs.Lock()
	kept := regionGraphs.entries[function] == element && element.Value == replacement && !replacement.stale
	rejected := entry.stale && entry.graph == nil
	regionGraphs.Unlock()
	if !kept || !rejected {
		t.Fatalf("stale publication changed replacement: kept=%v rejected=%v", kept, rejected)
	}
}

func TestRegionGraphRejectsChangedSummary(t *testing.T) {
	function, callee := &ssa.Function{}, &ssa.Function{}
	entry := &regionGraphEntry{function: function, done: make(chan struct{})}
	graph := &regionGraph{available: true, consulted: map[*ssa.Function]int{callee: heapSummaryGeneration(callee)}}
	regionGraphs.Lock()
	regionGraphs.entries[function] = regionGraphs.order.PushFront(entry)
	regionGraphs.Unlock()
	t.Cleanup(func() {
		regionGraphs.Lock()
		evictLocked(entry)
		delete(regionGraphs.dependents, callee)
		regionGraphs.Unlock()
		heapSummaries.Lock()
		delete(heapSummaries.entries, callee)
		delete(heapSummaries.generations, callee)
		heapSummaries.Unlock()
	})
	RegisterHeapSummary(callee, HeapSummary{})
	cacheRegionGraph(entry, graph)
	select {
	case <-entry.done:
	default:
		t.Fatal("changed-summary rejection did not finish its build")
	}
	regionGraphs.Lock()
	_, cached := regionGraphs.entries[function]
	indexed := regionGraphs.dependents[callee][entry]
	rejected := entry.stale && entry.graph == nil
	regionGraphs.Unlock()
	if cached || indexed || !rejected {
		t.Fatalf("changed-summary build was published: cached=%v indexed=%v rejected=%v", cached, indexed, rejected)
	}
}
