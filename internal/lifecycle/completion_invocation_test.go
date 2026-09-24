package lifecycle

import (
	"testing"

	"github.com/kojah/gohawk/internal/ssaflow"
)

const exactInvocationFixture = `
package ssaflowtest
func invoke(fn func()) { fn() }
func direct(fn func()) { invoke(fn) }
func forward(fn func()) { invoke(fn) }
func forwarded(fn func()) { forward(fn) }
func capture(fn func()) { func() { fn() }() }
func captured(fn func()) { capture(fn) }
func maybe(fn func(), yes bool) { if yes { fn() } }
func conditional(fn func(), yes bool) { maybe(fn, yes) }
func wrong(fn, other func()) { invoke(other) }
func replace(fn func()) { fn = func() {}; fn() }
func replaced(fn func()) { replace(fn) }
func mutate(fn func()) { change := func() { fn = func() {} }; change(); func() { fn() }() }
func mutated(fn func()) { mutate(fn) }
func choose(fn, other func(), yes bool) { if yes { fn = other }; fn() }
func mixed(fn, other func(), yes bool) { choose(fn, other, yes) }
var saved func()
func store(fn func()) { saved = fn }
func stored(fn func()) { store(fn) }
func first(fns []func()) { fns[0]() }
func aggregate(fn, other func()) { first([]func(){other, fn}) }
func launch(fn func()) { go fn() }
func asynchronous(fn func()) { launch(fn) }
func deferInvoke(fn func()) { defer fn() }
func deferred(fn func()) { deferInvoke(fn) }
func viaCallback(fn func(), callback func(func())) { callback(fn) }
func bound(fn func()) { viaCallback(fn, func(value func()) { value() }) }
func boundNoop(fn func()) { viaCallback(fn, func(value func()) {}) }
`

func TestExactInvocationCompletion(t *testing.T) {
	pkg := buildTestSSA(t, exactInvocationFixture)
	for _, test := range []struct {
		name   string
		proven bool
	}{
		{"direct", true},
		{"forwarded", true},
		{"captured", true},
		{"deferred", true},
		{"conditional", false},
		{"wrong", false},
		{"replaced", false},
		{"mutated", false},
		{"mixed", false},
		{"stored", false},
		{"aggregate", false},
		{"asynchronous", false},
		{"bound", true},
		{"boundNoop", false},
	} {
		t.Run(test.name, func(t *testing.T) {
			fn := pkg.Func(test.name)
			proof := ProveCompletion(CompletionRequest{
				Instruction: findLaunch(t, fn), Target: fn.Params[0], InvokeTarget: true,
				Budget: ssaflow.NewSearchBudget(1000),
			})
			if proof.Proven() != test.proven {
				t.Fatalf("proof = %+v, want proven %v", proof, test.proven)
			}
		})
	}
}

func TestExactInvocationBudgetAndCache(t *testing.T) {
	pkg := buildTestSSA(t, exactInvocationFixture)
	fn := pkg.Func("forwarded")
	request := CompletionRequest{Instruction: findLaunch(t, fn), Target: fn.Params[0], InvokeTarget: true, Budget: ssaflow.NewSearchBudget(1)}
	proof := ProveCompletion(request)
	if proof.State != ssaflow.EvidenceUnknown || proof.Reason != ssaflow.EvidenceBudgetExhausted {
		t.Fatalf("budget exhaustion = %+v", proof)
	}
	var evidence LocalEvidence
	request.Budget = ssaflow.NewSearchBudget(1000)
	if proof := evidence.Completion(request); !proof.Proven() {
		t.Fatalf("invocation = %+v", proof)
	}
	request.InvokeTarget = false
	if proof := evidence.Completion(request); proof.State != ssaflow.EvidenceUnknown {
		t.Fatalf("invalid method request reused invocation cache: %+v", proof)
	}
}
