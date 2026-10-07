package lifecycle

import (
	"strings"
	"testing"

	proofs "github.com/kojah/gohawk/internal/engine/proof"
	"github.com/kojah/gohawk/internal/engine/ssaflow"
	ssacall "github.com/kojah/gohawk/internal/engine/ssaflow/calls"
	"github.com/kojah/gohawk/internal/engine/ssaflow/ssaflowtest"
	"golang.org/x/tools/go/ssa"
)

func TestExactCallbackAlternatives(t *testing.T) {
	callback := &ssa.MakeClosure{}
	shared := &ssa.ChangeType{X: callback}
	cycle := &ssa.Phi{}
	cycle.Edges = []ssa.Value{callback, cycle}
	for _, test := range []struct {
		name  string
		value ssa.Value
		count int
	}{
		{"literal", callback, 1},
		{"shared branches", &ssa.Phi{Edges: []ssa.Value{shared, shared}}, 2},
		{"nested branches", &ssa.Phi{Edges: []ssa.Value{shared, &ssa.Phi{Edges: []ssa.Value{callback, shared}}}}, 3},
		{"opaque alternative", &ssa.Phi{Edges: []ssa.Value{callback, &ssa.Parameter{}}}, 0},
		{"cyclic alternative", cycle, 0},
		{"empty merge", &ssa.Phi{}, 0},
		{"missing value", nil, 0},
	} {
		t.Run(test.name, func(t *testing.T) {
			closures, ok := exactCallbacks(test.value, nil, false, nil)
			if ok != (test.count > 0) || len(closures) != test.count {
				t.Fatalf("exactCallbacks() returned %d callbacks, resolved=%t; want %d", len(closures), ok, test.count)
			}
			for _, closure := range closures {
				if closure != callback {
					t.Error("resolved a different closure")
				}
			}
		})
	}
}

func TestStoredCallbackCompletionAllowance(t *testing.T) {
	pkg := buildTestSSA(t, callbackBindingsFixture)
	for _, name := range []string{"field", "element", "dynamic"} {
		t.Run(name, func(t *testing.T) {
			function := pkg.Func(name)
			var dump strings.Builder
			if _, err := function.WriteTo(&dump); err != nil {
				t.Fatal(err)
			}
			t.Log(dump.String())
			request := CompletionRequest{Instruction: findLaunch(t, function), Target: function.Params[0], Methods: []string{"Close"}}
			complete := ProveCompletion(request)
			if !complete.Proven() {
				t.Fatalf("complete callback %+v", complete)
			}
			for limit := range 1000 {
				pool := proofs.NewSearchBudget(10000)
				request.Budget = pool.Within(limit)
				proof := ProveCompletion(request)
				if request.Budget.Exhausted() {
					if proof.Proven() || proof.Reason != proofs.EvidenceBudgetExhausted || pool.Exhausted() {
						t.Fatalf("cut%d supplied callback: %+v", limit, proof)
					}
					request.Budget = pool.Within(1000)
					if fresh := ProveCompletion(request); fresh != complete {
						t.Fatalf("fresh callback %+v want %+v", fresh, complete)
					}
					continue
				}
				if proof != complete {
					t.Fatalf("complete callback %+v want %+v", proof, complete)
				}
				return
			}
			t.Fatal("callback never completes")
		})
	}
}

func TestCallbackCapabilityPoliciesAndAllowance(t *testing.T) {
	pkg := buildTestSSA(t, `package ssaflowtest
 import "sync"
 func carry(cb func())func()
 func direct(mu *sync.Mutex)func(){return mu.Unlock}
 func literal(mu *sync.Mutex)func(){return func(){mu.Unlock()}}
 func mixed(mu *sync.Mutex,flag bool)func(){cb:=func(){};if flag{cb=mu.Unlock};return cb}
 func wrapped(mu *sync.Mutex)func(){return carry(mu.Unlock)}
 func boxed(mu *sync.Mutex)any{return mu.Unlock}
 func stored(mu *sync.Mutex)func(){var cb func();cb=mu.Unlock;defer func(){}();return cb}
 func different(mu,other *sync.Mutex)func(){return other.Unlock}
 func unknown(mu *sync.Mutex,cb func())func(){return cb}
 func recursive(mu *sync.Mutex)func(){var cb func();cb=func(){cb()};return cb}
 `)
	for _, test := range []struct {
		name string
		may  bool
	}{
		{"direct", true},
		{"literal", true},
		{"mixed", true},
		{"wrapped", true},
		{"boxed", true},
		{"stored", true},
		{"different", false},
		{"unknown", false},
		{"recursive", false},
	} {
		fn := pkg.Func(test.name)
		value := returnedValue(t, fn)
		proof := ProveValueCallsMethodWithin(value, "Unlock", fn.Params[0], proofs.NewSearchBudget(proofs.SummaryBudget))
		if proof.Proven() != test.may || ValueCallsMethod(value, "Unlock", fn.Params[0]) != test.may {
			t.Fatalf("%s policy: %+v", test.name, proof)
		}
		zero := proofs.NewSearchBudget(0)
		if proof := ProveValueCallsMethodWithin(value, "Unlock", fn.Params[0], zero); proof.Proven() ||
			proof.Reason != proofs.EvidenceBudgetExhausted || !zero.Exhausted() {
			t.Fatalf("%s zero allowance: %+v", test.name, proof)
		}
		if !test.may {
			continue
		}
		finished := false
		for limit := range proofs.SummaryBudget {
			pool := proofs.NewSearchBudget(100 * proofs.SummaryBudget)
			child := pool.Within(limit)
			proof := ProveValueCallsMethodWithin(value, "Unlock", fn.Params[0], child)
			if proof.Proven() {
				if child.Exhausted() || proof.Reason != proofs.EvidenceCallbackCompletion {
					t.Fatalf("%s proved interrupted capability: %+v", test.name, proof)
				}
				finished = true
				break
			}
			if proof.Reason != proofs.EvidenceBudgetExhausted || !child.Exhausted() || pool.Exhausted() {
				t.Fatalf("%s cut%d: %+v exhausted=%v/%v", test.name, limit, proof, child.Exhausted(), pool.Exhausted())
			}
			if !ProveValueCallsMethodWithin(value, "Unlock", fn.Params[0], pool.Within(proofs.SummaryBudget)).Proven() {
				t.Fatalf("%s fresh query fails", test.name)
			}
		}
		if !finished {
			t.Fatalf("%s capability never completes", test.name)
		}
	}
}

func TestCallbackCapabilityNestedCoverageChargesAllowance(t *testing.T) {
	pkg := buildTestSSA(t, `package ssaflowtest
 import "sync"
 var total int
 func padded(mu *sync.Mutex,n int)func(){return func(){`+strings.Repeat("n+=n;total=n\n", 60)+`mu.Unlock()}}
 `)
	fn := pkg.Func("padded")
	value := returnedValue(t, fn)
	pool := proofs.NewSearchBudget(100 * proofs.SummaryBudget)
	child := pool.Within(30)
	proof := ProveValueCallsMethodWithin(value, "Unlock", fn.Params[0], child)
	if proof.Proven() || proof.Reason != proofs.EvidenceBudgetExhausted || !child.Exhausted() || pool.Exhausted() {
		t.Fatalf("nested coverage bypassed allowance: %+v exhausted=%v/%v", proof, child.Exhausted(), pool.Exhausted())
	}
	// Mapping the padded numeric capture now shares this allowance too.
	// The thirty-step cutoff remains the coverage control; recovery gets
	// a separate test allowance without changing any production limit.
	fresh := pool.Within(2 * proofs.SummaryBudget)
	if proof := ProveValueCallsMethodWithin(value, "Unlock", fn.Params[0], fresh); !proof.Proven() {
		t.Fatalf("fresh nested coverage=%+v, exhausted=%v/%v", proof, fresh.Exhausted(), pool.Exhausted())
	}
}

func TestCallbackOriginRevisitInvalidatesEnclosingMemo(t *testing.T) {
	pkg := buildTestSSA(t, `package ssaflowtest
 import "sync"
 func callback(mu *sync.Mutex)func(){return mu.Unlock}
 `)
	fn := pkg.Func("callback")
	value, target := returnedValue(t, fn), fn.Params[0]
	search := newCompletionSearch("Unlock", CoverageEveryReturn, proofs.NewSearchBudget(proofs.SummaryBudget))
	if !search.valueCallsMethod(value, target) {
		t.Fatal("initial callback capability not found")
	}
	key := completionKey{target: target}
	attempts := 0
	for range 2 {
		answer := search.memo.Answer(key, func() completionAnswer {
			attempts++
			return completionAnswer{proven: search.valueCallsMethod(value, target)}
		})
		if answer.proven {
			t.Fatal("a revisited origin supplies new capability evidence")
		}
	}
	if attempts != 2 {
		t.Fatal("memo retained an answer shortened by the origin guard")
	}
}

func TestDeferredHelperCallbackBindings(t *testing.T) {
	pkg := buildTestSSA(t, `
package ssaflowtest
func invoke(fn func()) { fn() }
func ignore(fn func()) {}
func generic[T any](fn func(), value T) { invoke(fn) }
func captured(fn func()) { defer func() { invoke(fn) }() }
func argument(fn func()) { defer func(value func()) { invoke(value) }(fn) }
func mixed(fn, other func()) { defer func(value func()) { ignore(other); invoke(value) }(fn) }
func wrongCapture(fn, other func()) { defer func() { invoke(other) }() }
func wrongArgument(fn, other func()) { defer func(value func()) { invoke(value) }(other) }
func ignored(fn func()) { defer func() { ignore(fn) }() }
func instantiated(fn func()) { defer generic(fn, 1) }
func ordinaryCall(fn func()) { func() { invoke(fn) }() }
`)
	for _, test := range []struct {
		name string
		want bool
	}{
		// This broad handoff helper does not prove invocation through a
		// captured cell's load. The classifier has a separate unknown
		// capture boundary; sharing bindings must not strengthen this answer.
		{"captured", false},
		{"argument", true},
		{"mixed", true},
		{"wrongCapture", false},
		{"wrongArgument", false},
		{"ignored", false},
		{"instantiated", true},
		{"ordinaryCall", false},
	} {
		t.Run(test.name, func(t *testing.T) {
			function := pkg.Func(test.name)
			var instruction ssa.Instruction
			if deferred := ssaflow.InstructionsOf[*ssa.Defer](function); len(deferred) != 0 {
				instruction = deferred[0]
			} else {
				instruction = ssaflow.InstructionsOf[*ssa.Call](function)[0]
			}
			if got := DeferredClosureInvokesArgumentOnEveryReturn(instruction, function.Params[0]); got != test.want {
				t.Fatalf("deferred helper recognizes callback = %v, want %v", got, test.want)
			}
		})
	}
}

const completionCaptureTimingFixture = `
 func beforeCapture(p *resource){done:=true;defer func(){if done{p.Close()}}()}
 func earlyCapture(p *resource,early bool){var done bool;defer func(){if done{p.Close()}}();if early{return};done=true}
 func preStoreCapture(p *resource){var done bool;func(){if done{p.Close()}}();done=true}
 func lateCapture(p *resource){var done bool;f:=func(){if done{p.Close()}};done=true;f()}
 func namedCapture(p *resource)(done bool){defer func(){if done{p.Close()}}();return true}
`

func TestCompletionCapturedOutcomeTiming(t *testing.T) {
	pkg := buildTestSSA(t, completionCoverageBudgetFixture+completionCaptureTimingFixture)
	for _, test := range []struct {
		name  string
		known ssacall.Outcome
		want  bool
	}{
		{"beforeCapture", ssacall.OutcomeAny, true},
		{"earlyCapture", ssacall.OutcomeAny, false},
		{"preStoreCapture", ssacall.OutcomeAny, false},
		{"lateCapture", ssacall.OutcomeAny, false},
		{"lateCapture", ssacall.OutcomeTrue, true},
		{"namedCapture", ssacall.OutcomeAny, false},
		{"namedCapture", ssacall.OutcomeTrue, true},
		{"namedCapture", ssacall.OutcomeFalse, false},
	} {
		t.Run(test.name, func(t *testing.T) {
			fn := pkg.Func(test.name)
			call := findLaunch(t, fn)
			body, closure := ssacall.DirectCallee(ssaflow.InstructionCall(call))
			var dump strings.Builder
			if _, err := fn.WriteTo(&dump); err != nil {
				t.Fatal(err)
			}
			if _, err := body.WriteTo(&dump); err != nil {
				t.Fatal(err)
			}
			t.Log(dump.String())
			request := CompletionRequest{Instruction: call, Target: fn.Params[0], Methods: []string{"Close"}}
			if test.known != ssacall.OutcomeAny {
				request.Constants = ssacall.FixedValues{}
				for pair := range ssaflow.ClosureBindingPairsWithin(body, closure, nil) {
					if pair.Free.Name() == "done" {
						request.Constants[pair.Binding] = test.known
					}
				}
				if len(request.Constants) != 1 {
					t.Fatal("missing exact done capture")
				}
			}
			proof := ProveCompletion(request)
			if proof.Proven() != test.want {
				t.Fatalf("completion=%+v want proven=%v", proof, test.want)
			}
		})
	}
}

func TestPossibleClosureCapture(t *testing.T) {
	pkg := ssaflowtest.BuildPackage(t, "choicecapture", `package choicecapture
func register(func())
type callback func()
func mixed(ch, other chan int, flag bool) { var fn func()
if flag { fn = func() { <-ch } } else { fn = func() { <-other } }; register(fn) }
func unrelated(ch, other chan int, flag bool) { var fn func()
if flag { fn = func() { <-other } } else { fn = func() {} }; register(fn) }
func converted(ch, other chan int, flag bool) { var fn func()
if flag { fn = func() { <-ch } } else { fn = func() { <-other } }; go callback(fn)() }
`)
	for _, test := range []struct {
		name   string
		reason proofs.EvidenceReason
	}{
		{"mixed", proofs.EvidenceCapturedByClosure},
		{"unrelated", proofs.EvidenceNotFound},
		{"converted", proofs.EvidenceNotFound},
	} {
		fn := pkg.Func(test.name)
		var value ssa.Value
		if test.name == "converted" {
			value = ssaflow.InstructionsOf[*ssa.Go](fn)[0].Common().Value
		} else {
			value = ssaflow.InstructionsOf[*ssa.Call](fn)[0].Common().Args[0]
		}
		pool := proofs.NewSearchBudget(4 * proofs.QueryBudget)
		cut := ProvePossibleClosureCaptureWithin(value, fn.Params[0], pool.Within(0))
		if cut.State != proofs.EvidenceUnknown || cut.Reason != proofs.EvidenceBudgetExhausted || pool.Exhausted() {
			t.Fatalf("%s cutoff: %+v", test.name, cut)
		}
		fresh := ProvePossibleClosureCaptureWithin(value, fn.Params[0], pool.Within(proofs.QueryBudget))
		want := proofs.EvidenceDisproven
		if test.name == "mixed" {
			want = proofs.EvidenceProven
		}
		if fresh.Reason != test.reason || fresh.State != want {
			t.Fatalf("%s fresh: %+v", test.name, fresh)
		}
	}
}
