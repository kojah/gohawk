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
	for name, reason := range map[string]Reason{
		"five":     ReasonParticipantsUnknown,
		"optional": ReasonBranchAlternatives,
		"unknown":  ReasonBodyUnavailable,
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
	for _, name := range []string{"conditional", "five"} {
		if result := NewEngine().Root(pkg.Func(name), ssaflow.NewSearchBudget(2000)); result.Complete() {
			t.Errorf("%s should remain unknown: %+v", name, result)
		}
	}
}

// A root's results reach its caller only after the root returns, and the
// detached recovery block is dead unless a deferred call can recover. A helper
// returning a reference still adds its caller as a possible participant.
func TestRootResultsAndDetachedRecovery(t *testing.T) {
	pkg := ssaflowtest.BuildPackage(t, "rootresults", `package rootresults
import ("errors"; "sync")
var errFailed = errors.New("failed")
func Result(mu *sync.Mutex, fail bool) (int, error) {
	mu.Lock()
	defer mu.Unlock()
	if fail { return 0, errFailed }
	return 1, nil
}
func Returned(done chan int) chan int { close(done); return done }
func Recovered(mu *sync.Mutex) (err error) {
	mu.Lock()
	defer func() {
		if recover() != nil { err = errFailed }
		mu.Unlock()
	}()
	return nil
}
func Caller(done chan int) { _ = Returned(done) }
`)
	engine := NewEngine()
	for _, name := range []string{"Result", "Returned"} {
		if got := engine.Root(pkg.Func(name), ssaflow.NewSearchBudget(2000)); got.Completeness() != CompleteWithEffects {
			t.Errorf("%s root = %+v, want complete", name, got)
		}
	}
	for name, reason := range map[string]Reason{"Recovered": ReasonBodyUnavailable, "Caller": ReasonEffectUnknown} {
		if got := engine.Root(pkg.Func(name), ssaflow.NewSearchBudget(2000)); got.Complete() || got.Reason != reason {
			t.Errorf("%s root = %+v, want %s", name, got, reason)
		}
	}
	if got := engine.Function(pkg.Func("Returned"), ssaflow.NewSearchBudget(2000)); got.Complete() {
		t.Errorf("helper returning a channel = %+v, want incomplete", got)
	}
}

// Reads, writes, nil checks, and container work on unrelated values are passive.
// A value or slot that could be a resource identity, or a copy of one, still
// stops the summary, including a mutex on an object reached through storage.
func TestCallerStorageLoads(t *testing.T) {
	pkg := ssaflowtest.BuildPackage(t, "callerloads", `package callerloads
import ("context"; "sync")
type owner struct {
	mu    sync.Mutex
	ptr   *sync.Mutex
	done  chan int
	ctx   context.Context
	count int
	name  string
	peer  *owner
}
func Inert(o *owner, ok bool) int {
	o.mu.Lock()
	n := o.count
	_ = o.name
	_ = o.peer
	if !ok { n = -n }
	o.mu.Unlock()
	return n
}
func Writes(o *owner) {
	o.mu.Lock()
	o.count++
	o.peer.count = 0
	if o.peer != nil && o.done == nil { o.name = "set" }
	o.mu.Unlock()
}
func Containers(o *owner, key string) int {
	o.mu.Lock()
	seen := make(map[string]int)
	seen[key]++
	counts := make([]int, 4)
	counts[1] = seen[key]
	_, ok := seen["other"]
	o.mu.Unlock()
	if ok { return 0 }
	return counts[1]
}
func StoreInMap(ch chan int) { m := make(map[string]chan int); m["k"] = ch }
func ReadFromMap(m map[string]chan int) { <-m["k"] }
func PeerMutex(o *owner) { o.peer.mu.Lock(); o.peer.mu.Unlock() }
func StoreChannel(o *owner, ch chan int) { o.done = ch }
func Channel(o *owner) { <-o.done }
func Pointer(o *owner) { o.ptr.Lock(); o.ptr.Unlock() }
func Copy(o *owner) { peer := *o.peer; _ = peer }
func Context(o *owner) { <-o.ctx.Done() }
`)
	engine := NewEngine()
	if got := engine.Root(pkg.Func("Inert"), ssaflow.NewSearchBudget(2000)); got.Completeness() != CompleteWithEffects || len(got.Operations) != 2 {
		t.Errorf("inert reads = %+v, want the lock pair alone", got)
	}
	if got := engine.Root(pkg.Func("Writes"), ssaflow.NewSearchBudget(2000)); got.Completeness() != CompleteWithEffects || len(got.Operations) != 2 {
		t.Errorf("inert writes = %+v, want the lock pair alone", got)
	}
	if got := engine.Root(pkg.Func("Containers"), ssaflow.NewSearchBudget(2000)); got.Completeness() != CompleteWithEffects || len(got.Operations) != 2 {
		t.Errorf("inert containers = %+v, want the lock pair alone", got)
	}
	for _, name := range []string{"Channel", "Pointer", "Copy", "Context", "PeerMutex", "StoreChannel", "StoreInMap", "ReadFromMap"} {
		if got := engine.Root(pkg.Func(name), ssaflow.NewSearchBudget(2000)); got.Complete() {
			t.Errorf("%s = %+v, want incomplete", name, got)
		}
	}
}
