package concurrencyfacts

import (
	"testing"

	proofs "github.com/kojah/gohawk/internal/engine/proof"
	"github.com/kojah/gohawk/internal/engine/ssaflow"
	"github.com/kojah/gohawk/internal/engine/ssaflow/ssaflowtest"
	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/analysis/analysistest"
	"golang.org/x/tools/go/analysis/passes/buildssa"
)

// A call through a function input is a hole in the ordered effects. Callers
// that supply a known function fill it in place; callers that forward their
// own input keep it; anything else stays unknown.
func TestCallbackHoles(t *testing.T) {
	pkg := ssaflowtest.BuildPackage(t, "callbacks", `package callbacks
import "sync"
func run(mu *sync.Mutex, f func()) { mu.Lock(); f(); mu.Unlock() }
func closure(mu *sync.Mutex, done chan int) { run(mu, func() { close(done) }) }
func nothing() {}
func static(mu *sync.Mutex) { run(mu, nothing) }
func forward(mu *sync.Mutex, g func()) { run(mu, g) }
func launch(mu *sync.Mutex, done chan int) { run(mu, func() { go func() { close(done) }() }) }
type holder struct{ cb func() }
func field(mu *sync.Mutex, h *holder) { run(mu, h.cb) }
func worker(f func()) { go func() { f() }() }
func channelArgument(f func(chan int), done chan int) { f(done) }
func branching(mu *sync.Mutex, done chan int, flag bool) {
	run(mu, func() { if flag { close(done) } })
}
`)
	engine := NewEngine()
	budget := func() *proofs.SearchBudget { return proofs.NewSearchBudget(2000) }

	hole := engine.Function(pkg.Func("run"), budget())
	if hole.Complete() || hole.Reason != ReasonCallbackBindingRequired || len(hole.Operations) != 3 ||
		hole.Operations[1].Kind != Invoke {
		t.Fatalf("run = %+v, want lock, hole, unlock", hole)
	}
	closure := engine.Root(pkg.Func("closure"), budget())
	if closure.Completeness() != CompleteWithEffects || !kinds(closure, Lock, Close, Unlock) ||
		closure.Operations[1].Resource.Value != pkg.Func("closure").Params[1] {
		t.Errorf("closure = %+v, want the close spliced between lock and unlock", closure)
	}
	if static := engine.Root(pkg.Func("static"), budget()); static.Completeness() != CompleteWithEffects || !kinds(static, Lock, Unlock) {
		t.Errorf("static = %+v, want an empty callback filled", static)
	}
	forward := engine.Function(pkg.Func("forward"), budget())
	if forward.Reason != ReasonCallbackBindingRequired || !kinds(forward, Lock, Invoke, Unlock) ||
		forward.Operations[1].Resource.Value != pkg.Func("forward").Params[1] {
		t.Errorf("forward = %+v, want the hole moved to the caller's input", forward)
	}
	// A callback that launches a goroutine hands its captured cell to an
	// asynchronous participant, so the heap model cannot prove the cell stable
	// across the enclosing call. The worker stays unknown rather than guessed.
	if launch := engine.Root(pkg.Func("launch"), budget()); launch.Complete() || launch.Reason != ReasonChannelBindingUnknown {
		t.Errorf("launch = %+v, want the captured cell left unknown", launch)
	}
	for name, reason := range map[string]Reason{
		"field":           ReasonCallbackUnknown,
		"worker":          ReasonBodyUnavailable,
		"channelArgument": ReasonBodyUnavailable,
		"branching":       ReasonCallbackUnknown,
	} {
		if got := engine.Root(pkg.Func(name), budget()); got.Complete() || got.Reason != reason {
			t.Errorf("%s = %+v, want %s", name, got, reason)
		}
	}
}

func kinds(summary Summary, want ...Kind) bool {
	if len(summary.Operations) != len(want) {
		return false
	}
	for index, kind := range want {
		if summary.Operations[index].Kind != kind {
			return false
		}
	}
	return true
}

// A published hole crosses the package boundary by parameter position and is
// filled by the importing caller's closure.
func TestImportedCallbackHoles(t *testing.T) {
	consumer := &analysis.Analyzer{
		Name: "testcallbacks", Doc: "assert imported callback holes", Requires: []*analysis.Analyzer{Analyzer, buildssa.Analyzer},
		Run: func(pass *analysis.Pass) (any, error) {
			engine := pass.ResultOf[Analyzer].(*Engine)
			functions, err := ssaflow.SourceSSAFunctions(pass)
			if err != nil {
				return nil, err
			}
			for _, function := range functions {
				switch function.Name() {
				case "closeUnderLock":
					got := engine.Root(function, proofs.NewSearchBudget(2000))
					if got.Completeness() != CompleteWithEffects || !kinds(got, Lock, Close, Unlock) {
						t.Errorf("closeUnderLock = %+v", got)
					}
				case "forwardHole":
					got := engine.Function(function, proofs.NewSearchBudget(2000))
					if got.Reason != ReasonCallbackBindingRequired || !kinds(got, Lock, Invoke, Unlock) {
						t.Errorf("forwardHole = %+v", got)
					}
				}
			}
			return nil, nil
		},
	}
	analysistest.Run(t, analysistest.TestData(), consumer, "callbackuse")
}

// A method call on an interface input is a hole, like a call through a
// function input. A caller that boxes one concrete value fills it with that
// type's method bound to the value; a caller that forwards its own interface
// input, directly or through an interface conversion, keeps it. An interface
// read from memory, merged from several types, or whose method branches stays
// unknown, and a method that takes a resource argument is no hole at all.
func TestInterfaceHoles(t *testing.T) {
	pkg := ssaflowtest.BuildPackage(t, "ifaces", `package ifaces
import "sync"
type Doer interface{ Do() }
type DoCloser interface{ Do(); Close() }
type Sender interface{ Send(chan int) }
type quiet struct{ n int }
func (q *quiet) Do() { q.n++ }
func (q *quiet) Close() {}
type locked struct{ mu sync.Mutex }
func (l *locked) Do() { l.mu.Lock(); l.mu.Unlock() }
type branchy struct{ mu sync.Mutex; flag bool }
func (b *branchy) Do() { if b.flag { b.mu.Lock(); b.mu.Unlock() } }
type holder struct{ d Doer }
func run(mu *sync.Mutex, d Doer) { mu.Lock(); d.Do(); mu.Unlock() }
func Quiet(mu *sync.Mutex) { run(mu, &quiet{}) }
func Locked(mu *sync.Mutex) { l := &locked{}; l.mu.Lock(); l.mu.Unlock(); run(mu, l) }
func forward(mu *sync.Mutex, d Doer) { run(mu, d) }
func Forwarded(mu *sync.Mutex) { forward(mu, &quiet{}) }
func widened(mu *sync.Mutex, d DoCloser) { run(mu, d) }
func Field(mu *sync.Mutex, h *holder) { run(mu, h.d) }
func Either(mu *sync.Mutex, flag bool) {
	var d Doer = &quiet{}
	if flag { d = &locked{} }
	run(mu, d)
}
func Branchy(mu *sync.Mutex) { run(mu, &branchy{}) }
func send(s Sender, ch chan int) { s.Send(ch) }
`)
	engine := NewEngine()
	budget := func() *proofs.SearchBudget { return proofs.NewSearchBudget(2000) }

	if hole := engine.Function(pkg.Func("run"), budget()); hole.Reason != ReasonCallbackBindingRequired || !kinds(hole, Lock, Invoke, Unlock) {
		t.Fatalf("run = %+v, want lock, hole, unlock", hole)
	}
	for _, name := range []string{"Quiet", "Forwarded"} {
		if got := engine.Root(pkg.Func(name), budget()); got.Completeness() != CompleteWithEffects || !kinds(got, Lock, Unlock) {
			t.Errorf("%s = %+v, want the quiet method filled", name, got)
		}
	}
	// The filled method's lock binds to the caller's own l.mu.
	locked := engine.Root(pkg.Func("Locked"), budget())
	if !locked.Complete() || !kinds(locked, Lock, Unlock, Lock, Lock, Unlock, Unlock) ||
		locked.Operations[3].Resource != locked.Operations[0].Resource {
		t.Errorf("Locked = %+v, want l.mu locked inside mu", locked)
	}
	for _, name := range []string{"forward", "widened"} {
		function := pkg.Func(name)
		got := engine.Function(function, budget())
		if got.Reason != ReasonCallbackBindingRequired || !kinds(got, Lock, Invoke, Unlock) ||
			got.Operations[1].Resource.Value != function.Params[1] {
			t.Errorf("%s = %+v, want the hole moved to the caller's input", name, got)
		}
	}
	for _, name := range []string{"Field", "Either", "Branchy"} {
		if got := engine.Root(pkg.Func(name), budget()); got.Complete() {
			t.Errorf("%s = %+v, want incomplete", name, got)
		}
	}
	if got := engine.Function(pkg.Func("send"), budget()); got.Complete() || got.Reason == ReasonCallbackBindingRequired {
		t.Errorf("send = %+v, want an opaque call, not a hole", got)
	}
}

// A published interface hole names its method and is filled by the importing
// caller's concrete value.
func TestImportedInterfaceHoles(t *testing.T) {
	consumer := &analysis.Analyzer{
		Name: "testinterfaces", Doc: "assert imported interface holes", Requires: []*analysis.Analyzer{Analyzer, buildssa.Analyzer},
		Run: func(pass *analysis.Pass) (any, error) {
			engine := pass.ResultOf[Analyzer].(*Engine)
			functions, err := ssaflow.SourceSSAFunctions(pass)
			if err != nil {
				return nil, err
			}
			for _, function := range functions {
				switch function.Name() {
				case "quietUnderLock":
					got := engine.Root(function, proofs.NewSearchBudget(2000))
					if got.Completeness() != CompleteWithEffects || !kinds(got, Lock, Unlock) {
						t.Errorf("quietUnderLock = %+v", got)
					}
				case "forwardInterface":
					got := engine.Function(function, proofs.NewSearchBudget(2000))
					if got.Reason != ReasonCallbackBindingRequired || !kinds(got, Lock, Invoke, Unlock) {
						t.Errorf("forwardInterface = %+v", got)
					}
				}
			}
			return nil, nil
		},
	}
	analysistest.Run(t, analysistest.TestData(), consumer, "ifaceuse")
}

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
		got := NewEngine().Root(pkg.Func(name), proofs.NewSearchBudget(proofs.SummaryBudget))
		if got.Complete() {
			t.Errorf("%s must remain unknown: %+v", name, got)
		}
	}
	for _, name := range []string{"direct", "locker"} {
		got := NewEngine().Root(pkg.Func(name), proofs.NewSearchBudget(proofs.SummaryBudget))
		if !got.Complete() || len(got.Operations) != 2 || got.Operations[0].Kind != Lock || got.Operations[1].Kind != Unlock {
			t.Errorf("%s = %+v, want exact lock/unlock", name, got)
		}
		if name == "direct" && (len(got.Workers) != 1 || len(got.Workers[0].Operations) != 3) {
			t.Errorf("concrete worker lost its effects: %+v", got)
		}
	}
}
