package concurrencyfacts

import (
	"testing"

	"github.com/kojah/gohawk/internal/ssaflow"
	"github.com/kojah/gohawk/internal/ssaflow/ssaflowtest"
	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/analysis/analysistest"
	"golang.org/x/tools/go/analysis/passes/buildssa"
)

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
	budget := func() *ssaflow.SearchBudget { return ssaflow.NewSearchBudget(2000) }

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
					got := engine.Root(function, ssaflow.NewSearchBudget(2000))
					if got.Completeness() != CompleteWithEffects || !kinds(got, Lock, Unlock) {
						t.Errorf("quietUnderLock = %+v", got)
					}
				case "forwardInterface":
					got := engine.Function(function, ssaflow.NewSearchBudget(2000))
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
