package heapmodel

import (
	"testing"

	"github.com/kojah/gohawk/internal/ssaflow/ssaflowtest"
)

func TestCachedGraphEvidenceDoesNotBuild(t *testing.T) {
	pkg := ssaflowtest.BuildPackage(t, "observation", `package observation
func untouched() *int { return new(int) }
`)
	function := pkg.Func("untouched")
	if got := CachedGraphEvidence(function); got.Cached || got.BuildReason != GraphBuildUnknown || len(got.Calls) != 0 {
		t.Fatalf("observation built a graph: %+v", got)
	}
	if allocations := testing.AllocsPerRun(100, func() { CachedGraphEvidence(function) }); allocations != 0 {
		t.Fatalf("cache miss allocated: %v", allocations)
	}
	regionGraphs.Lock()
	_, found := regionGraphs.entries[function]
	regionGraphs.Unlock()
	if found {
		t.Fatal("observation published a graph")
	}
}

func TestCachedGraphEvidenceDeduplicatesAndCopies(t *testing.T) {
	pkg := ssaflowtest.BuildPackage(t, "observation", `package observation
func opaque(*int)
func body() { p := new(int); opaque(p) }
`)
	function := pkg.Func("body")
	graph := regionsOfFunction(function)
	if len(graph.applied) != 1 {
		t.Fatalf("expected opaque call record, got %d", len(graph.applied))
	}
	instruction := graph.applied[0].instruction
	// Two visits of one site are one loss; a different slot is another.
	graph.widened = []widening{
		{target: slot{region: graph.unkR}, at: instruction, size: 33},
		{target: slot{region: graph.unkR}, at: instruction, size: 34},
		{target: slot{region: graph.nilR}, at: instruction, size: 33},
	}
	first := CachedGraphEvidence(function)
	if !first.Cached || first.Building || first.BuildReason != GraphBuildComplete || first.Widenings != 2 || len(first.Calls) != 1 {
		t.Fatalf("unexpected snapshot: %+v", first)
	}
	want := first.Calls[0].Reason
	first.Calls[0].Reason = CallApplicationUnknown
	second := CachedGraphEvidence(function)
	if second.Calls[0].Reason != want || second.Widenings != first.Widenings {
		t.Fatal("snapshot mutation or repeated reads changed authoritative evidence")
	}
}

func TestCachedGraphEvidenceBuilding(t *testing.T) {
	pkg := ssaflowtest.BuildPackage(t, "building", `package building
func body() {}
`)
	function := pkg.Func("body")
	regionGraphs.Lock()
	element := regionGraphs.order.PushFront(&regionGraphEntry{function: function})
	regionGraphs.entries[function] = element
	regionGraphs.Unlock()
	t.Cleanup(func() {
		regionGraphs.Lock()
		delete(regionGraphs.entries, function)
		regionGraphs.order.Remove(element)
		regionGraphs.Unlock()
	})
	if got := CachedGraphEvidence(function); !got.Cached || !got.Building || got.BuildReason != GraphBuildUnknown {
		t.Fatalf("in-progress graph presented as complete: %+v", got)
	}
}
