package goroutineownership

import (
	"testing"

	"github.com/kojah/gohawk/internal/ssaflow"
	"github.com/kojah/gohawk/internal/ssaflow/ssaflowtest"
)

func TestWorkerOutputNilAllowance(t *testing.T) {
	pkg := ssaflowtest.BuildPackage(t, "outputnil", `package outputnil
func publish(done chan int)
func disabled(){publish(nil)}
func enabled(done chan int){publish(done)}
`)
	for _, test := range []struct {
		name        string
		parentLimit int
		publishes   bool
	}{
		{"disabled", 3, false}, {"enabled", 2, true},
	} {
		t.Run(test.name, func(t *testing.T) {
			fn := pkg.Func(test.name)
			pool := ssaflow.NewSearchBudget(spawnPoolBudget)
			child := pool.Within(test.parentLimit)
			got := workerHandsOffOutputChannelWithin(fn, child)
			t.Logf("%s: publishes=%v child exhausted=%v pool exhausted=%v", test.name, got, child.Exhausted(), pool.Exhausted())
			if !child.Exhausted() || pool.Exhausted() || got {
				t.Fatalf("unfinished publication query=%v, exhausted=%v/%v", got, child.Exhausted(), pool.Exhausted())
			}
			fresh := ssaflow.NewSearchBudget(ssaflow.SummaryBudget)
			if got := workerHandsOffOutputChannelWithin(fn, fresh); got != test.publishes || fresh.Exhausted() {
				t.Fatalf("fresh publication query=%v, exhausted=%v", got, fresh.Exhausted())
			}
		})
	}
}
