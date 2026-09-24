package concurrencyfacts

import (
	"testing"

	"github.com/kojah/gohawk/internal/ssaflow"
	"github.com/kojah/gohawk/internal/ssaflow/ssaflowtest"
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
	budget := func() *ssaflow.SearchBudget { return ssaflow.NewSearchBudget(2000) }

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
					got := engine.Root(function, ssaflow.NewSearchBudget(2000))
					if got.Completeness() != CompleteWithEffects || !kinds(got, Lock, Close, Unlock) {
						t.Errorf("closeUnderLock = %+v", got)
					}
				case "forwardHole":
					got := engine.Function(function, ssaflow.NewSearchBudget(2000))
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
