package syncmodel

import (
	"testing"

	"github.com/kojah/gohawk/internal/passes/concurrencyfacts"
	"github.com/kojah/gohawk/internal/ssaflow"
	"github.com/kojah/gohawk/internal/ssaflow/ssaflowtest"
)

func TestResourceScopePreservesParticipantsAndLaunchOrder(t *testing.T) {
	pkg := ssaflowtest.BuildPackage(t, "scope", `package scope
import "sync"
func root() {
 var mu, other sync.Mutex
 done := make(chan int)
 mu.Lock(); other.Lock(); other.Unlock()
 go func(){ mu.Lock(); close(done); mu.Unlock() }()
 go func(){}()
 <-done; mu.Unlock()
}
`)
	summary := concurrencyfacts.NewEngine().Root(pkg.Func("root"), ssaflow.NewSearchBudget(ssaflow.SummaryBudget))
	graph := FromSummary(summary)
	if !graph.Complete() || len(graph.Parent) != 5 {
		t.Fatalf("input graph = %+v", graph)
	}
	resources := []concurrencyfacts.Reference{graph.Parent[0].Resource, graph.Parent[3].Resource}
	scoped := graph.Scope(resources...)
	if !scoped.Complete() || len(scoped.Parent) != 3 || len(scoped.Children) != 2 {
		t.Fatalf("scope = %+v", scoped)
	}
	for _, child := range scoped.Children {
		if child.Prefix != 1 {
			t.Errorf("launch prefix = %d, want 1", child.Prefix)
		}
	}
	if len(graph.Parent) != 5 || graph.Children[0].Prefix != 3 {
		t.Error("scope mutated the input")
	}
	graph.Failure = summaryFailure(concurrencyfacts.ReasonEffectUnknown)
	if scoped := graph.Scope(resources...); scoped.Complete() {
		t.Error("projection repaired an incomplete participant model")
	}
}

func TestScopeCannotEraseUnknownIdentityOrDependencies(t *testing.T) {
	pkg := ssaflowtest.BuildPackage(t, "unknownscope", `package unknownscope
import "sync"
func f(a, b *sync.Mutex) { a.Lock(); b.Unlock() }
`)
	function := pkg.Func("f")
	summary := concurrencyfacts.NewEngine().Root(function, ssaflow.NewSearchBudget(ssaflow.SummaryBudget))
	graph := FromSummary(summary)
	if !graph.Complete() {
		t.Fatalf("input = %+v", graph)
	}
	if scoped := graph.Scope(graph.Parent[0].Resource); scoped.Complete() {
		t.Error("possibly aliased unlock was discarded")
	}
	if FreshResource(graph.Parent[0].Resource).Proven() {
		t.Error("external parameter was treated as fresh")
	}
	graph.AddDependency(0, 1)
	if scoped := graph.Scope(graph.Parent[0].Resource, graph.Parent[1].Resource); scoped.Complete() {
		t.Error("dependency graph was silently rebuilt")
	}
}
