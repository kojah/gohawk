package lockorder

import (
	"go/constant"
	"strings"
	"testing"

	"github.com/kojah/gohawk/internal/lifecycle"
	"github.com/kojah/gohawk/internal/passes/concurrencyfacts"
	"github.com/kojah/gohawk/internal/ssaflow"
	"github.com/kojah/gohawk/internal/ssaflow/ssaflowtest"
	"golang.org/x/tools/go/ssa"
)

func TestLockReturnContractsShareAllowance(t *testing.T) {
	pkg := lockReturnPackage(t)
	for _, name := range []string{"heldSuccess", "heldFalse"} {
		fn := pkg.Func(name)
		setup := buildLockSetup(lockSetupPass(fn, concurrencyfacts.NewEngine()), fn, ssaflow.NewSearchBudget(lockStateWorkBudget)).setup
		identity := lockIdentityOf(pkg.Var("global"))
		heldAt := lockReturnStates(setup, identity, name == "heldSuccess")
		var acquisitions []ssa.Instruction
		for instruction, effect := range setup.direct {
			if effect.operation == mutexAcquire {
				acquisitions = append(acquisitions, instruction)
			}
		}
		caller := pkg.Func("callerSuccess")
		if name == "heldFalse" {
			caller = pkg.Func("callerFalse")
		}
		callers := conditionalCallerSet{calls: ssaflow.InstructionsOf[*ssa.Call](caller)[:1]}
		complete := false
		for limit := range ssaflow.SummaryBudget {
			pool := ssaflow.NewSearchBudget(lockStateWorkBudget)
			child := pool.Within(limit)
			query := lockReturnQueries{setup: setup, budget: child}
			proof := query.conditionalCallerRelease(fn, []ssa.Value{pkg.Var("global")}, heldAt, identity, callers)
			if proof.proven {
				if child.Exhausted() {
					t.Fatalf("%s proved interrupted caller contract", name)
				}
				complete = true
				break
			}
			if !child.Exhausted() || pool.Exhausted() {
				t.Fatalf("%s cut%d exhausted=%v/%v proof=%+v", name, limit, child.Exhausted(), pool.Exhausted(), proof)
			}
			fresh := lockReturnQueries{setup: setup, budget: pool.Within(ssaflow.SummaryBudget)}
			if proof := fresh.conditionalCallerRelease(fn, []ssa.Value{pkg.Var("global")}, heldAt, identity, callers); !proof.proven {
				t.Fatalf("%s fresh caller contract: %+v", name, proof)
			}
		}
		if !complete {
			t.Fatalf("%s caller contract never completes", name)
		}
		query := lockReturnQueries{setup: setup, budget: ssaflow.NewSearchBudget(ssaflow.SummaryBudget)}
		proof := query.acquiresForCaller(fn, acquisitions, heldAt, identity)
		if proof.proven != (name == "heldSuccess") {
			t.Fatalf("%s held-success policy changed: %+v", name, proof)
		}
		callers.escaped = true
		if proof := query.conditionalCallerRelease(fn, []ssa.Value{pkg.Var("global")}, heldAt, identity, callers); proof.proven {
			t.Fatal("escaped caller establishes a release")
		}
	}
}

func TestLockHeldContractCutoffAndGuardedError(t *testing.T) {
	pkg := lockReturnPackage(t)
	fn := pkg.Func("heldSuccess")
	setup := buildLockSetup(lockSetupPass(fn, concurrencyfacts.NewEngine()), fn, ssaflow.NewSearchBudget(lockStateWorkBudget)).setup
	identity := lockIdentityOf(pkg.Var("global"))
	heldAt := lockReturnStates(setup, identity, true)
	acquisition := ssaflow.InstructionsOf[*ssa.Call](fn)[0]
	complete := false
	for limit := range ssaflow.SummaryBudget {
		pool := ssaflow.NewSearchBudget(lockStateWorkBudget)
		child := pool.Within(limit)
		query := lockReturnQueries{setup: setup, budget: child}
		proof := query.acquiresForCaller(fn, []ssa.Instruction{acquisition}, heldAt, identity)
		if proof.proven {
			if child.Exhausted() {
				t.Fatal("proved interrupted held-success contract")
			}
			complete = true
			break
		}
		if !child.Exhausted() || pool.Exhausted() {
			t.Fatalf("cut%d proof=%+v exhausted=%v/%v", limit, proof, child.Exhausted(), pool.Exhausted())
		}
		fresh := lockReturnQueries{setup: setup, budget: pool.Within(ssaflow.SummaryBudget)}
		if !fresh.acquiresForCaller(fn, []ssa.Instruction{acquisition}, heldAt, identity).proven {
			t.Fatal("fresh held-success query fails")
		}
	}
	if !complete {
		t.Fatal("held-success query never completes")
	}
	fn = pkg.Func("guarded")
	setup = buildLockSetup(lockSetupPass(fn, concurrencyfacts.NewEngine()), fn, ssaflow.NewSearchBudget(lockStateWorkBudget)).setup
	query := lockReturnQueries{setup: setup, budget: ssaflow.NewSearchBudget(ssaflow.SummaryBudget)}
	successes := 0
	for _, returned := range setup.returns {
		if query.successfulReturn(fn, returned) {
			successes++
			cut := lockReturnQueries{setup: setup, budget: ssaflow.NewSearchBudget(0)}
			if cut.successfulReturn(fn, returned) || !cut.budget.Exhausted() {
				t.Fatal("nil-guard proof bypassed zero allowance")
			}
		}
	}
	if successes != 1 || query.budget.Exhausted() {
		t.Fatalf("guarded error success count=%d", successes)
	}
}

func TestLockFinalMetadataCutoffDiscardsReportsAndOrders(t *testing.T) {
	assertLockWalkCutoffs(t, newLockWalkFixture(lockReturnPackage(t).Func("witness")), 2)
}

func TestLockCallerCoverageChargesAllowance(t *testing.T) {
	fn := lockReturnPackage(t).Func("paddedCaller")
	call := ssaflow.InstructionsOf[*ssa.Call](fn)[0]
	pool := ssaflow.NewSearchBudget(ssaflow.SummaryBudget)
	child := pool.Within(20)
	query := lockReturnQueries{budget: child}
	global := fn.Pkg.Var("global")
	heldWhen := ssaflow.CallCondition{Result: 0, Outcome: ssaflow.OutcomeFalse}
	if query.callerReleasesOnFlag(call, global, heldWhen) || !child.Exhausted() || pool.Exhausted() {
		t.Fatalf("caller coverage cutoff exhausted=%v/%v", child.Exhausted(), pool.Exhausted())
	}
	fresh := lockReturnQueries{budget: pool.Within(ssaflow.SummaryBudget)}
	if !fresh.callerReleasesOnFlag(call, global, heldWhen) || fresh.budget.Exhausted() {
		t.Fatal("fresh padded caller fails")
	}
}

func lockReturnStates(setup *lockFunctionSetup, identity string, heldWhen bool) map[*ssa.Return]lockReturnState {
	held := map[*ssa.Return]lockReturnState{}
	for _, returned := range setup.returns {
		literal := lifecycle.ReturnedResult(returned, 0).(*ssa.Const)
		state := lockReturnState{}
		if constant.BoolVal(literal.Value) == heldWhen {
			state = lockReturnState{possible: []string{identity}, definite: []string{identity}}
		}
		held[returned] = state
	}
	return held
}

func lockReturnPackage(t *testing.T) *ssa.Package {
	t.Helper()
	return ssaflowtest.BuildPackage(t, "lockreturns", `package lockreturns
 import "sync"
 var global, second sync.Mutex
 var shared struct {mu sync.RWMutex; value int}
 func heldSuccess(fail bool)bool {global.Lock();if fail{global.Unlock();return false};return true}
 func heldFalse(fail bool)bool {global.Lock();if fail{global.Unlock();return true};return false}
 func callerSuccess(fail bool){if heldSuccess(fail){global.Unlock();return};return}
 func callerFalse(fail bool){if heldFalse(fail){return};global.Unlock()}
 func guarded(err error)error{global.Lock();if err!=nil{global.Unlock();return err};return err}
 func witness(fail bool){
 shared.mu.RLock();shared.value++;shared.mu.RUnlock()
 global.Lock();second.Lock();second.Unlock()
 if fail {global.Unlock();return};return
 }
 func paddedCaller(fail bool,n int)int {if heldFalse(fail){return n};global.Unlock()
 `+strings.Repeat("n+=n\n", 80)+`return n}
 `)
}
