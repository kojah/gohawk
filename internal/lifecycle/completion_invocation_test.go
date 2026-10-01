package lifecycle

import (
	"testing"

	"github.com/kojah/gohawk/internal/ssaflow"
	"golang.org/x/tools/go/ssa"
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
func spawnedDirect(fn func()) { go invoke(fn) }
func spawnedForwarded(fn func()) { go forward(fn) }
func spawnedCaptured(fn func()) { go capture(fn) }
func spawnedDeferred(fn func()) { go deferInvoke(fn) }
func spawnedConditional(fn func(), yes bool) { go maybe(fn, yes) }
func spawnedWrong(fn, other func()) { go invoke(other) }
func spawnedReplaced(fn func()) { go replace(fn) }
func spawnedMixed(fn, other func(), yes bool) { go choose(fn, other, yes) }
func spawnedAsynchronous(fn func()) { go launch(fn) }
func spawnedBound(fn func()) { go viaCallback(fn, func(value func()) { value() }) }
func spawnedBoundNoop(fn func()) { go viaCallback(fn, func(value func()) {}) }
func recursive(fn func()) { recursive(fn) }
func spawnedRecursive(fn func()) { go recursive(fn) }
func unavailable(fn func())
func spawnedUnavailable(fn func()) { go unavailable(fn) }
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
			if got := CallInvokesArgumentOnEveryReturn(findLaunch(t, fn), fn.Params[0]); got != proof.Proven() {
				t.Fatalf("callback helper = %v, structured proof = %+v", got, proof)
			}
		})
	}
}

func TestSpawnedInvocationCompletion(t *testing.T) {
	pkg := buildTestSSA(t, exactInvocationFixture)
	for _, test := range []struct {
		name string
		want bool
	}{
		{"spawnedDirect", true},
		{"spawnedForwarded", true},
		{"spawnedCaptured", true},
		{"spawnedDeferred", true},
		{"spawnedConditional", false},
		{"spawnedWrong", false},
		{"spawnedReplaced", false},
		{"spawnedMixed", false},
		{"spawnedAsynchronous", false},
		{"spawnedBound", true},
		{"spawnedBoundNoop", false},
		{"spawnedRecursive", false},
		{"spawnedUnavailable", false},
	} {
		t.Run(test.name, func(t *testing.T) {
			function := pkg.Func(test.name)
			spawn := ssaflow.InstructionsOf[*ssa.Go](function)[0]
			proof := proveSpawnedInvocation(spawn, function.Params[0], ssaflow.NewSearchBudget(1000))
			if proof.Proven() != test.want {
				t.Fatalf("body invocation = %+v, want proven %v", proof, test.want)
			}
			if got := SpawnInvokesArgumentOnEveryReturn(spawn, function.Params[0]); got != proof.Proven() {
				t.Fatalf("spawn helper = %v, structured proof = %+v", got, proof)
			}
			// The body promise never makes its outer launch synchronous.
			if outer := ProveCompletion(CompletionRequest{
				Instruction: spawn, Target: function.Params[0], InvokeTarget: true,
				Budget: ssaflow.NewSearchBudget(1000),
			}); outer.Proven() {
				t.Fatalf("asynchronous caller completion = %+v", outer)
			}
		})
	}
}

func TestSpawnedInvocationUnknownAndBudget(t *testing.T) {
	pkg := buildTestSSA(t, exactInvocationFixture)
	function := pkg.Func("recursive")
	proof := ProveCompletion(CompletionRequest{
		Instruction: findLaunch(t, function), Target: function.Params[0], InvokeTarget: true,
		Budget: ssaflow.NewSearchBudget(1000),
	})
	if proof.State != ssaflow.EvidenceUnknown {
		t.Fatalf("ordinary recursive invocation = %+v, want unknown", proof)
	}
	for _, name := range []string{"spawnedRecursive", "spawnedUnavailable", "spawnedForwarded"} {
		function := pkg.Func(name)
		spawn := ssaflow.InstructionsOf[*ssa.Go](function)[0]
		limit := 1000
		if name == "spawnedForwarded" {
			limit = 1
		}
		proof := proveSpawnedInvocation(spawn, function.Params[0], ssaflow.NewSearchBudget(limit))
		if proof.State != ssaflow.EvidenceUnknown {
			t.Fatalf("%s proof = %+v, want unknown", name, proof)
		}
		if name == "spawnedForwarded" && proof.Reason != ssaflow.EvidenceBudgetExhausted {
			t.Fatalf("budget proof = %+v", proof)
		}
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
