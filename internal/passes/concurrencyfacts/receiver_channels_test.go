package concurrencyfacts

import (
	"testing"

	"github.com/kojah/gohawk/internal/ssaflow"
	"github.com/kojah/gohawk/internal/ssaflow/ssaflowtest"
)

func TestReceiverChannelRelationships(t *testing.T) {
	pkg := ssaflowtest.BuildPackage(t, "owners", `package owners
import "sync"
type owner struct { mu sync.Mutex; done chan int }
func (o *owner) run() { o.mu.Lock(); close(o.done); o.mu.Unlock() }
func (o *owner) start() { go o.run() }
func (o *owner) stop() { <-o.done; o.mu.Unlock() }
func root() { var o owner; o.done = make(chan int); o.mu.Lock(); o.start(); o.stop() }
func mutable() { var o owner; o.done = make(chan int); o.mu.Lock(); o.start(); o.done = make(chan int); o.stop() }
func external(o *owner) { o.mu.Lock(); o.start(); o.stop() }
func pair() {
 var a, b owner
 a.done = make(chan int); b.done = make(chan int)
 a.mu.Lock(); b.mu.Lock(); a.start(); b.start(); a.stop(); b.stop()
}
`)
	for _, name := range []string{"mutable", "external"} {
		got := NewEngine().Root(pkg.Func(name), ssaflow.NewSearchBudget(ssaflow.SummaryBudget))
		if got.Complete() {
			t.Errorf("%s accepted unstable channel identity: %+v", name, got)
		}
	}
	got := NewEngine().Root(pkg.Func("root"), ssaflow.NewSearchBudget(ssaflow.SummaryBudget))
	if !got.Complete() || len(got.Workers) != 1 || len(got.Operations) != 3 {
		t.Fatalf("receiver channel protocol = %+v", got)
	}
	if got.Workers[0].Operations[1].Resource != got.Operations[1].Resource {
		t.Error("worker completion and parent wait disagree on channel identity")
	}
	pair := NewEngine().Root(pkg.Func("pair"), ssaflow.NewSearchBudget(ssaflow.SummaryBudget))
	if !pair.Complete() || len(pair.Workers) != 2 || len(pair.Operations) != 6 {
		t.Fatalf("two-owner protocol = %+v", pair)
	}
	for index, worker := range pair.Workers {
		if worker.Operations[1].Resource != pair.Operations[2+2*index].Resource {
			t.Errorf("owner %d completion was rebound to another invocation", index)
		}
	}
	if pair.Workers[0].Operations[1].Resource == pair.Workers[1].Operations[1].Resource {
		t.Error("separate owners shared a channel identity")
	}
}
