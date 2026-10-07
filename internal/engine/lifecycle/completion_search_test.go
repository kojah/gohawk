package lifecycle

import (
	"testing"

	proofs "github.com/kojah/gohawk/internal/engine/proof"
	"github.com/kojah/gohawk/internal/engine/ssaflow"
	ssacall "github.com/kojah/gohawk/internal/engine/ssaflow/calls"
	ssapath "github.com/kojah/gohawk/internal/engine/ssaflow/path"
	"golang.org/x/tools/go/ssa"
)

const completionFixture = `
package ssaflowtest

import (
	"sync"
	"testing"
)

type closer struct{}

func (*closer) Close() {}

type owner struct{ body *closer }

var stored func()

func closeHelper(value *closer)                  { value.Close() }
func closeOwner(value *owner)                    { value.body.Close() }
func closeOther(other, value *closer)            { other.Close() }
func closeMaybe(value *closer, enabled bool)     { if enabled { value.Close() } }
func closeChain(value *closer)                   { closeHelper(value) }
func closeLater(value *closer)                   { defer closeHelper(value) }
func invoke(callback func())                     { callback() }
func maybeInvoke(callback func(), enabled bool)  { if enabled { callback() } }
func startWaiter(value *closer)                  { go func() { value.Close() }() }
func register(t *testing.T, value *closer)       { t.Cleanup(func() { value.Close() }) }
func opaque(value *closer, callback func(*closer)) { callback(value) }

func deferredLiteral(value *closer)            { defer func() { value.Close() }() }
func deferredHelper(value *closer)             { defer closeHelper(value) }
func deferredConditional(value *closer, ok bool) { defer func() { if ok { value.Close() } }() }
func deferredOtherReceiver(value, other *closer) { defer func() { other.Close() }() }
func deferredReassignedAfter(value, other *closer) {
	defer func() { value.Close() }()
	value = other
}
func deferredReassignedBefore(value, other *closer) {
	other = value
	defer func() { other.Close() }()
}
func deferredInsideLoop(value *closer) {
	for range 3 {
		current := value
		defer func() { current.Close() }()
	}
}
func deferredAssignedInBranches(value, other *closer, pick bool) {
	var current *closer
	if pick {
		current = value
	} else {
		current = other
	}
	defer func() { current.Close() }()
}
func deferredClearedAfterUse(value *closer, ok bool) {
	var current *closer = value
	defer func() {
		if current != nil {
			current.Close()
		}
	}()
	if ok {
		current = nil
	}
}
func deferredStoredCallback(value *closer) {
	callback := func() { value.Close() }
	defer callback()
}
func deferredOnceCallback(value *closer) {
	callback := sync.OnceFunc(func() { value.Close() })
	defer callback()
}
func deferredPhiCallback(value *closer, unlock bool) {
	callback := func() {}
	if unlock {
		callback = func() { value.Close() }
	}
	defer callback()
}
func deferredPhiBothCallbacks(value *closer, pick bool) {
	callback := func() { value.Close() }
	if pick {
		callback = func() { closeHelper(value) }
	}
	defer callback()
}
func deferredProjection(value *owner)          { defer closeHelper(value.body) }
func deferredProjectionLiteral(value *owner)   { defer func(body *closer) { body.Close() }(value.body) }
func deferredProjectionOther(value, other *owner) { defer closeOther(other.body, value.body) }
func deferredOwnerCapture(value *owner)        { defer func() { value.body.Close() }() }
func deferredBoundCallback(value *closer)      { defer invoke(value.Close) }
func deferredBoundConditional(value *closer, ok bool) { defer maybeInvoke(value.Close, ok) }
func calledHelper(value *closer)               { closeHelper(value) }
func calledOwner(value *closer)                { closeOwner(&owner{body: value}) }
func calledConditional(value *closer, ok bool) { closeMaybe(value, ok) }
func calledChain(value *closer)                { closeChain(value) }
func calledDeferringHelper(value *closer)      { closeLater(value) }
func calledLiteral(value *closer)              { func() { value.Close() }() }
func calledStarter(value *closer)              { startWaiter(value) }
func calledRegistrar(t *testing.T, value *closer) { register(t, value) }
func calledOpaque(value *closer, callback func(*closer)) { opaque(value, callback) }
func startInvoker(callback func()) {
	go func() { callback() }()
}
func storeInvoker(callback func()) { stored = callback }
func calledStartedCallback(value *closer)  { startInvoker(value.Close) }
func calledStoredCallback(value *closer)   { storeInvoker(value.Close) }
func startedLiteral(value *closer)             { go func() { value.Close() }() }
func startedGroup(value *closer) {
	var group sync.WaitGroup
	group.Go(func() { value.Close() })
	group.Wait()
}
func registeredCleanup(t *testing.T, value *closer) { t.Cleanup(func() { value.Close() }) }
func recursiveLiteral(value *closer) {
	var walk func(int)
	walk = func(depth int) {
		if depth > 0 {
			walk(depth - 1)
		}
		value.Close()
	}
	defer walk(3)
}
func selfCapturingCallback(value *closer) {
	var callback func()
	callback = func() { _ = callback; value.Close() }
	defer callback()
}
func onceCalledNow(value *closer) {
	callback := sync.OnceFunc(func() { value.Close() })
	callback()
}
`

var completionCases = []struct {
	function string
	proven   bool
	reason   proofs.EvidenceReason
	coverage CompletionCoverage
	unknown  bool
}{
	{function: "deferredLiteral", proven: true, reason: proofs.EvidenceDeferredCompletion},
	{function: "deferredHelper", proven: true, reason: proofs.EvidenceDeferredCompletion},
	{function: "deferredConditional"},
	{function: "deferredConditional", proven: true, reason: proofs.EvidenceDeferredCompletion, coverage: CoverageAnywhere},
	{function: "deferredOtherReceiver"},
	{function: "deferredReassignedAfter"},
	{function: "deferredReassignedBefore", proven: true, reason: proofs.EvidenceDeferredCompletion},
	{function: "deferredInsideLoop", proven: true, reason: proofs.EvidenceDeferredCompletion},
	{function: "deferredAssignedInBranches", proven: true, reason: proofs.EvidenceDeferredCompletion},
	{function: "deferredClearedAfterUse", proven: true, reason: proofs.EvidenceDeferredCompletion, coverage: CoverageAnywhere},
	{function: "deferredClearedAfterUse"},
	{function: "deferredStoredCallback", proven: true, reason: proofs.EvidenceDeferredCompletion},
	{function: "deferredOnceCallback", proven: true, reason: proofs.EvidenceDeferredCompletion},
	{function: "deferredPhiCallback"},
	{function: "deferredPhiBothCallbacks", proven: true, reason: proofs.EvidenceDeferredCompletion},
	{function: "deferredProjection", proven: true, reason: proofs.EvidenceDeferredCompletion},
	{function: "deferredProjectionLiteral", proven: true, reason: proofs.EvidenceDeferredCompletion},
	{function: "deferredProjectionOther"},
	{function: "deferredOwnerCapture", proven: true, reason: proofs.EvidenceDeferredCompletion},
	{function: "deferredBoundCallback", proven: true, reason: proofs.EvidenceDeferredCompletion},
	{function: "deferredBoundConditional"},
	{function: "calledHelper", proven: true, reason: proofs.EvidenceCalledCompletion},
	{function: "calledOwner", proven: true, reason: proofs.EvidenceCalledCompletion},
	{function: "calledConditional"},
	{function: "calledChain", proven: true, reason: proofs.EvidenceCalledCompletion},
	{function: "calledDeferringHelper", proven: true, reason: proofs.EvidenceCalledCompletion},
	{function: "calledLiteral", proven: true, reason: proofs.EvidenceCalledCompletion},
	{function: "calledStarter", proven: true, reason: proofs.EvidenceCalledCompletion},
	{function: "calledRegistrar", proven: true, reason: proofs.EvidenceCalledCompletion},
	{function: "calledOpaque"},
	{function: "calledStartedCallback", proven: true, reason: proofs.EvidenceCalledCompletion},
	{function: "calledStoredCallback"},
	{function: "startedLiteral", proven: true, reason: proofs.EvidenceStartedCompletion},
	{function: "startedGroup", proven: true, reason: proofs.EvidenceStartedCompletion},
	{function: "registeredCleanup", proven: true, reason: proofs.EvidenceDeferredCompletion},
	// A literal that captures the variable holding itself must terminate the
	// search; the deferred call still completes the target on every return.
	{function: "recursiveLiteral", proven: true, reason: proofs.EvidenceDeferredCompletion},
	{function: "selfCapturingCallback", proven: true, reason: proofs.EvidenceDeferredCompletion},
	{function: "onceCalledNow", unknown: true},
}

// The completion engine has one search; these cases pin its boundaries by
// launch form (deferred, called, started, registered), by how the target is
// mapped into the callee (exact, projection, owner, callback), and by the
// coverage the caller asks for.
func TestCompletionBoundaries(t *testing.T) {
	pkg := buildTestSSA(t, completionFixture)
	for _, test := range completionCases {
		name := test.function
		if test.coverage == CoverageAnywhere {
			name += "/anywhere"
		}
		t.Run(name, func(t *testing.T) {
			function := pkg.Func(test.function)
			target := function.Params[0]
			if len(function.Params) > 1 && function.Params[0].Name() == "t" {
				target = function.Params[1]
			}
			instruction := findLaunch(t, function)
			proof := ProveCompletion(CompletionRequest{
				Instruction: instruction,
				Target:      target,
				Methods:     []string{"Close"},
				Coverage:    test.coverage,
			})
			if test.unknown {
				if proof.State != proofs.EvidenceUnknown {
					t.Fatalf("ProveCompletion() = %#v, want unknown", proof)
				}
				return
			}
			if proof.Proven() != test.proven {
				t.Fatalf("ProveCompletion() = %#v, want proven %v", proof, test.proven)
			}
			if test.proven && (proof.Reason != test.reason || proof.Method != "Close") {
				t.Fatalf("ProveCompletion() reason = %q, want %q with method Close", proof.Reason, test.reason)
			}
		})
	}
}

// findLaunch returns the function's defer or go statement, or else its first
// call to a non-builtin function, which is the launch under test.
func findLaunch(t *testing.T, function *ssa.Function) ssa.Instruction { //nolint:ireturn // SSA launches have several instruction forms.
	t.Helper()
	var call ssa.Instruction
	for _, block := range function.Blocks {
		for _, instruction := range block.Instrs {
			switch typed := instruction.(type) {
			case *ssa.Defer, *ssa.Go:
				return instruction
			case *ssa.Call:
				_, builtin := typed.Common().Value.(*ssa.Builtin)
				if call == nil && !builtin && ssaflow.CallName(typed.Common()) != "Wait" {
					call = instruction
				}
			}
		}
	}
	if call == nil {
		t.Fatalf("no launch found in %s", function.Name())
	}
	return call
}

const completionCoverageBudgetFixture = `package ssaflowtest
 type resource struct{}
 func(*resource)Close(){}
 func finish(p *resource){p.Close()}
 func maybe(p *resource,yes bool){if yes{p.Close()}}
 func nested(p *resource,yes bool){maybe(p,yes)}
 func typed(x interface{}){if p,ok:=x.(*resource);ok{p.Close()}}
 func readOnly(p *resource){}
 func caseTrue(p *resource,yes bool)bool{if yes{p.Close();return true};return false}
 func argumentCase(p *resource,yes bool){if yes{p.Close()}}
 func runExact(p *resource){finish(p)}
 func runTrue(p *resource){maybe(p,true)}
 func runFalse(p *resource){maybe(p,false)}
 func runNested(p *resource){nested(p,true)}
 func runType(p *resource){typed(p)}
 func runMissing(p *resource){readOnly(p)}
`

func TestCompletionCoverageSearchAllowance(t *testing.T) {
	pkg := buildTestSSA(t, completionCoverageBudgetFixture)
	for _, test := range []struct {
		name string
		want bool
	}{
		{"runExact", true}, {"runTrue", true}, {"runFalse", false}, {"runNested", true}, {"runType", true}, {"runMissing", false},
	} {
		t.Run(test.name, func(t *testing.T) {
			fn := pkg.Func(test.name)
			request := CompletionRequest{Instruction: findLaunch(t, fn), Target: fn.Params[0], Methods: []string{"Close"}}
			baseline := ProveCompletion(request)
			if baseline.Proven() != test.want {
				t.Fatalf("default=%+v", baseline)
			}
			for limit := 0; limit <= proofs.SummaryBudget; limit++ {
				request.Budget = proofs.NewSearchBudget(limit)
				got := ProveCompletion(request)
				if request.Budget.Exhausted() {
					if got.State != proofs.EvidenceUnknown || got.Reason != proofs.EvidenceBudgetExhausted {
						t.Fatalf("cut%d=%+v", limit, got)
					}
					continue
				}
				if got.State == proofs.EvidenceUnknown || got.Proven() != test.want {
					t.Fatalf("complete%d=%+v want%v", limit, got, test.want)
				}
				return
			}
			t.Fatal("query never completed")
		})
	}
}

func TestCompletionCoverageMemoChildAndFresh(t *testing.T) {
	pkg := buildTestSSA(t, completionCoverageBudgetFixture)
	fn := pkg.Func("runNested")
	call := findLaunch(t, fn)
	pool := proofs.NewSearchBudget(proofs.SummaryBudget)
	search := newCompletionSearch("Close", CoverageEveryReturn, pool.Within(1))
	if got := search.completes(call, fn.Params[0]); got.proven || !search.budget.Exhausted() || pool.Exhausted() {
		t.Fatalf("cut=%+v parent exhausted=%v", got, pool.Exhausted())
	}
	search.budget = pool.Within(proofs.SummaryBudget)
	if got := search.completes(call, fn.Params[0]); !got.proven {
		t.Fatalf("fresh memo query=%+v", got)
	}
}

func TestCompletionCaseCoverageAllowance(t *testing.T) {
	pkg := buildTestSSA(t, completionCoverageBudgetFixture)
	for _, test := range []struct {
		name      string
		condition ssacall.CallCondition
	}{
		{"caseTrue", ssacall.CallCondition{Outcome: ssacall.OutcomeTrue}},
		{"argumentCase", ssacall.CallCondition{Arguments: ssacall.ArgumentConstants{Bound: 2, Values: 2}}},
	} {
		t.Run(test.name, func(t *testing.T) {
			fn := pkg.Func(test.name)
			request := CompletionRequest{Target: fn.Params[0], Methods: []string{"Close"}}
			for limit := 0; limit <= proofs.SummaryBudget; limit++ {
				request.Budget = proofs.NewSearchBudget(limit)
				proof := ProveCompletionForCase(fn, test.condition, request)
				if request.Budget.Exhausted() {
					if proof.Proven() || proof.Reason != proofs.EvidenceBudgetExhausted {
						t.Fatalf("cut case%d=%+v", limit, proof)
					}
					continue
				}
				if !proof.Proven() {
					t.Fatalf("complete case%d=%+v", limit, proof)
				}
				return
			}
			t.Fatal("case never completed")
		})
	}
}

func TestCompletionUnavailableSummaryCut(t *testing.T) {
	pkg := buildTestSSA(t, completionOutcomeFixture)
	fn := pkg.Func("callOpaque")
	pool := proofs.NewSearchBudget(proofs.SummaryBudget)
	request := CompletionRequest{Instruction: findLaunch(t, fn), Target: fn.Params[0], Methods: []string{"Close"}, Budget: pool.Within(0)}
	request.Summarized = func(ssa.Instruction, ssa.Value, string, bool, ssacall.CallCondition) bool {
		return request.Budget.Spend()
	}
	cut := ProveCompletion(request)
	if cut.State != proofs.EvidenceUnknown || cut.Reason != proofs.EvidenceBudgetExhausted || pool.Exhausted() {
		t.Fatalf("unavailable cut=%+v parent exhausted=%v", cut, pool.Exhausted())
	}
	request.Budget = pool.Within(proofs.SummaryBudget)
	if fresh := ProveCompletion(request); !fresh.Proven() {
		t.Fatalf("fresh summary=%+v", fresh)
	}
}

func TestCompletionAssumedCoverageAllowance(t *testing.T) {
	pkg := buildTestSSA(t, completionCoverageBudgetFixture)
	for _, yes := range []bool{false, true} {
		fn := pkg.Func("maybe")
		outcome := ssacall.OutcomeFalse
		if yes {
			outcome = ssacall.OutcomeTrue
		}
		assumptions := ssapath.EntryAssumptions{NonNil: fn.Params[0], Constants: ssacall.FixedValues{fn.Params[1]: outcome}}
		calls := func(instruction ssa.Instruction) bool {
			common := ssaflow.InstructionCall(instruction)
			return common != nil && ssaflow.CallName(common) == "Close"
		}
		completed := false
		for limit := 0; limit <= proofs.SummaryBudget; limit++ {
			budget := proofs.NewSearchBudget(limit)
			proof := proveMethodCallCoverageAssumingWithin(fn, calls, CoverageEveryReturn, assumptions, budget)
			if budget.Exhausted() || limit == 0 {
				if proof.State != proofs.EvidenceUnknown || proof.Reason != proofs.EvidenceBudgetExhausted {
					t.Fatalf("assumed cut%d=%+v", limit, proof)
				}
				continue
			}
			if proof.State == proofs.EvidenceUnknown || proof.Proven() != yes {
				t.Fatalf("assumed complete%d=%+v want%v", limit, proof, yes)
			}
			completed = true
			break
		}
		if !completed {
			t.Fatal("assumed query never completed")
		}
	}
}
