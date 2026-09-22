package concurrencyfacts

import (
	"sync"
	"testing"

	"github.com/kojah/gohawk/internal/ssaflow"
	"github.com/kojah/gohawk/internal/ssaflow/ssaflowtest"

	"golang.org/x/tools/go/ssa"
)

func TestConcurrentConsumers(t *testing.T) {
	pkg := ssaflowtest.BuildPackage(t, "effects", `package effects
func closeChannel(ch chan int) { close(ch) }
func forward(ch chan int) { closeChannel(ch) }
func root(ch chan int) { forward(ch) }
`)
	function := pkg.Func("root")
	calls := ssaflow.InstructionsOf[*ssa.Call](function)
	if len(calls) != 1 {
		t.Fatalf("calls = %d, want 1", len(calls))
	}
	engine := NewEngine()
	start := make(chan struct{})
	var workers sync.WaitGroup
	for worker := range 12 {
		workers.Go(func() {
			<-start
			for range 20 {
				budget := ssaflow.NewSearchBudget(2000)
				var summary Summary
				switch worker % 3 {
				case 0:
					summary = engine.Function(function, budget)
				case 1:
					summary = engine.Root(function, budget)
				case 2:
					summary = engine.AtCall(calls[0], budget)
				}
				if summary.Reason != "" || len(summary.Operations) != 1 ||
					summary.Operations[0].Kind != Close || summary.Operations[0].Resource.Value != function.Params[0] {
					t.Errorf("consumer %d: %+v", worker, summary)
				}
			}
		})
	}
	close(start)
	workers.Wait()
}
