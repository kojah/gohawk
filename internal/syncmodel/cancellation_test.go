package syncmodel

import (
	"testing"

	"github.com/kojah/gohawk/internal/passes/concurrencyfacts"
	"github.com/kojah/gohawk/internal/ssaflow"
	"github.com/kojah/gohawk/internal/ssaflow/ssaflowtest"
)

func TestCancellationRelationshipsAreNotOrderEdges(t *testing.T) {
	pkg := ssaflowtest.BuildPackage(t, "cancellation", `package cancellation
import "context"
func wait(ctx context.Context) { <-ctx.Done() }
func root() {
 ctx, cancel := context.WithCancel(context.Background())
 other, cancelOther := context.WithCancel(context.Background())
 go wait(ctx)
 go wait(ctx)
 go wait(other)
 cancel()
 cancelOther()
}
`)
	result := concurrencyfacts.NewEngine().Root(pkg.Func("root"), ssaflow.NewSearchBudget(2000))
	graph := FromSummary(result)
	if !graph.Complete() || len(graph.Cancellations) != 2 {
		t.Fatalf("graph = %+v", graph)
	}
	first, second := graph.Cancellations[0], graph.Cancellations[1]
	if first.Request != graph.Parent[0].ID || len(first.Receives) != 2 ||
		first.Receives[0] != graph.Children[0].Events[0].ID || first.Receives[1] != graph.Children[1].Events[0].ID ||
		second.Request != graph.Parent[1].ID || len(second.Receives) != 1 || second.Receives[0] != graph.Children[2].Events[0].ID {
		t.Errorf("wrong cancellation relationships: %+v", graph.Cancellations)
	}
	// Only the parent's two requests have program order. There is no join,
	// prerequisite, or happens-before edge from a request to either worker.
	if len(graph.Edges) != 1 || graph.Edges[0].Kind != ProgramOrder || graph.HasCycle() {
		t.Errorf("cancellation invented order: %+v", graph.Edges)
	}
}

func TestCancellationSelectExpansionRequiresExactOrigins(t *testing.T) {
	pkg := ssaflowtest.BuildPackage(t, "cancelchoice", `package cancelchoice
import "context"
func worker(ctx context.Context, out chan int) {
 select { case out <- 1: case <-ctx.Done(): }
}
func unknown(ctx context.Context, out chan int) { go worker(ctx, out) }
func root() {
 ctx, cancel := context.WithCancel(context.Background())
 out := make(chan int)
 go worker(ctx, out)
 cancel()
}
`)
	engine := concurrencyfacts.NewEngine()
	unknown := engine.Root(pkg.Func("unknown"), ssaflow.NewSearchBudget(2000))
	if graphs, reason := Expand(unknown); len(graphs) != 0 || reason != summaryFailure(concurrencyfacts.ReasonContextBindingRequired) {
		t.Fatalf("unbound alternatives expanded: %+v, %s", graphs, reason)
	}
	root := engine.Root(pkg.Func("root"), ssaflow.NewSearchBudget(2000))
	graphs, reason := Expand(root)
	if !reason.Empty() || len(graphs) != 2 {
		t.Fatalf("bound alternatives = %+v, %s", graphs, reason)
	}
	if len(graphs[0].Cancellations) != 1 || len(graphs[0].Cancellations[0].Receives) != 0 ||
		len(graphs[1].Cancellations) != 1 || len(graphs[1].Cancellations[0].Receives) != 1 {
		t.Errorf("mutually exclusive cancellation observations conflated: %+v", graphs)
	}
}
