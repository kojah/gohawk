package goroutineownership

import (
	"testing"

	proofs "github.com/kojah/gohawk/internal/proof"
	"github.com/kojah/gohawk/internal/ssaflow"
	"github.com/kojah/gohawk/internal/ssaflow/ssaflowtest"
	"golang.org/x/tools/go/ssa"
)

func TestCompletionDiscoveryNilAllowance(t *testing.T) {
	pkg := ssaflowtest.BuildPackage(t, "nilallowance", `package nilallowance
import "sync"
func signal(done chan int) { done <- 1 }
func direct(group *sync.WaitGroup) { group.Done() }
func deferred(group *sync.WaitGroup) { defer group.Done() }
func nilSignal() { go signal(nil) }
func nilDirect() { go direct(nil) }
func nilDeferred() { go deferred(nil) }
func liveSignal() { go signal(make(chan int)) }
`)
	for _, test := range []struct {
		name       string
		beforeFold int
		signals    int
	}{
		{"nilSignal", 11, 0}, {"nilDirect", 10, 0}, {"nilDeferred", 14, 0}, {"liveSignal", 11, 1},
	} {
		t.Run(test.name, func(t *testing.T) {
			fn := pkg.Func(test.name)
			spawn := ssaflow.InstructionsOf[*ssa.Go](fn)[0]
			t.Log(spawn.String())
			// The parent completed these actual SSA queries at these allowances
			// without charging its nil folds. They now cut before discovery can
			// publish even an already found completion handle.
			pool := proofs.NewSearchBudget(spawnPoolBudget)
			child := pool.Within(test.beforeFold)
			signals, groups, _ := spawnedCompletionValues(nil, spawn, child)
			if !child.Exhausted() || pool.Exhausted() {
				t.Fatal("nil query bypassed its child allowance or exhausted the outer pool")
			}
			candidate := &spawnAnalysis{function: fn, spawn: spawn, signals: signals, groups: groups, discoveryBudget: child}
			got := candidate.prove()
			if got.Outcome != GoroutineUnknown || got.Reason != reasonDiscoveryBudgetExhausted {
				t.Fatalf("nil evidence bypassed the discovery allowance: %+v, exhausted %v/%v", got, child.Exhausted(), pool.Exhausted())
			}
			complete := false
			for limit := 0; limit <= proofs.SummaryBudget; limit++ {
				budget := proofs.NewSearchBudget(limit)
				signals, groups, _ := spawnedCompletionValues(nil, spawn, budget)
				if budget.Exhausted() {
					continue
				}
				if len(signals) != test.signals || len(groups) != 0 {
					t.Fatalf("fresh nil policy changed: signals=%d groups=%d", len(signals), len(groups))
				}
				complete = true
				break
			}
			if !complete {
				t.Fatal("fresh completion discovery never finished after cutoff")
			}
		})
	}
}
