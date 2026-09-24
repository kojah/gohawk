package concurrencyfacts

import (
	"testing"

	"github.com/kojah/gohawk/internal/ssaflow"
	"github.com/kojah/gohawk/internal/ssaflow/ssaflowtest"
)

func TestRootTracksBoundedChildren(t *testing.T) {
	pkg := ssaflowtest.BuildPackage(t, "rootworkers", `package rootworkers
func signal(ch chan int) { close(ch) }
func two(a, b chan int) { go signal(a); go signal(b) }
func emptyChild() { go func() {}() }
func five(a chan int) {
 go signal(a); go signal(a); go signal(a); go signal(a); go signal(a)
}
func optional(a chan int, run bool) { if run { go signal(a) } }
func unknown(a chan int, callback func()) { go signal(a); go callback() }
`)
	engine := NewEngine()
	function := pkg.Func("two")
	result := engine.Root(function, ssaflow.NewSearchBudget(2000))
	if !result.Complete() || len(result.Workers) != 2 || len(result.Operations) != 0 {
		t.Fatalf("two-child root = %+v", result)
	}
	for index, parameter := range function.Params[:2] {
		worker := result.Workers[index]
		if worker.Spawn == nil || worker.Prefix != 0 || len(worker.Operations) != 1 ||
			worker.Operations[0].Kind != Close || worker.Operations[0].Resource.Value != parameter {
			t.Errorf("child %d = %+v, want one close of %v", index, worker, parameter)
		}
	}
	empty := engine.Root(pkg.Func("emptyChild"), ssaflow.NewSearchBudget(2000))
	if !empty.Complete() || empty.Completeness() != CompleteWithEffects || len(empty.Workers) != 1 ||
		len(empty.Workers[0].Operations) != 0 {
		t.Errorf("event-free child was not retained: %+v", empty)
	}
	for name, reason := range map[string]string{
		"five":     "protocol-participants-unknown",
		"optional": "protocol-branch-effects-differ",
		"unknown":  "protocol-body-unavailable",
	} {
		t.Run(name, func(t *testing.T) {
			got := engine.Root(pkg.Func(name), ssaflow.NewSearchBudget(2000))
			if got.Complete() || got.Reason != reason || len(got.Workers) != 0 {
				t.Errorf("%s root = %+v, want no usable children (%s)", name, got, reason)
			}
		})
	}
	if got := engine.Root(function, ssaflow.NewSearchBudget(1)); got.Complete() || len(got.Workers) != 0 {
		t.Fatalf("budget-cut root retained children: %+v", got)
	}
}

func TestMutexPointerCaptureIsNotStateCopy(t *testing.T) {
	pkg := ssaflowtest.BuildPackage(t, "mutexlaunch", `package mutexlaunch
import "sync"
func Spawn(a *sync.Mutex) { go func(){ a.Lock(); a.Unlock() }() }
func Copy(a *sync.Mutex) { b := *a; go func(){ b.Lock(); b.Unlock() }() }
func Mutable(a, b *sync.Mutex) { p := a; go func(){ p.Lock(); p.Unlock() }(); p = b; _ = p }
`)
	got := NewEngine().Function(pkg.Func("Spawn"), ssaflow.NewSearchBudget(2000))
	if got.Completeness() != CompleteWithEffects || len(got.Workers) != 1 || len(got.Workers[0].Operations) != 2 {
		t.Fatalf("pointer capture = %+v", got)
	}
	for i, kind := range []Kind{Lock, Unlock} {
		if op := got.Workers[0].Operations[i]; op.Kind != kind || op.Resource.Value != pkg.Func("Spawn").Params[0] {
			t.Errorf("worker operation %d = %+v", i, op)
		}
	}
	for _, name := range []string{"Copy", "Mutable"} {
		if result := NewEngine().Function(pkg.Func(name), ssaflow.NewSearchBudget(2000)); result.Complete() {
			t.Errorf("%s should remain unknown: %+v", name, result)
		}
	}
}

func TestHelperLaunchesComposeAsDistinctChildren(t *testing.T) {
	pkg := ssaflowtest.BuildPackage(t, "helperlaunch", `package helperlaunch
func launch(ch chan int) { go func() { close(ch) }() }
func forward(ch chan int) { launch(ch) }
func two(a, b chan int) { forward(a); forward(b) }
func conditional(a chan int, run bool) { if run { forward(a) } }
func loop(a chan int) { for i := 0; i < 2; i++ { forward(a) } }
func five(a, b, c, d, e chan int) {
 launch(a)
 launch(b)
 launch(c)
 launch(d)
 launch(e)
}
`)
	function := pkg.Func("two")
	got := NewEngine().Root(function, ssaflow.NewSearchBudget(2000))
	if !got.Complete() || len(got.Workers) != 2 {
		t.Fatalf("two helper launches = %+v", got)
	}
	for index, worker := range got.Workers {
		if !worker.Site.IsValid() || len(worker.Operations) != 1 || worker.Operations[0].Kind != Close ||
			worker.Operations[0].Resource.Value != function.Params[index] {
			t.Errorf("worker %d = %+v", index, worker)
		}
	}
	if got.Workers[0].Site == got.Workers[1].Site {
		t.Error("separate helper calls must retain distinct launch sites")
	}
	for _, name := range []string{"conditional", "loop", "five"} {
		if result := NewEngine().Root(pkg.Func(name), ssaflow.NewSearchBudget(2000)); result.Complete() {
			t.Errorf("%s should remain unknown: %+v", name, result)
		}
	}
}
