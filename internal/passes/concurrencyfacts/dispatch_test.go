package concurrencyfacts

import (
	"testing"

	"github.com/kojah/gohawk/internal/ssaflow"
	"github.com/kojah/gohawk/internal/ssaflow/ssaflowtest"
)

func TestClosedInterfaceDispatch(t *testing.T) {
	pkg := ssaflowtest.BuildPackage(t, "dispatch", `package dispatch
import "sync"
type runner interface { Run(chan int) }
type worker struct { mu sync.Mutex }
func (w *worker) Run(c chan int) { w.mu.Lock(); close(c); w.mu.Unlock() }
func direct(c chan int) { var w worker; var r runner = &w; w.mu.Lock(); go r.Run(c); w.mu.Unlock() }
func locker(m *sync.Mutex) { var l sync.Locker = m; l.Lock(); l.Unlock() }
func opaque(r runner, c chan int) { go r.Run(c) }
func mixed(a, b *worker, c chan int, flag bool) { var r runner = a; if flag { r = b }; go r.Run(c) }
func escape(w *worker, f func(runner)) { var r runner = w; f(r) }
`)
	for _, name := range []string{"opaque", "mixed", "escape"} {
		got := NewEngine().Root(pkg.Func(name), ssaflow.NewSearchBudget(ssaflow.SummaryBudget))
		if got.Complete() {
			t.Errorf("%s must remain unknown: %+v", name, got)
		}
	}
	for _, name := range []string{"direct", "locker"} {
		got := NewEngine().Root(pkg.Func(name), ssaflow.NewSearchBudget(ssaflow.SummaryBudget))
		if !got.Complete() || len(got.Operations) != 2 || got.Operations[0].Kind != Lock || got.Operations[1].Kind != Unlock {
			t.Errorf("%s = %+v, want exact lock/unlock", name, got)
		}
		if name == "direct" && (len(got.Workers) != 1 || len(got.Workers[0].Operations) != 3) {
			t.Errorf("concrete worker lost its effects: %+v", got)
		}
	}
}
