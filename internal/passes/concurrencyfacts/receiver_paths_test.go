package concurrencyfacts

import (
	"testing"

	"github.com/kojah/gohawk/internal/ssaflow"
	"github.com/kojah/gohawk/internal/ssaflow/ssaflowtest"
	"golang.org/x/tools/go/ssa"
)

func TestForwardedReceiverFieldsKeepInvocationIdentity(t *testing.T) {
	pkg := ssaflowtest.BuildPackage(t, "receivers", `package receivers
import "sync"
type owner struct { mu sync.RWMutex }
func (o *owner) run(done chan int) { o.mu.RLock(); close(done); o.mu.RUnlock() }
func (o *owner) Start(done chan int) { go o.run(done) }
func root() {
 var a, b owner
 x, y := make(chan int), make(chan int)
 a.mu.Lock(); b.mu.Lock()
 a.Start(x); b.Start(y)
 <-x; <-y
 a.mu.Unlock(); b.mu.Unlock()
}
`)
	engine := NewEngine()
	result := engine.Root(pkg.Func("root"), ssaflow.NewSearchBudget(ssaflow.SummaryBudget))
	if !result.Complete() || len(result.Workers) != 2 {
		t.Fatalf("root = %+v", result)
	}
	for index, worker := range result.Workers {
		if len(worker.Operations) != 3 || worker.Operations[0].Kind != ReadLock || worker.Operations[2].Kind != ReadUnlock {
			t.Fatalf("worker %d = %+v", index, worker)
		}
		if worker.Operations[0].Resource != result.Operations[index].Resource {
			t.Errorf("worker %d bound to a different receiver", index)
		}
	}
	if result.Workers[0].Operations[0].Resource == result.Workers[1].Operations[0].Resource {
		t.Error("two invocations shared a receiver identity")
	}
	// The launch method does not itself select mu. Its field relation must
	// still be publishable relative to its receiver, without an invented SSA cell.
	for _, call := range ssaflow.InstructionsOf[*ssa.Call](pkg.Func("root")) {
		function := call.Common().StaticCallee()
		if function != nil && function.Name() == "Start" {
			assertReceiverWorkerFact(t, engine, function)
		}
	}
}

func assertReceiverWorkerFact(t *testing.T, engine *Engine, function *ssa.Function) {
	t.Helper()
	declaration := engine.Function(function, ssaflow.NewSearchBudget(ssaflow.SummaryBudget))
	fact, exported := exportSummary(function, declaration)
	if !exported || len(fact.Workers) != 1 || len(fact.Workers[0].Effects) != 3 {
		t.Fatalf("receiver declaration was not exported: %+v (%+v)", fact, declaration)
	}
	first := fact.Workers[0].Effects[0]
	if first.Parameter != 0 || first.Kind != ReadLock || len(first.Fields) != 1 || first.Fields[0] != 0 {
		t.Errorf("exported field relation = %+v", first)
	}
}
