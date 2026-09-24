package heapmodel

import (
	"sync"
	"testing"

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
