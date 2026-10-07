package lifecycle

import (
	"fmt"
	"strings"
	"testing"

	proofs "github.com/kojah/gohawk/internal/engine/proof"
	"github.com/kojah/gohawk/internal/engine/ssaflow"
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
				Budget: proofs.NewSearchBudget(1000),
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
			proof := ProveSpawnedInvocation(spawn, function.Params[0], proofs.NewSearchBudget(1000))
			if proof.Proven() != test.want {
				t.Fatalf("body invocation = %+v, want proven %v", proof, test.want)
			}
			// The body promise never makes its outer launch synchronous.
			if outer := ProveCompletion(CompletionRequest{
				Instruction: spawn, Target: function.Params[0], InvokeTarget: true,
				Budget: proofs.NewSearchBudget(1000),
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
		Budget: proofs.NewSearchBudget(1000),
	})
	if proof.State != proofs.EvidenceUnknown {
		t.Fatalf("ordinary recursive invocation = %+v, want unknown", proof)
	}
	for _, name := range []string{"spawnedRecursive", "spawnedUnavailable", "spawnedForwarded"} {
		function := pkg.Func(name)
		spawn := ssaflow.InstructionsOf[*ssa.Go](function)[0]
		limit := 1000
		if name == "spawnedForwarded" {
			limit = 1
		}
		proof := ProveSpawnedInvocation(spawn, function.Params[0], proofs.NewSearchBudget(limit))
		if proof.State != proofs.EvidenceUnknown {
			t.Fatalf("%s proof = %+v, want unknown", name, proof)
		}
		if name == "spawnedForwarded" && proof.Reason != proofs.EvidenceBudgetExhausted {
			t.Fatalf("budget proof = %+v", proof)
		}
	}
}

func TestExactInvocationBudgetAndCache(t *testing.T) {
	pkg := buildTestSSA(t, exactInvocationFixture)
	fn := pkg.Func("forwarded")
	request := CompletionRequest{Instruction: findLaunch(t, fn), Target: fn.Params[0], InvokeTarget: true, Budget: proofs.NewSearchBudget(1)}
	proof := ProveCompletion(request)
	if proof.State != proofs.EvidenceUnknown || proof.Reason != proofs.EvidenceBudgetExhausted {
		t.Fatalf("budget exhaustion = %+v", proof)
	}
	var evidence LocalEvidence
	request.Budget = proofs.NewSearchBudget(1000)
	if proof := evidence.Completion(request); !proof.Proven() {
		t.Fatalf("invocation = %+v", proof)
	}
	request.InvokeTarget = false
	if proof := evidence.Completion(request); proof.State != proofs.EvidenceUnknown {
		t.Fatalf("invalid method request reused invocation cache: %+v", proof)
	}
}

func TestCompletionCalleeResolutionAllowance(t *testing.T) {
	pkg := buildTestSSA(t, `package ssaflowtest
 type resource struct{}
 func (*resource) Close(){}
 func (*resource) Touch(){}
 func literal(p *resource) { defer func(){p.Close()}() }
 func alternatives(p *resource, yes bool) {
  f:=func(){p.Close()}; if yes { f=func(){p.Close()} }; defer f()
 }
 func mixed(p *resource, yes bool) {
  f:=func(){p.Close()}; if yes { f=func(){p.Touch()} }; defer f()
 }
 func opaque(p *resource, f func(), yes bool) {
  if yes { f=func(){p.Close()} }; defer f()
 }
 func stored(p *resource) {
  f:=func(){p.Close()}; keep:=func(){f()}; _=keep; defer f()
 }
 func replaced(p *resource) {
  f:=func(){p.Close()}; keep:=func(){f=func(){}}; defer keep(); defer f()
 }
 `)
	for _, test := range []struct {
		name      string
		count     int
		completes bool
	}{
		{"literal", 1, true},
		{"alternatives", 2, true},
		{"mixed", 2, false},
		{"opaque", 0, false},
		{"stored", 1, true},
		{"replaced", 0, false},
	} {
		t.Run(test.name, func(t *testing.T) {
			fn := pkg.Func(test.name)
			var launch *ssa.Defer
			for instruction := range ssaflow.InstructionsWithin(fn, nil) {
				if deferred, ok := instruction.(*ssa.Defer); ok {
					launch = deferred
				}
			}
			if launch == nil {
				t.Fatal("missing deferred invocation")
			}
			baseline, ok := resolveCallees(launch, nil)
			if ok != (test.count > 0) || len(baseline) != test.count {
				t.Fatalf("baseline targets=%d available=%v", len(baseline), ok)
			}
			request := CompletionRequest{Instruction: launch, Target: fn.Params[0], Methods: []string{"Close"}}
			if test.name == "alternatives" || test.name == "stored" {
				budget := proofs.NewSearchBudget(2)
				callbacks, resolved := exactCallbacks(launch.Common().Value, launch, false, budget)
				if resolved || len(callbacks) != 0 || !budget.Exhausted() {
					t.Fatalf("origin cutoff callbacks=%d resolved=%v exhausted=%v", len(callbacks), resolved, budget.Exhausted())
				}
			}
			proof := ProveCompletion(request)
			if proof.Proven() != test.completes {
				t.Fatalf("completion=%+v", proof)
			}
			pool := proofs.NewSearchBudget(200000)
			assertCompletionCalleeCutoffs(t, launch, baseline, ok, pool)
			request.Budget = pool.Within(proofs.QueryBudget)
			freshProof := ProveCompletion(request)
			if request.Budget.Exhausted() || freshProof.Proof != proof.Proof {
				t.Fatalf("fresh completion=%+v baseline=%+v", freshProof, proof)
			}
		})
	}
}

func assertCompletionCallees(t *testing.T, got []completionCallee, available bool, want []completionCallee, wantAvailable bool) {
	t.Helper()
	if available != wantAvailable || len(got) != len(want) {
		t.Fatalf("targets=%d/%d available=%v/%v", len(got), len(want), available, wantAvailable)
	}
	for i := range got {
		if got[i] != want[i] {
			t.Fatalf("target %d changed identity, order or launch", i)
		}
	}
}

func TestCompletionPaddedCalleeOrigins(t *testing.T) {
	var source strings.Builder
	source.WriteString(`package ssaflowtest
type resource struct{}
func (*resource) Close(){}
func padded(p *resource, bits uint64){
f:=func(){p.Close()}
`)
	for i := range 40 {
		fmt.Fprintf(&source, "if bits & (1<<%d) != 0 { f=func(){p.Close()} }\n", i)
	}
	source.WriteString("defer f()\n}\n")
	pkg := buildTestSSA(t, source.String())
	fn := pkg.Func("padded")
	var launch *ssa.Defer
	for instruction := range ssaflow.InstructionsWithin(fn, nil) {
		if deferred, ok := instruction.(*ssa.Defer); ok {
			launch = deferred
		}
	}
	baseline, ok := resolveCallees(launch, nil)
	if !ok || len(baseline) != 41 {
		t.Fatalf("baseline targets=%d available=%v", len(baseline), ok)
	}
	pool := proofs.NewSearchBudget(200000)
	originsBudget := pool.Within(30)
	callbacks, resolved := exactCallbacks(launch.Common().Value, launch, false, originsBudget)
	if resolved || len(callbacks) != 0 || !originsBudget.Exhausted() {
		t.Fatalf("padded origin cutoff callbacks=%d resolved=%v exhausted=%v", len(callbacks), resolved, originsBudget.Exhausted())
	}
	child := pool.Within(30)
	targets, available := resolveCallees(launch, child)
	if available || len(targets) != 0 || !child.Exhausted() || pool.Exhausted() {
		t.Fatalf("cut targets=%d available=%v exhausted=%v", len(targets), available, child.Exhausted())
	}
	targets, available = resolveCallees(launch, pool.Within(proofs.SummaryBudget))
	assertCompletionCallees(t, targets, available, baseline, ok)
	request := CompletionRequest{Instruction: launch, Target: fn.Params[0], Methods: []string{"Close"}, Budget: pool.Within(30)}
	if proof := ProveCompletion(request); proof.Proven() || proof.Reason != proofs.EvidenceBudgetExhausted {
		t.Fatalf("cut proof=%+v", proof)
	}
}

func assertCompletionCalleeCutoffs(t *testing.T, launch ssa.Instruction, baseline []completionCallee, ok bool, pool *proofs.SearchBudget) {
	t.Helper()

	finished := false
	for limit := range proofs.SummaryBudget {
		child := pool.Within(limit)
		targets, available := resolveCallees(launch, child)
		if child.Exhausted() {
			if available || len(targets) != 0 {
				t.Fatalf("cut%d publishes %d targets, available=%v", limit, len(targets), available)
			}
		} else {
			assertCompletionCallees(t, targets, available, baseline, ok)
			finished = true
		}
		fresh := pool.Within(proofs.SummaryBudget)
		targets, available = resolveCallees(launch, fresh)
		if fresh.Exhausted() || pool.Exhausted() {
			t.Fatal("fresh or parent exhausted")
		}
		assertCompletionCallees(t, targets, available, baseline, ok)
		if finished {
			break
		}
	}
	if !finished {
		t.Fatal("resolution never completed")
	}
}

func TestEnclosingCallbackCompletion(t *testing.T) {
	pkg := buildTestSSA(t, `package ssaflowtest
import "testing"
type db struct{}
func (*db) Close() {}
func (*db) Use() {}
type wrapper struct { db *db }
func (w *wrapper) observe() { w.db.Use() }
func run(t *testing.T, callbacks ...func(*wrapper)) {
 for _, fn := range callbacks {
  fn := fn
  t.Run("case", func(t *testing.T) {
   database := new(db)
   t.Cleanup(func(){ database.Close() })
   fn(&wrapper{database})
  })
 }
}
func good(t *testing.T) { run(t, func(w *wrapper){ w.observe(); w.db.Use() }) }
func without(fn func(*wrapper)) { fn(&wrapper{new(db)}) }
func missing() { without(func(w *wrapper){ w.db.Use() }) }
func wrongDB(fn func(*wrapper)) { database := new(db); defer database.Close(); fn(&wrapper{new(db)}) }
func wrong() { wrongDB(func(w *wrapper){ w.db.Use() }) }
func mutated(t *testing.T) { run(t, func(w *wrapper){ w.db = new(db); w.db.Use() }) }
var saved func(*wrapper)
func escaping(fn func(*wrapper)) { database := new(db); defer database.Close(); fn(&wrapper{database}); saved = fn }
func escape() { escaping(func(w *wrapper){ w.db.Use() }) }
func deferredDB(fn func(*wrapper)) { database := new(db); defer database.Close(); fn(&wrapper{database}) }
func direct() { deferredDB(func(w *wrapper){ w.db.Use() }) }
func maybeCleanup(t *testing.T, yes bool, fn func(*wrapper)) {
 database := new(db); other := new(db); selected := database
 if yes { selected = other }
 t.Cleanup(func(){ selected.Close() }); fn(&wrapper{database})
}
func ambiguous(t *testing.T, yes bool) { maybeCleanup(t, yes, func(w *wrapper){ w.db.Use() }) }
func partialCleanup(t *testing.T, yes bool, fn func(*wrapper)) {
 database := new(db); if yes { t.Cleanup(func(){ database.Close() }) }; fn(&wrapper{database})
}
func conditional(t *testing.T, yes bool) { partialCleanup(t, yes, func(w *wrapper){ w.db.Use() }) }
func twoCalls(fn func(*wrapper)) { database := new(db); defer database.Close(); fn(&wrapper{database}); fn(&wrapper{new(db)}) }
func secondUnowned() { twoCalls(func(w *wrapper){ w.db.Use() }) }
var callbacks = map[int]func(*wrapper){}
func keepInMap(fn func(*wrapper)) { database := new(db); defer database.Close(); fn(&wrapper{database}); callbacks[0] = fn }
func mapEscape() { keepInMap(func(w *wrapper){ w.db.Use() }) }
func loopDB(t *testing.T, fn func(*wrapper)) {
 var database *db
 for i:=0; i<2; i++ { database = new(db); t.Cleanup(func(){ database.Close() }); fn(&wrapper{database}) }
}
func reusedCell(t *testing.T) { loopDB(t, func(w *wrapper){ w.db.Use() }) }
`)
	for _, test := range []struct {
		name   string
		proven bool
	}{
		{"good", true},
		{"missing", false},
		{"wrong", false},
		{"mutated", false},
		{"escape", false},
		{"direct", true},
		{"ambiguous", false},
		{"conditional", false},
		{"secondUnowned", false},
		{"mapEscape", false},
		{"reusedCell", false},
	} {
		t.Run(test.name, func(t *testing.T) {
			fn := pkg.Func(test.name).AnonFuncs[0]
			var receiver ssa.Value
			for _, call := range ssaflow.InstructionsOf[*ssa.Call](fn) {
				if ssaflow.CallName(call.Common()) == "Use" {
					receiver = ssaflow.CallReceiver(call.Common())
				}
			}
			proof := ProveEnclosingCompletion(EnclosingCompletionRequest{
				Function: fn, Value: receiver, Methods: []string{"Close"}, Budget: proofs.NewSearchBudget(10000),
			})
			if proof.Proven() != test.proven {
				t.Fatalf("proof = %+v, want proven %v", proof, test.proven)
			}
			limited := ProveEnclosingCompletion(EnclosingCompletionRequest{
				Function: fn, Value: receiver, Methods: []string{"Close"}, Budget: proofs.NewSearchBudget(1),
			})
			if limited.State != proofs.EvidenceUnknown || limited.Reason != proofs.EvidenceBudgetExhausted {
				t.Fatalf("exhausted proof = %+v", limited)
			}
		})
	}
}
