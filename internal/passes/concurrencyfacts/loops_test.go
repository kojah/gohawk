package concurrencyfacts

import (
	"testing"

	"github.com/kojah/gohawk/internal/ssaflow"
	"github.com/kojah/gohawk/internal/ssaflow/ssaflowtest"
)

func TestExactWorkerLoops(t *testing.T) {
	pkg := ssaflowtest.BuildPackage(t, "loops", `package loops
func signal(c chan int) { close(c) }
func two(c chan int) { for i := 0; i < 2; i++ { go signal(c) } }
func zero(c chan int) { for i := 0; i < 0; i++ { go signal(c) } }
func dynamic(c chan int, n int) { for i := 0; i < n; i++ { go signal(c) } }
func captured(c chan int) { for i := 0; i < 2; i++ { go func(){ c <- i }() } }
func conditional(c chan int, stop bool) { for i := 0; i < 2; i++ { if stop { break }; go signal(c) } }
func fresh() { for i := 0; i < 2; i++ { c := make(chan int); go signal(c) } }
func five(c chan int) { for i := 0; i < 5; i++ { go signal(c) } }
func forever(c chan int) { for { go signal(c) } }
`)
	for _, name := range []string{"dynamic", "captured", "conditional", "fresh", "five", "forever"} {
		got := NewEngine().Root(pkg.Func(name), ssaflow.NewSearchBudget(ssaflow.SummaryBudget))
		if got.Complete() || len(got.Workers) != 0 {
			t.Errorf("%s retained unsupported loop effects: %+v", name, got)
		}
	}
	for name, count := range map[string]int{"two": 2, "zero": 0} {
		got := NewEngine().Root(pkg.Func(name), ssaflow.NewSearchBudget(ssaflow.SummaryBudget))
		if !got.Complete() || len(got.Workers) != count {
			t.Errorf("%s = %+v, want %d children", name, got, count)
		}
	}
}
