package lockorder

import (
	"go/constant"
	"go/token"
	"slices"
	"strings"
	"testing"

	"github.com/kojah/gohawk/internal/analysis/passes/concurrencyfacts"
	"github.com/kojah/gohawk/internal/engine/lifecycle"
	proofs "github.com/kojah/gohawk/internal/engine/proof"
	"github.com/kojah/gohawk/internal/engine/ssaflow"
	ssacall "github.com/kojah/gohawk/internal/engine/ssaflow/calls"
	"github.com/kojah/gohawk/internal/engine/ssaflow/ssaflowtest"
	"golang.org/x/tools/go/ssa"
)

func TestLockReturnContractsShareAllowance(t *testing.T) {
	pkg := lockReturnPackage(t)
	for _, name := range []string{"heldSuccess", "heldFalse"} {
		fn := pkg.Func(name)
		setup := buildLockSetup(lockSetupPass(fn, concurrencyfacts.NewEngine()), fn, proofs.NewSearchBudget(lockStateWorkBudget)).setup
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
		callers := conditionalCallerSet{Calls: ssaflow.InstructionsOf[*ssa.Call](caller)[:1]}
		complete := false
		for limit := range proofs.SummaryBudget {
			pool := proofs.NewSearchBudget(lockStateWorkBudget)
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
			fresh := lockReturnQueries{setup: setup, budget: pool.Within(proofs.SummaryBudget)}
			if proof := fresh.conditionalCallerRelease(fn, []ssa.Value{pkg.Var("global")}, heldAt, identity, callers); !proof.proven {
				t.Fatalf("%s fresh caller contract: %+v", name, proof)
			}
		}
		if !complete {
			t.Fatalf("%s caller contract never completes", name)
		}
		query := lockReturnQueries{setup: setup, budget: proofs.NewSearchBudget(proofs.SummaryBudget)}
		proof := query.acquiresForCaller(fn, acquisitions, heldAt, identity)
		if proof.proven != (name == "heldSuccess") {
			t.Fatalf("%s held-success policy changed: %+v", name, proof)
		}
		callers.Escaped = true
		if proof := query.conditionalCallerRelease(fn, []ssa.Value{pkg.Var("global")}, heldAt, identity, callers); proof.proven {
			t.Fatal("escaped caller establishes a release")
		}
	}
}

func TestLockHeldContractCutoffAndGuardedError(t *testing.T) {
	pkg := lockReturnPackage(t)
	fn := pkg.Func("heldSuccess")
	setup := buildLockSetup(lockSetupPass(fn, concurrencyfacts.NewEngine()), fn, proofs.NewSearchBudget(lockStateWorkBudget)).setup
	identity := lockIdentityOf(pkg.Var("global"))
	heldAt := lockReturnStates(setup, identity, true)
	acquisition := ssaflow.InstructionsOf[*ssa.Call](fn)[0]
	complete := false
	for limit := range proofs.SummaryBudget {
		pool := proofs.NewSearchBudget(lockStateWorkBudget)
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
		fresh := lockReturnQueries{setup: setup, budget: pool.Within(proofs.SummaryBudget)}
		if !fresh.acquiresForCaller(fn, []ssa.Instruction{acquisition}, heldAt, identity).proven {
			t.Fatal("fresh held-success query fails")
		}
	}
	if !complete {
		t.Fatal("held-success query never completes")
	}
	fn = pkg.Func("guarded")
	setup = buildLockSetup(lockSetupPass(fn, concurrencyfacts.NewEngine()), fn, proofs.NewSearchBudget(lockStateWorkBudget)).setup
	query := lockReturnQueries{setup: setup, budget: proofs.NewSearchBudget(proofs.SummaryBudget)}
	successes := 0
	for _, returned := range setup.returns {
		if query.successfulReturn(fn, returned) {
			successes++
			cut := lockReturnQueries{setup: setup, budget: proofs.NewSearchBudget(0)}
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
	pool := proofs.NewSearchBudget(proofs.SummaryBudget)
	child := pool.Within(20)
	query := lockReturnQueries{budget: child}
	global := fn.Pkg.Var("global")
	heldWhen := ssacall.CallCondition{Result: 0, Outcome: ssacall.OutcomeFalse}
	if query.callerReleasesOnFlag(call, global, heldWhen) || !child.Exhausted() || pool.Exhausted() {
		t.Fatalf("caller coverage cutoff exhausted=%v/%v", child.Exhausted(), pool.Exhausted())
	}
	fresh := lockReturnQueries{budget: pool.Within(proofs.SummaryBudget)}
	if !fresh.callerReleasesOnFlag(call, global, heldWhen) || fresh.budget.Exhausted() {
		t.Fatal("fresh padded caller fails")
	}
}

func lockReturnStates(setup *lockFunctionSetup, identity string, heldWhen bool) map[*ssa.Return]lockReturnState {
	held := map[*ssa.Return]lockReturnState{}
	for _, returned := range setup.returns {
		literal := lifecycle.ReturnedResultWithin(returned, 0, nil).(*ssa.Const)
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

func TestLockReturnGuardAllowance(t *testing.T) {
	pkg := ssaflowtest.BuildPackage(t, "returnguard", `package returnguard
func guarded(err error) error { if err != nil { return nil }; return err }
func unrelated(err, other error) error { if other != nil { return nil }; return err }
`)
	for _, name := range []string{"guarded", "unrelated"} {
		fn := pkg.Func(name)
		setup := &lockFunctionSetup{branches: ssaflow.InstructionsOf[*ssa.If](fn)}
		var returned *ssa.Return
		for _, candidate := range ssaflow.InstructionsOf[*ssa.Return](fn) {
			if candidate.Results[0] == fn.Params[0] {
				returned = candidate
			}
		}
		if returned == nil {
			t.Fatal("actual SSA must return the checked value directly")
		}
		if name == "guarded" {
			pool := proofs.NewSearchBudget(proofs.SummaryBudget)
			query := lockReturnQueries{setup: setup, budget: pool.Within(3)}
			if query.nilGuardDominatesReturn(fn.Params[0], returned) || !query.budget.Exhausted() || pool.Exhausted() {
				t.Fatal("guard identity bypassed the selection-only allowance")
			}
		}
		complete := false
		for limit := 0; limit <= proofs.SummaryBudget; limit++ {
			query := lockReturnQueries{setup: setup, budget: proofs.NewSearchBudget(limit)}
			got := query.nilGuardDominatesReturn(fn.Params[0], returned)
			if query.budget.Exhausted() {
				if got {
					t.Fatal("interrupted guard supplied a return contract")
				}
				continue
			}
			if got != (name == "guarded") {
				t.Fatalf("complete %s guard = %v", name, got)
			}
			complete = true
			break
		}
		if !complete {
			t.Fatal("fresh guard never completed")
		}
	}
}

func TestLockReturnRetentionAndMerge(t *testing.T) {
	pkg := lockRetentionPackage(t)
	for _, name := range []string{"keep", "callback", "returnedOwner"} {
		fn := pkg.Func(name)
		returned := ssaflow.InstructionsOf[*ssa.Return](fn)[0]
		value := ssa.Value(fn.Params[0])
		if name == "returnedOwner" {
			value = ssaflow.InstructionsOf[*ssa.FieldAddr](fn)[0]
		}
		identity := lockIdentityOf(value)
		values := map[string][]ssa.Value{identity: {value}}
		heldAt := map[*ssa.Return]lockReturnState{}
		unreleased := map[string][]token.Pos{}
		query := lockReturnQueries{budget: proofs.NewSearchBudget(proofs.SummaryBudget)}
		incoming := []string{identity}
		query.recordUnreleasedLocks(returned, incoming, nil, values, unreleased, heldAt)
		if query.budget.Exhausted() || len(unreleased[identity]) != boolCount(name == "keep") {
			t.Fatalf("%s retained=%v exhausted=%v", name, unreleased, query.budget.Exhausted())
		}
		if name != "keep" {
			continue
		}
		incoming[0] = "different"
		if !slices.Equal(heldAt[returned].definite, []string{identity}) {
			t.Fatal("return mask aliases incoming state")
		}
		query.recordUnreleasedLocks(returned, nil, nil, values, unreleased, heldAt)
		if !slices.Equal(heldAt[returned].possible, []string{identity}) || len(heldAt[returned].definite) != 0 {
			t.Fatalf("merged possible/definite retention: %+v", heldAt[returned])
		}
		query.recordUnreleasedLocks(returned, []string{identity}, []string{identity}, values, unreleased, heldAt)
		if len(unreleased[identity]) != 1 {
			t.Fatal("deferred release added another uncovered return")
		}
	}
}

func TestLockReturnOwnerAndHandoffCutoff(t *testing.T) {
	pkg := lockRetentionPackage(t)
	for _, name := range []string{"padded", "handoff"} {
		fn := pkg.Func(name)
		pool := proofs.NewSearchBudget(lockStateWorkBudget)
		child := pool.Within(30)
		ask := func(budget *proofs.SearchBudget) bool {
			if name == "handoff" {
				call := ssaflow.InstructionsOf[*ssa.Call](fn)[0]
				return handedUnlockCallback(call, fn.Params[0], budget)
			}
			query := lockReturnQueries{budget: budget}
			returned := ssaflow.InstructionsOf[*ssa.Return](fn)[0]
			return query.returnedUnlockOwner(returned, []ssa.Value{fn.Params[0]})
		}
		if ask(child) || !child.Exhausted() || pool.Exhausted() {
			t.Fatalf("%s cutoff exhausted=%v/%v", name, child.Exhausted(), pool.Exhausted())
		}
		// Both padded callback bodies now charge numeric capture mapping too;
		// retain their padding and small cutoff with separate recovery allowance.
		fresh := pool.Within(2 * proofs.SummaryBudget)
		if !ask(fresh) {
			t.Fatalf("%s fresh capability query fails, exhausted=%v/%v", name, fresh.Exhausted(), pool.Exhausted())
		}
	}
}

func TestLockReturnMergeCutoffDiscardsMasks(t *testing.T) {
	for _, seen := range []bool{false, true} {
		pool := proofs.NewSearchBudget(proofs.SummaryBudget)
		child := pool.Within(1)
		query := lockReturnQueries{budget: child}
		previous := lockReturnState{possible: []string{"first"}, definite: []string{"first"}}
		merged := query.mergeReturnState(previous, []string{"first", "second"}, seen)
		if !child.Exhausted() || pool.Exhausted() || len(merged.possible) != 0 || len(merged.definite) != 0 {
			t.Fatalf("seen=%v partial merge: %+v exhausted=%v/%v", seen, merged, child.Exhausted(), pool.Exhausted())
		}
		fresh := lockReturnQueries{budget: pool.Within(proofs.SummaryBudget)}
		merged = fresh.mergeReturnState(previous, []string{"first", "second"}, seen)
		if !slices.Equal(merged.possible, []string{"first", "second"}) || len(merged.definite) != 2-boolCount(seen) {
			t.Fatalf("seen=%v fresh merge: %+v", seen, merged)
		}
	}
}

func TestLockRetentionCutoffDiscardsEarlierFindings(t *testing.T) {
	assertLockWalkCutoffs(t, newLockWalkFixture(lockRetentionPackage(t).Func("witness")), 1)
}

func boolCount(value bool) int {
	if value {
		return 1
	}
	return 0
}

func lockRetentionPackage(t *testing.T) *ssa.Package {
	t.Helper()
	return ssaflowtest.BuildPackage(t, "lockretention", `package lockretention
 import "sync"
 var total int
 var global, second sync.Mutex
 var shared struct {mu sync.RWMutex; value int}
 type owner struct {mu sync.Mutex}
 func keep(mu *sync.Mutex)int{return 1}
 func callback(mu *sync.Mutex)func(){return mu.Unlock}
 func returnedOwner(o *owner)*owner{_=&o.mu;return o}
 func padded(mu *sync.Mutex,n int)func(){return func(){`+strings.Repeat("n+=n;total=n\n", 60)+`mu.Unlock()}}
 func accept(cb func())
 func handoff(mu *sync.Mutex,n int){accept(func(){`+strings.Repeat("n+=n;total=n\n", 60)+`mu.Unlock()})}
 func witness(n int)func(){
 shared.mu.RLock();shared.value++;shared.mu.RUnlock()
 global.Lock();second.Lock();second.Unlock();global.Unlock();global.Lock()
 return func(){`+strings.Repeat("n+=n;total=n\n", 60)+`global.Unlock()}
 }
 `)
}

func TestReturnedSuccessCellSharesAllowance(t *testing.T) {
	pkg := ssaflowtest.BuildPackage(t, "returnstorage", `package returnstorage
func cleanup() {}
func subject() (ok bool) { defer cleanup(); return true }
func declined() (ok bool) { defer cleanup(); return false }
func plain() bool { return true }
`)
	for _, name := range []string{"subject", "declined", "plain"} {
		t.Run(name, func(t *testing.T) {
			function := pkg.Func(name)
			var dump strings.Builder
			if _, err := function.WriteTo(&dump); err != nil {
				t.Fatal(err)
			}
			t.Log(dump.String())
			var returned *ssa.Return
			for _, candidate := range ssaflow.InstructionsOf[*ssa.Return](function) {
				if candidate.Block().Comment != "recover" {
					returned = candidate
					break
				}
			}
			if returned == nil {
				t.Fatal("no normal return")
			}
			for _, limit := range []int{0, 1, proofs.SummaryBudget} {
				budget := proofs.NewSearchBudget(limit)
				query := lockReturnQueries{budget: budget}
				success := query.successfulReturn(function, returned)
				if limit == 0 && (!budget.Exhausted() || success) {
					t.Fatalf("zero allowance supplied success=%v exhausted=%v", success, budget.Exhausted())
				}
				if budget.Exhausted() && success {
					t.Fatal("interrupted result storage supplied success")
				}
				if limit == proofs.SummaryBudget && (budget.Exhausted() || success != (name != "declined")) {
					t.Fatalf("complete success=%v exhausted=%v", success, budget.Exhausted())
				}
			}
		})
	}
}
