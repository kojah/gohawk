package lifecycle

import (
	"go/token"
	"reflect"
	"testing"

	proofs "github.com/kojah/gohawk/internal/engine/proof"
	"github.com/kojah/gohawk/internal/engine/ssaflow"
	ssacall "github.com/kojah/gohawk/internal/engine/ssaflow/calls"
	"github.com/kojah/gohawk/internal/engine/ssaflow/ssaflowtest"
	"golang.org/x/tools/go/ssa"
)

const conditionalCompletionFixture = `package ssaflowtest
type resource struct{}
func (*resource) Close() {}
func maybe(r *resource, yes bool) bool { if yes { r.Close(); return true }; return false }
func forward(r *resource, yes bool) bool { return maybe(r, yes) }
func wrong(r, other *resource, yes bool) bool { return maybe(other, yes) }
func lie(r *resource, yes bool) bool { if yes { return true }; r.Close(); return false }
func partial(r *resource, yes, skip bool) bool { if yes { if !skip { r.Close() }; return true }; return false }
func async(r *resource, yes bool) bool { if yes { go r.Close(); return true }; return false }
func deferred(r *resource, yes bool) bool { if yes { defer r.Close(); return true }; return false }
func noMatch(r *resource) bool { r.Close(); return false }
func nilSuccess(r *resource, yes bool, err error) error { if yes { r.Close(); return nil }; return err }
func recurse(r *resource, yes bool) bool { return recurse(r, yes) }
func invoke(fn func(), yes bool) bool { if yes { fn(); return true }; return false }
func good(r *resource, yes bool) { if maybe(r, yes) { return }; r.Close() }
func storedDouble(r *resource, yes bool) { a := !maybe(r, yes); b := !a; if b { return }; r.Close() }
func storedOddFalse(r *resource, yes bool) { a := !lie(r, yes); if a { return }; r.Close() }
func storedOddLie(r *resource, yes bool) { a := !maybe(r, yes); if a { return }; r.Close() }
func forwarded(r *resource, yes bool) { if forward(r, yes) { return }; r.Close() }
func other(r, another *resource, yes bool) { if wrong(r, another, yes) { return }; r.Close() }
func falseResult(r *resource, yes bool) { if !lie(r, yes) { return }; r.Close() }
func lying(r *resource, yes bool) { if lie(r, yes) { return }; r.Close() }
func partialResult(r *resource, yes, skip bool) { if partial(r, yes, skip) { return }; r.Close() }
func launched(r *resource, yes bool) { if async(r, yes) { return }; r.Close() }
func deferResult(r *resource, yes bool) { if deferred(r, yes) { return }; r.Close() }
func impossible(r *resource) { if noMatch(r) { return }; r.Close() }
func errorResult(r *resource, yes bool, err error) { if nilSuccess(r, yes, err) == nil { return }; r.Close() }
func recursive(r *resource, yes bool) { if recurse(r, yes) { return }; r.Close() }
func cancel(fn func(), yes bool) { if invoke(fn, yes) { return }; fn() }
type failure struct{}
func (failure) Error() string { return "failed" }
func tryClose(r *resource, yes bool) error { if !yes { return failure{} }; r.Close(); return nil }
func tryForward(r *resource, yes bool) error { return tryClose(r, yes) }
func errorSuccess(r *resource, yes bool) { if tryForward(r, yes) == nil { return }; r.Close() }
func tuple(r *resource, yes bool) (int, bool) { return 1, maybe(r, yes) }
func tupleResult(r *resource, yes bool) { _, ok := tuple(r, yes); if ok { return }; r.Close() }
func panicOnly(r *resource) bool { panic("not completion") }
func panicking(r *resource) { if panicOnly(r) { return }; r.Close() }
func closeOnError(r *resource, yes bool) error { if !yes { return nil }; r.Close(); return failure{} }
func errorFailure(r *resource, yes bool) { if closeOnError(r, yes) != nil { return }; r.Close() }
type pointerError struct{}
func (*pointerError) Error() string { return "typed nil" }
func typedNil(r *resource, yes bool) error { if yes { r.Close(); return nil }; var err *pointerError; return err }
func typedNilFailure(r *resource, yes bool) { if typedNil(r, yes) != nil { return }; r.Close() }
func typedNilSuccess(r *resource, yes bool) { if typedNil(r, yes) == nil { return }; r.Close() }
`

func TestConditionalCompletion(t *testing.T) {
	pkg := buildTestSSA(t, conditionalCompletionFixture)
	for _, test := range []struct {
		name string
		want bool
	}{
		{"good", true},
		{"storedDouble", true},
		{"storedOddFalse", true},
		{"storedOddLie", false},
		{"forwarded", true},
		{"other", false},
		{"falseResult", true},
		{"lying", false},
		{"partialResult", false},
		{"launched", false},
		{"deferResult", true},
		{"impossible", false},
		{"errorResult", false},
		{"recursive", false},
		{"cancel", true},
		{"errorSuccess", true},
		{"tupleResult", true},
		{"panicking", false},
		{"errorFailure", true},
		{"typedNilFailure", false},
		{"typedNilSuccess", true},
	} {
		t.Run(test.name, func(t *testing.T) {
			fn := pkg.Func(test.name)
			branch := ssaflow.InstructionsOf[*ssa.If](fn)[0].Block()
			request := CompletionRequest{Target: fn.Params[0], Methods: []string{"Close"}, Budget: proofs.NewSearchBudget(1000)}
			if test.name == "cancel" {
				request.Methods, request.InvokeTarget = nil, true
			}
			successor := branch.Succs[0]
			if test.name == "falseResult" {
				successor = branch.Succs[1]
			}
			proof := ProveCompletionOnEdge(branch, successor, request)
			if proof.Proven() != test.want {
				t.Fatalf("proof = %+v, want proven %v", proof, test.want)
			}
		})
	}
}

func TestConditionalCompletionMemoIsolation(t *testing.T) {
	pkg := buildTestSSA(t, conditionalCompletionFixture)
	fn := pkg.Func("good")
	call := ssaflow.InstructionsOf[*ssa.Call](fn)[0]
	search := newCompletionSearch("Close", CoverageEveryReturn, proofs.NewSearchBudget(1000))
	search.exactTarget = true
	for _, test := range []struct {
		kind ssacall.Outcome
		want bool
	}{
		{ssacall.OutcomeTrue, true},
		{ssacall.OutcomeFalse, false},
		{ssacall.OutcomeAny, false},
		{ssacall.OutcomeTrue, true},
	} {
		search.condition = ssacall.CallCondition{Outcome: test.kind}
		if proven := search.completes(call, fn.Params[0]).proven; proven != test.want {
			t.Errorf("condition %v: proven %v, want %v", test.kind, proven, test.want)
		}
	}
}

func TestConditionalCompletionDoesNotBecomeUnconditional(t *testing.T) {
	pkg := buildTestSSA(t, conditionalCompletionFixture)
	fn := pkg.Func("good")
	branch := ssaflow.InstructionsOf[*ssa.If](fn)[0].Block()
	request := CompletionRequest{Target: fn.Params[0], Methods: []string{"Close"}, Budget: proofs.NewSearchBudget(1000)}
	if proof := ProveCompletionOnEdge(branch, branch.Succs[0], request); !proof.Proven() {
		t.Fatalf("true edge: %+v", proof)
	}
	if proof := ProveCompletionOnEdge(branch, branch.Succs[1], request); proof.Proven() {
		t.Fatalf("false edge: %+v", proof)
	}
	request.Instruction = ssaflow.InstructionsOf[*ssa.Call](fn)[0]
	if proof := ProveCompletion(request); proof.Proven() {
		t.Fatalf("unconditional: %+v", proof)
	}
	request.Budget = proofs.NewSearchBudget(1)
	if proof := ProveCompletionOnEdge(branch, branch.Succs[0], request); proof.State != proofs.EvidenceUnknown {
		t.Fatalf("exhausted query: %+v", proof)
	}
}

func TestMethodCoverageAllowance(t *testing.T) {
	pkg := ssaflowtest.BuildPackage(t, "coverage", `package coverage
 func action(){}
 func exact(){action()}
 func conditional(flag bool){if flag{action()}}
 func empty(){}
 func noReturn(){action();for{}}
 func assumed(p *int){if p!=nil{action()}}
 `)
	for _, test := range []struct {
		name       string
		any, every bool
	}{
		{"exact", true, true}, {"conditional", true, false}, {"empty", false, false}, {"noReturn", true, false}, {"assumed", true, true},
	} {
		for _, coverage := range []CompletionCoverage{CoverageEveryReturn, CoverageAnywhere} {
			t.Run(test.name+map[CompletionCoverage]string{CoverageEveryReturn: "Every", CoverageAnywhere: "Anywhere"}[coverage], func(t *testing.T) {
				function := pkg.Func(test.name)
				var nonNil ssa.Value
				if test.name == "assumed" {
					nonNil = function.Params[0]
				}
				calls := func(instruction ssa.Instruction) bool {
					common := ssaflow.InstructionCall(instruction)
					return common != nil && ssaflow.CallName(common) == "action"
				}
				want := test.every
				if coverage == CoverageAnywhere {
					want = test.any
				}
				if MethodCallCoverage(function, calls, coverage, nonNil) != want {
					t.Fatal("default contract changed")
				}
				for limit := 0; limit <= proofs.SummaryBudget; limit++ {
					budget := proofs.NewSearchBudget(limit)
					proof := ProveMethodCallCoverageWithin(function, calls, coverage, nonNil, budget)
					if budget.Exhausted() || limit == 0 {
						if proof.State != proofs.EvidenceUnknown || proof.Reason != proofs.EvidenceBudgetExhausted {
							t.Fatalf("cut%d=%+v", limit, proof)
						}
						continue
					}
					if proof.State == proofs.EvidenceUnknown || proof.Proven() != want {
						t.Fatalf("complete%d=%+v want%v", limit, proof, want)
					}
					return
				}
				t.Fatal("coverage never completed")
			})
		}
	}
}

func TestMethodCoverageChildAndFresh(t *testing.T) {
	pkg := ssaflowtest.BuildPackage(t, "coveragechild", `package coveragechild;func action(){};func exact(){action()}`)
	calls := func(instruction ssa.Instruction) bool {
		common := ssaflow.InstructionCall(instruction)
		return common != nil && ssaflow.CallName(common) == "action"
	}
	pool := proofs.NewSearchBudget(proofs.SummaryBudget)
	cut := ProveMethodCallCoverageWithin(pkg.Func("exact"), calls, CoverageEveryReturn, nil, pool.Within(1))
	if cut.State != proofs.EvidenceUnknown || cut.Reason != proofs.EvidenceBudgetExhausted || pool.Exhausted() {
		t.Fatalf("cut=%+v parent exhausted=%v", cut, pool.Exhausted())
	}
	fresh := ProveMethodCallCoverageWithin(pkg.Func("exact"), calls, CoverageEveryReturn, nil, pool.Within(proofs.SummaryBudget))
	if !fresh.Proven() {
		t.Fatalf("fresh=%+v", fresh)
	}
}

const completionOutcomeFixture = `package ssaflowtest
type resource struct{}
func (*resource) Close() {}
func missing(r *resource) {}
func opaque(r *resource)
func recursive(r *resource) { recursive(r) }
func cyclic(r *resource, again bool) { for again { r.Close() } }
func mixed(r *resource, again bool) { for again { r.Close() }; recursive(r) }
func callMissing(r *resource) { missing(r) }
func callOpaque(r *resource) { opaque(r) }
func callRecursive(r *resource) { recursive(r) }
func callCyclic(r *resource, again bool) { cyclic(r, again) }
func callMixed(r *resource, again bool) { mixed(r, again) }
`

// Actual searches combine unknown causes: a cycle can also contain recursive
// work, and an unavailable search can exhaust before it ever visits a body.
// The final give-up must retain the authoritative reason and provenance.
func TestCompletionUnknownReasonPriority(t *testing.T) {
	pkg := buildTestSSA(t, completionOutcomeFixture)
	for _, test := range []struct {
		name   string
		limit  int
		state  proofs.EvidenceState
		reason proofs.EvidenceReason
		local  bool
	}{
		{"callMissing", proofs.QueryBudget, proofs.EvidenceDisproven, proofs.EvidenceNotFound, true},
		{"callOpaque", proofs.QueryBudget, proofs.EvidenceUnknown, proofs.EvidenceUnavailable, false},
		{"callRecursive", proofs.QueryBudget, proofs.EvidenceUnknown, proofs.EvidenceUnavailable, true},
		{"callCyclic", proofs.QueryBudget, proofs.EvidenceUnknown, proofs.EvidenceCompletionInCycle, true},
		{"callMixed", proofs.QueryBudget, proofs.EvidenceUnknown, proofs.EvidenceCompletionInCycle, true},
		{"callMissing", 1, proofs.EvidenceUnknown, proofs.EvidenceBudgetExhausted, false},
		{"callMixed", 1, proofs.EvidenceUnknown, proofs.EvidenceBudgetExhausted, false},
		{"callOpaque", 0, proofs.EvidenceUnknown, proofs.EvidenceBudgetExhausted, false},
	} {
		fn := pkg.Func(test.name)
		instruction := findLaunch(t, fn)
		finalObservations := 0
		var reason string
		var position token.Pos
		var details map[string]string
		budget := proofs.NewSearchBudget(test.limit).Observed(func(code string, at token.Pos, data map[string]string) {
			reason, position, details = code, at, data
			if at == instruction.Pos() && data["instruction"] == instruction.String() && data["target"] == "r" && data["methods"] == "Close" {
				finalObservations++
			}
		})
		proof := ProveCompletion(CompletionRequest{Instruction: instruction, Target: fn.Params[0], Methods: []string{"Close"}, Budget: budget})
		provenance := proofs.EvidenceProvenance(0)
		if test.local {
			provenance = proofs.EvidenceFromLocalSSA
		}
		want := proofs.Proof{State: test.state, Reason: test.reason, Provenance: provenance}
		if proof.Proof != want || proof.PathKnown || proof.Path != "" {
			t.Errorf("%s(limit=%d): got %+v, want %+v", test.name, test.limit, proof, want)
		}
		wantDetails := map[string]string{"instruction": instruction.String(), "target": "r", "methods": "Close"}
		if finalObservations != 1 || reason != want.Reason.String() || position != instruction.Pos() || !reflect.DeepEqual(details, wantDetails) {
			t.Errorf("%s(limit=%d): final observation %s at %d %+v", test.name, test.limit, reason, position, details)
		}
	}
}

func TestCallbackEnvironmentMetadata(t *testing.T) {
	pkg := buildTestSSA(t, `package ssaflowtest
func subject(first, second chan int) { go func(arg chan int) { <-arg; <-first; <-second }(first) }
`)
	spawn := ssaflow.InstructionsOf[*ssa.Go](pkg.Func("subject"))[0]
	function, closure := ssacall.DirectCallee(spawn.Common())
	outer, captures := &callbackBindings{}, &callbackBindings{}
	callee := completionCallee{
		common: spawn.Common(), function: function, closure: closure,
		environment: captures, invocation: spawn,
	}
	search := newCompletionSearch("", CoverageEveryReturn, proofs.NewSearchBudget(2))
	search.bindings = outer
	key := completionKey{}
	attempts := 0
	query := func() completionAnswer {
		return search.memo.Answer(key, func() completionAnswer {
			attempts++
			return completionAnswer{proven: search.bindCallbackArguments(callee) != nil}
		})
	}
	if answer := query(); answer.proven || !search.budget.Exhausted() || !*search.incomplete {
		t.Fatalf("partial callback environment = %+v, incomplete=%v", answer, *search.incomplete)
	}
	search.budget = proofs.NewSearchBudget(3)
	if answer := query(); !answer.proven || attempts != 2 {
		t.Fatalf("cutoff poisoned memo: answer=%+v attempts=%d", answer, attempts)
	}
	search.budget = proofs.NewSearchBudget(3)
	bindings := search.bindCallbackArguments(callee)
	if bindings == nil || len(bindings.values) != 3 || search.budget.Exhausted() {
		t.Fatal("complete three-binding environment unavailable")
	}
	argument := bindings.values[function.Params[0]]
	if argument.value != spawn.Common().Args[0] || argument.bindings != outer || argument.observation != spawn {
		t.Fatalf("argument environment changed: %+v", argument)
	}
	for index, free := range function.FreeVars {
		binding := bindings.values[free]
		if binding.value != closure.Bindings[index] || binding.bindings != captures || binding.observation != spawn {
			t.Fatalf("capture environment changed: %+v", binding)
		}
	}
}

func TestEnclosingReadOnlyMetadata(t *testing.T) {
	pkg := buildTestSSA(t, `package ssaflowtest
 type wrapper struct { value int }
 func helper(first int, w *wrapper, last int) {}
 func subject(w *wrapper) { helper(1,w,2) }
`)
	value := pkg.Func("subject").Params[0]
	pool := proofs.NewSearchBudget(8)
	search := &enclosingSearch{
		request: EnclosingCompletionRequest{Budget: pool.Within(1)},
		memo:    ssacall.NewCallGraphMemo[*enclosingFrame, bool](),
	}
	if search.readOnly(value) || !search.request.Budget.Exhausted() {
		t.Fatal("partial helper-binding census proved read-only")
	}
	search.request.Budget = pool.Within(4)
	if !search.readOnly(value) || search.request.Budget.Exhausted() {
		t.Fatal("fresh allowance did not recover complete read-only evidence")
	}
	search.request.Budget = proofs.NewSearchBudget(1).Within(4)
	if search.readOnly(value) || !search.request.Budget.PoolExhausted() {
		t.Fatal("shared-pool cutoff proved read-only")
	}
}
