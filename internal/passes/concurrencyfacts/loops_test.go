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
	// A loop with an unknown count launches one or more identical workers:
	// one representative, incomplete until a consumer opts in.
	for _, name := range []string{"dynamic", "conditional", "five"} {
		got := NewEngine().Root(pkg.Func(name), ssaflow.NewSearchBudget(ssaflow.SummaryBudget))
		if got.Complete() || got.Reason != ReasonReplicatedWorkers || len(got.Workers) != 1 || !got.Workers[0].Replicated {
			t.Errorf("%s = %+v, want one replicated worker", name, got)
		}
		if read := got.Representatives(); !read.Complete() || len(read.Workers) != 1 {
			t.Errorf("%s representatives = %+v, want one complete worker", name, read)
		}
	}
	for _, name := range []string{"captured", "fresh", "forever"} {
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

// A loop that synchronizes nothing and provably ends adds no effect, so the
// effects around it compose. A loop that may not end, or that does anything
// the summary records, is still a cycle the collectors decline; repeating a
// synchronizing body needs a model of repetition that this fold is not.
func TestQuietLoopsFold(t *testing.T) {
	pkg := ssaflowtest.BuildPackage(t, "quietloops", `package quietloops
import "sync"
func Around(ch chan int, xs []int) int {
	n := 0
	for _, x := range xs { n += x }
	close(ch)
	for i := range n { n += i }
	return n
}
func Nested(ch chan int, rows [][]int) (n int) {
	for _, row := range rows { for i := 0; i < len(row); i++ { n += row[i] } }
	close(ch)
	return
}
func EarlyReturn(ch chan int, xs []int) {
	for _, x := range xs { if x == 0 { return } }
	close(ch)
}
func Flag(ch chan int, done *bool) { for !*done {}; close(ch) }
func Mapped(ch chan int, m map[string]int) (n int) { for _, v := range m { n += v }; close(ch); return }
func Growing(ch chan int, xs []int) []int { for i := 0; i < len(xs); i++ { xs = append(xs, i) }; close(ch); return xs }
func Locking(mu *sync.Mutex, xs []int) { for range xs { mu.Lock(); mu.Unlock() } }
func Sending(ch chan int, xs []int) { for _, x := range xs { ch <- x } }
func Launching(ch chan int, xs []int) { for range xs { go func() {}() }; close(ch) }
func Deferring(ch chan int, xs []int) { for range xs { defer func() {}() }; close(ch) }
func Opaque(ch chan int, xs []func()) { for _, f := range xs { f() }; close(ch) }
func Carried(chans []chan int) {
	var ch chan int
	for _, c := range chans { ch = c }
	close(ch)
}
`)
	engine := NewEngine()
	budget := func() *ssaflow.SearchBudget { return ssaflow.NewSearchBudget(4000) }
	for _, name := range []string{"Around", "Nested"} {
		if got := engine.Root(pkg.Func(name), budget()); got.Completeness() != CompleteWithEffects || !kinds(got, Close) {
			t.Errorf("%s = %+v, want the close alone", name, got)
		}
	}
	// The early return is an exit of the loop, so the paths differ.
	if got := engine.Root(pkg.Func("EarlyReturn"), budget()); len(got.Paths) != 2 {
		t.Errorf("EarlyReturn = %+v, want two paths", got)
	}
	for _, name := range []string{"Flag", "Mapped", "Growing", "Locking", "Sending", "Launching", "Deferring", "Opaque", "Carried"} {
		if got := engine.Root(pkg.Func(name), budget()); got.Complete() || len(got.Paths) != 0 {
			t.Errorf("%s = %+v, want unknown", name, got)
		}
	}
}
