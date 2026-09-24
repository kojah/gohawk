package lifecyclefacts

import (
	"strings"
	"testing"

	"github.com/kojah/gohawk/internal/heapmodel"
	"github.com/kojah/gohawk/internal/ssaflow/ssaflowtest"
)

func TestHeapTraceCountsWithoutWarmingCache(t *testing.T) {
	pkg := ssaflowtest.BuildPackage(t, "heaptrace", `package heaptrace
type user interface { Use() }
func calls(x user) {
`+strings.Repeat("x.Use()\n", 10)+"}\n")
	function := pkg.Func("calls")
	before := heapTraceDetails(function, nil)
	if before["heap-cached"] != "false" || heapmodel.CachedGraphEvidence(function).Cached {
		t.Fatal("trace collection built a graph")
	}
	// Explicitly request the graph as an ordinary consumer would, then observe.
	heapmodel.RenderRegions(function)
	after := heapTraceDetails(function, &heapmodel.HeapSummary{})
	for key, want := range map[string]string{
		"heap-cached": "true", "heap-building": "false", "heap-build-reason": "graph-build-complete",
		"heap-truncated-count": "0", "calls-interface-call": "10", "calls-applied": "0", "calls-applied-truncated": "0",
	} {
		if after[key] != want {
			t.Errorf("%s: got %q, want %q", key, after[key], want)
		}
	}
	if count := strings.Count(after["calls-unsummarized"], "[interface-call]"); count != tracedCallLimit {
		t.Fatalf("sample has %d calls, want %d (full count must remain 10)", count, tracedCallLimit)
	}
}
