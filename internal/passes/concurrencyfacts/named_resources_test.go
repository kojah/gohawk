package concurrencyfacts

import (
	"testing"

	"github.com/kojah/gohawk/internal/ssaflow"
	"github.com/kojah/gohawk/internal/ssaflow/ssaflowtest"
)

// A sync/atomic operation is a memory access that never blocks, so it adds
// no effect when every value it moves is inert. A value that could carry a
// resource still stops the summary, and a project method that merely shares
// an atomic name is summarized from its own body.
func TestAtomicOperationsAreInert(t *testing.T) {
	pkg := ssaflowtest.BuildPackage(t, "atomics", `package atomics
import (
	"sync"
	"sync/atomic"
)
type Bool struct{ n int }
var guard sync.Mutex
func (b *Bool) Load() bool { guard.Lock(); guard.Unlock(); return b.n > 0 }
var ready atomic.Bool
var count int64
var box atomic.Value
var mutexes atomic.Pointer[sync.Mutex]
func Counters(ch chan int) {
	ready.Store(true)
	atomic.AddInt64(&count, 1)
	_ = ready.Load()
	close(ch)
}
func Loaded(ch chan int) string {
	close(ch)
	s, _ := box.Load().(string)
	return s
}
func LoadedChannel() { close(box.Load().(chan int)) }
func StoredChannel(ch chan int) { box.Store(ch); close(ch) }
func PointerToMutex(ch chan int) { mutexes.Load(); close(ch) }
func Misleading(b *Bool, ch chan int) { b.Load(); close(ch) }
`)
	engine := NewEngine()
	for name, closes := range map[string]int{"Counters": 1, "Loaded": 1, "Misleading": 3} {
		got := engine.Root(pkg.Func(name), ssaflow.NewSearchBudget(2000))
		if got.Completeness() != CompleteWithEffects || len(got.Operations) != closes {
			t.Errorf("%s = %+v, want %d operations", name, got, closes)
		}
	}
	for _, name := range []string{"LoadedChannel", "StoredChannel", "PointerToMutex"} {
		if got := engine.Root(pkg.Func(name), ssaflow.NewSearchBudget(2000)); got.Complete() {
			t.Errorf("%s = %+v, want incomplete", name, got)
		}
	}
}

// When a closure captures a parameter, the builder spills it to a cell and
// reads it back. A read is the parameter while the heap model proves the cell
// still holds it. After a local reassignment the read is the new parameter, so
// the lock and unlock name different mutexes; after the closure writes the
// cell, the read is unnamed.
func TestSpilledParameterNamesItsFields(t *testing.T) {
	pkg := ssaflowtest.BuildPackage(t, "spills", `package spills
import "sync"
type D struct{ mu sync.Mutex; n int }
func Reads(d *D) { d.mu.Lock(); f := func() { d.n++ }; f(); d.mu.Unlock() }
func Writes(d, o *D) { d.mu.Lock(); f := func() { d = o }; f(); d.mu.Unlock() }
func Reassigns(d, o *D) { d.mu.Lock(); f := func() { _ = d.n }; f(); d = o; d.mu.Unlock() }
func Caller(x *D) { Reads(x); x.mu.Lock(); x.mu.Unlock() }
`)
	engine := NewEngine()
	if got := engine.Root(pkg.Func("Reads"), ssaflow.NewSearchBudget(2000)); !pairedLock(got) {
		t.Errorf("Reads = %+v, want a lock and unlock of one mutex", got)
	}
	// The helper's lock binds to the caller's own address for x.mu.
	caller := engine.Root(pkg.Func("Caller"), ssaflow.NewSearchBudget(2000))
	if !caller.Complete() || len(caller.Operations) != 4 || caller.Operations[0].Resource != caller.Operations[2].Resource {
		t.Errorf("Caller = %+v, want the helper's lock of x.mu to match the caller's", caller)
	}
	reassigns := engine.Root(pkg.Func("Reassigns"), ssaflow.NewSearchBudget(2000))
	if !reassigns.Complete() || len(reassigns.Operations) != 2 || reassigns.Operations[0].Resource == reassigns.Operations[1].Resource {
		t.Errorf("Reassigns = %+v, want a lock of d.mu and an unlock of o.mu", reassigns)
	}
	if got := engine.Root(pkg.Func("Writes"), ssaflow.NewSearchBudget(2000)); got.Complete() {
		t.Errorf("Writes = %+v, want incomplete", got)
	}
}

// A mutex stored in a package variable is one object for every caller, so a
// helper's lock of it binds unchanged. A pointer or channel read from a
// package variable may change between reads and stays unnamed.
func TestGlobalMutexBindsUnchanged(t *testing.T) {
	pkg := ssaflowtest.BuildPackage(t, "globals", `package globals
import "sync"
var state struct{ sync.Mutex; n int }
var plain sync.Mutex
var shared *sync.Mutex
var holder struct{ mu *sync.Mutex }
var box struct{ ch chan int }
func lockState() { state.Lock(); state.n++; state.Unlock() }
func lockPlain() { plain.Lock(); plain.Unlock() }
func lockShared() { shared.Lock(); shared.Unlock() }
func lockHeld() { holder.mu.Lock(); holder.mu.Unlock() }
func closeBox() { close(box.ch) }
func State() { lockState() }
func Plain() { lockPlain() }
func Shared() { lockShared() }
func Held() { lockHeld() }
func Box() { closeBox() }
`)
	engine := NewEngine()
	for _, name := range []string{"lockState", "State", "Plain"} {
		if got := engine.Root(pkg.Func(name), ssaflow.NewSearchBudget(2000)); !pairedLock(got) {
			t.Errorf("%s = %+v, want a lock and unlock of one mutex", name, got)
		}
	}
	for _, name := range []string{"Shared", "Held", "Box"} {
		if got := engine.Root(pkg.Func(name), ssaflow.NewSearchBudget(2000)); got.Complete() {
			t.Errorf("%s = %+v, want incomplete", name, got)
		}
	}
}

func pairedLock(summary Summary) bool {
	return summary.Completeness() == CompleteWithEffects && len(summary.Operations) == 2 &&
		summary.Operations[0].Kind == Lock && summary.Operations[1].Kind == Unlock &&
		summary.Operations[0].Resource == summary.Operations[1].Resource
}

// A mutex reached through a write-once pointer field is one object at every
// load, in the function, its goroutines, and its callers. A field the package
// reassigns, and a map element, stay unnamed.
func TestWriteOnceFieldsNameMutexes(t *testing.T) {
	pkg := ssaflowtest.BuildPackage(t, "writeonce", `package writeonce
import "sync"
type conn struct{ mu sync.Mutex }
func (c *conn) unlock() { c.mu.Unlock() }
type server struct{ conn *conn }
func newServer() *server { return &server{conn: &conn{}} }
func Serve(s *server) { s.conn.mu.Lock(); s.conn.mu.Unlock() }
func Parent(s *server) {
	s.conn.mu.Lock()
	done := make(chan int)
	go func() { s.conn.mu.Lock(); s.conn.mu.Unlock(); close(done) }()
	<-done
	s.conn.mu.Unlock()
}
func unlockConn(s *server) { s.conn.mu.Unlock() }
func Helper(s *server) { s.conn.mu.Lock(); unlockConn(s) }
func Argument(s *server) { s.conn.mu.Lock(); s.conn.unlock() }
type loose struct{ conn *conn }
func swap(l *loose) { l.conn = &conn{} }
func Loose(l *loose) { l.conn.mu.Lock(); l.conn.mu.Unlock() }
func Mapped(m map[string]*conn) { m["k"].mu.Lock(); m["k"].mu.Unlock() }
func Rebinds(s, o *server) { s.conn.mu.Lock(); go func() { s = o }(); s.conn.mu.Unlock() }
`)
	engine := NewEngine()
	budget := func() *ssaflow.SearchBudget { return ssaflow.NewSearchBudget(4000) }
	for _, name := range []string{"Serve", "Helper", "Argument"} {
		if got := engine.Root(pkg.Func(name), budget()); !pairedLock(got) {
			t.Errorf("%s = %+v, want a lock and unlock of one mutex", name, got)
		}
	}
	parent := engine.Root(pkg.Func("Parent"), budget())
	if !parent.Complete() || len(parent.Workers) != 1 || len(parent.Workers[0].Operations) == 0 ||
		parent.Workers[0].Operations[0].Resource != parent.Operations[0].Resource {
		t.Errorf("Parent = %+v, want the child to lock the parent's mutex", parent)
	}
	// A goroutine that reassigns the captured parameter makes later reads
	// name some other object.
	if got := engine.Root(pkg.Func("Rebinds"), budget()); pairedLock(got) {
		t.Errorf("Rebinds = %+v, want the reads after the launch unnamed", got)
	}
	for _, name := range []string{"Loose", "Mapped"} {
		if got := engine.Root(pkg.Func(name), budget()); got.Complete() {
			t.Errorf("%s = %+v, want incomplete", name, got)
		}
	}
}
