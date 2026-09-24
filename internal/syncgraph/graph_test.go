package syncgraph

import (
	"testing"

	"github.com/kojah/gohawk/internal/passes/concurrencyfacts"
	"golang.org/x/tools/go/ssa"
)

func TestFromSummaryKeepsGoroutineOrderSeparate(t *testing.T) {
	summary := concurrencyfacts.Summary{
		Operations: []concurrencyfacts.Operation{
			{Kind: concurrencyfacts.Lock},
			{Kind: concurrencyfacts.Receive},
			{Kind: concurrencyfacts.Unlock},
		},
		Worker: []concurrencyfacts.Operation{{Kind: concurrencyfacts.Lock}, {Kind: concurrencyfacts.Close}},
		Spawn:  &ssa.Go{}, Prefix: 1,
	}
	graph := FromSummary(summary)
	if !graph.Complete() || len(graph.Parent) != 3 || len(graph.Child) != 2 || len(graph.Edges) != 4 {
		t.Fatalf("graph = %+v, want three parent and two worker events with four ordering edges", graph)
	}
	if graph.Parent[0].Goroutine != Root || graph.Child[0].Goroutine != Worker ||
		graph.Parent[2].ID != 2 || graph.Child[0].ID != 3 {
		t.Fatalf("event identity not preserved: parent=%+v worker=%+v", graph.Parent, graph.Child)
	}
	if graph.HasCycle() {
		t.Fatal("program order alone cannot establish a deadlock cycle")
	}
	if !graph.AddDependency(graph.Parent[2].ID, graph.Child[0].ID) ||
		!graph.AddDependency(graph.Child[1].ID, graph.Parent[1].ID) || !graph.HasCycle() {
		t.Fatal("two proven blocking dependencies should close the cycle")
	}
}

func TestIncompleteSummaryDoesNotExposePartialEvents(t *testing.T) {
	graph := FromSummary(concurrencyfacts.Summary{
		Operations: []concurrencyfacts.Operation{{Kind: concurrencyfacts.Lock}},
		Reason:     "protocol-effect-unknown",
	})
	if graph.Complete() || len(graph.Parent) != 0 || graph.AddDependency(0, 0) || graph.HasCycle() {
		t.Fatalf("incomplete summary produced usable graph: %+v", graph)
	}
}
