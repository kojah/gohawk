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
		Workers: []concurrencyfacts.WorkerSummary{{
			Operations: []concurrencyfacts.Operation{{Kind: concurrencyfacts.Lock}, {Kind: concurrencyfacts.Close}},
			Spawn:      &ssa.Go{}, Prefix: 1,
		}},
	}
	graph := FromSummary(summary)
	if !graph.Complete() || len(graph.Parent) != 3 || len(graph.Children) != 1 ||
		len(graph.Children[0].Events) != 2 || len(graph.Edges) != 4 {
		t.Fatalf("graph = %+v, want three parent and two worker events with four ordering edges", graph)
	}
	child := graph.Children[0].Events
	if graph.Parent[0].Goroutine != Root || child[0].Goroutine != 1 ||
		graph.Parent[2].ID != 2 || child[0].ID != 3 {
		t.Fatalf("event identity not preserved: parent=%+v child=%+v", graph.Parent, child)
	}
	if graph.HasCycle() {
		t.Fatal("program order alone cannot establish a deadlock cycle")
	}
	if !graph.AddDependency(graph.Parent[2].ID, child[0].ID) ||
		!graph.AddDependency(child[1].ID, graph.Parent[1].ID) || !graph.HasCycle() {
		t.Fatal("two proven blocking dependencies should close the cycle")
	}
}

func TestFromSummaryKeepsChildrenDistinct(t *testing.T) {
	summary := concurrencyfacts.Summary{
		Operations: []concurrencyfacts.Operation{{Kind: concurrencyfacts.Lock}, {Kind: concurrencyfacts.Receive}},
		Workers: []concurrencyfacts.WorkerSummary{
			{Spawn: &ssa.Go{}, Prefix: 1, Operations: []concurrencyfacts.Operation{{Kind: concurrencyfacts.Lock}, {Kind: concurrencyfacts.Close}}},
			{Spawn: &ssa.Go{}, Prefix: 2, Operations: []concurrencyfacts.Operation{{Kind: concurrencyfacts.Send}, {Kind: concurrencyfacts.Close}}},
		},
	}
	graph := FromSummary(summary)
	if !graph.Complete() || len(graph.Children) != 2 || len(graph.Edges) != 5 {
		t.Fatalf("multi-child graph = %+v", graph)
	}
	first, second := graph.Children[0], graph.Children[1]
	if first.Events[0].Goroutine != 1 || second.Events[0].Goroutine != 2 ||
		first.Events[0].ID != 2 || second.Events[0].ID != 4 || first.Prefix != 1 || second.Prefix != 2 {
		t.Fatalf("child identities or launch points lost: first=%+v second=%+v", first, second)
	}
	for _, edge := range graph.Edges {
		if edge.Before == first.Events[1].ID && edge.After == second.Events[0].ID {
			t.Fatal("children inherited order from one another")
		}
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
