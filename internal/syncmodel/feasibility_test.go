package syncmodel

import (
	"testing"

	"github.com/kojah/gohawk/internal/passes/concurrencyfacts"
	"github.com/kojah/gohawk/internal/ssaflow"
	"github.com/kojah/gohawk/internal/ssaflow/ssaflowtest"
)

// Each case is one root whose path alternatives are judged by Feasibility.
// The counts say how many alternatives must be feasible, contradictory, and
// unknown under the independence policy.
func TestPathFeasibility(t *testing.T) {
	pkg := ssaflowtest.BuildPackage(t, "feasible", `package feasible
import "sync"
type state struct{ debug bool }
func check(n int) error { return nil }
func other(n int) error { return nil }
func optional(mu *sync.Mutex, flag bool) { if flag { mu.Lock(); mu.Unlock() } }
func correlated(mu *sync.Mutex, flag bool) { if flag { mu.Lock() }; if flag { mu.Unlock() } }
func independentFlags(mu *sync.Mutex, a, b bool) { if a { mu.Lock() }; if b { mu.Unlock() } }
func errorPath(done chan int, n int) { if err := check(n); err != nil { return }; close(done) }
func twoCallees(mu *sync.Mutex) { if check(1) != nil { mu.Lock() }; if other(2) != nil { mu.Unlock() } }
func sameCallee(mu *sync.Mutex) { if check(1) != nil { mu.Lock() }; if check(2) != nil { mu.Unlock() } }
func constants(mu *sync.Mutex, n int) { if n == 1 { mu.Lock() }; if n == 2 { mu.Unlock() } }
func memory(mu *sync.Mutex, s *state) { if s.debug { mu.Lock() }; if s.debug { mu.Unlock() } }
func memoryAlone(mu *sync.Mutex, s *state) { if s.debug { mu.Lock(); mu.Unlock() } }
func memoryAndParameter(mu *sync.Mutex, s *state, flag bool) { if s.debug { mu.Lock() }; if flag { mu.Unlock() } }
func pick(a, b chan int, flag bool) { if flag { close(a) } else { close(b) } }
func workerFlag(a, b chan int, mu *sync.Mutex, flag bool) { go pick(a, b, flag); if flag { mu.Lock(); mu.Unlock() } }
func pickN(a, b chan int, n int) { if n == 1 { close(a) } else { close(b) } }
func comparedTwice(a, b chan int, x int) { pickN(a, b, x); pickN(a, b, x) }
func comparedConstant(a, b chan int) { pickN(a, b, 1) }
func pickF(a, b chan int, f func()) { if f != nil { close(a) } else { close(b) } }
func nilCallback(a, b chan int) { pickF(a, b, nil) }
`)
	engine := concurrencyfacts.NewEngine()
	for name, want := range map[string][3]int{
		"optional":         {2, 0, 0},
		"correlated":       {2, 2, 0},
		"independentFlags": {4, 0, 0},
		"errorPath":        {2, 0, 0},
		"twoCallees":       {4, 0, 0},
		"sameCallee":       {0, 0, 4},
		"constants":        {3, 1, 0},
		// Two reads of one field are one loaded guard: agreeing is feasible,
		// disagreeing is unknown because a store could explain it.
		"memory":             {2, 0, 2},
		"memoryAlone":        {2, 0, 0},
		"memoryAndParameter": {0, 0, 4},
		// The worker's test of its parameter binds to the parent's flag, so
		// agreeing combinations are feasible and disagreeing ones are not.
		"workerFlag": {2, 2, 0},
		// A helper's comparison of its parameter binds to the caller's value:
		// the same x twice must agree, and a constant argument folds.
		"comparedTwice":    {2, 2, 0},
		"comparedConstant": {1, 1, 0},
		"nilCallback":      {1, 1, 0},
	} {
		t.Run(name, func(t *testing.T) {
			summary := engine.Root(pkg.Func(name), ssaflow.NewSearchBudget(4000))
			graphs, failure := Expand(summary)
			if !failure.Empty() {
				t.Fatalf("expand failed: %s (%+v)", failure, summary)
			}
			var got [3]int
			for _, graph := range graphs {
				switch proof := graph.Feasibility(); proof.State {
				case ssaflow.EvidenceProven:
					got[0]++
				case ssaflow.EvidenceDisproven:
					got[1]++
				default:
					got[2]++
				}
			}
			if got != want {
				t.Errorf("feasible/contradictory/unknown = %v, want %v", got, want)
			}
		})
	}
}
