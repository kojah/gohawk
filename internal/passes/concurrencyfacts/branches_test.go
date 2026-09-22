package concurrencyfacts

import (
	"testing"

	"github.com/kojah/gohawk/internal/ssaflow"
	"github.com/kojah/gohawk/internal/ssaflow/ssaflowtest"
)

func TestEquivalentBranches(t *testing.T) {
	pkg := ssaflowtest.BuildPackage(t, "branches", `package branches
func different(a, b chan int, flag bool) { if flag { close(a) } else { close(b) } }
func optional(a chan int, flag bool) { if flag { close(a) } }
func reordered(a, b chan int, flag bool) {
 if flag { close(a); close(b) } else { close(b); close(a) }
}
func loop(a chan int, flag bool) { for flag { a <- 1 } }
func opaque(a chan int, flag bool, f func()) { if flag { close(a) } else { f(); close(a) } }
func deferredDifferent(a chan int, flag bool) { if flag { defer close(a) } else { close(a) } }
func same(a chan int, flag bool) { if flag { a <- 1 } else { a <- 2 }; close(a) }
func early(a chan int, flag bool) { if flag { close(a); return }; close(a) }
func deferred(a chan int, flag bool) { defer close(a); if flag { a <- 1 } else { a <- 2 } }
func scalar(a chan int, flag bool) int { n := 1; if flag { n = 2 }; close(a); return n }
func diamonds(a chan int, x, y bool) {
 if x { a <- 1 } else { a <- 2 }; if y { close(a) } else { close(a) }
}
`)
	for _, name := range []string{"different", "optional", "reordered", "loop", "opaque", "deferredDifferent"} {
		t.Run(name, func(t *testing.T) {
			result := NewEngine().Function(pkg.Func(name), ssaflow.NewSearchBudget(2000))
			if result.Reason == "" || len(result.Operations) != 0 {
				t.Fatalf("incomplete branches produced effects: %+v", result)
			}
		})
	}
	for name, count := range map[string]int{"same": 2, "early": 1, "deferred": 2, "scalar": 1, "diamonds": 2} {
		t.Run(name, func(t *testing.T) {
			function := pkg.Func(name)
			result := NewEngine().Function(function, ssaflow.NewSearchBudget(2000))
			if result.Reason != "" || len(result.Operations) != count {
				t.Fatalf("equivalent effects lost: %+v", result)
			}
			for _, operation := range result.Operations {
				if operation.Resource.Value != function.Params[0] {
					t.Errorf("wrong resource: %+v", operation)
				}
			}
		})
	}
	engine := NewEngine()
	if result := engine.Function(pkg.Func("diamonds"), ssaflow.NewSearchBudget(1)); result.Reason != "protocol-budget-exhausted" {
		t.Fatalf("small branch budget: %+v", result)
	}
	if result := engine.Function(pkg.Func("diamonds"), ssaflow.NewSearchBudget(2000)); result.Reason != "" {
		t.Fatalf("budget-shortened branch summary poisoned cache: %+v", result)
	}
}
