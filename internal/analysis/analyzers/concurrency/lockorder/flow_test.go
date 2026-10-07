package lockorder

import (
	"go/ast"
	"go/constant"
	"go/parser"
	"go/token"
	"go/types"
	"reflect"
	"strings"
	"testing"

	"github.com/kojah/gohawk/internal/analysis/passes/resultfacts"
	"github.com/kojah/gohawk/internal/engine/lifecycle"
	proofs "github.com/kojah/gohawk/internal/engine/proof"
	"github.com/kojah/gohawk/internal/engine/ssaflow"
	cfg "github.com/kojah/gohawk/internal/engine/ssaflow/cfg"
	"github.com/kojah/gohawk/internal/engine/ssaflow/ssaflowtest"
	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/ssa"
	"golang.org/x/tools/go/ssa/ssautil"
)

func TestLockWalkCutoffDiscardsReportsAndOrders(t *testing.T) {
	// Findings precede a padded tail; a later cutoff discards all function
	// evidence. The same contract also covers final metadata and retention.
	assertLockWalkCutoffs(t, newLockWalkFixture(lockWalkBudgetPackage(t).Func("witness")), 1)
}

func assertLockWalkCutoffs(t *testing.T, fixture *lockWalkFixture, minReports int) {
	t.Helper()
	complete, baseline, edges := fixture.run(proofs.NewSearchBudget(lockStateWorkBudget))
	if !complete || len(baseline) < minReports || edges == 0 {
		t.Fatalf("baseline complete=%v reports=%v edges=%d", complete, baseline, edges)
	}
	finished := false
	for limit := range proofs.SummaryBudget {
		pool := proofs.NewSearchBudget(lockStateWorkBudget)
		child := pool.Within(limit)
		ok, reports, orders := fixture.run(child)
		if ok {
			if child.Exhausted() || len(reports) != len(baseline) || orders != edges {
				t.Fatalf("accepted partial final evidence at%d: reports=%v orders=%d", limit, reports, orders)
			}
			finished = true
			break
		}
		if !child.Exhausted() || pool.Exhausted() || len(reports) != 0 || orders != 0 {
			t.Fatalf("cut%d reports=%v orders=%d exhausted=%v/%v", limit, reports, orders, child.Exhausted(), pool.Exhausted())
		}
		if recovered, reports, orders := fixture.run(pool.Within(lockStateWorkBudget / 2)); !recovered || len(reports) != len(baseline) || orders != edges {
			t.Fatalf("fresh%d complete=%v reports=%v orders=%d", limit, recovered, reports, orders)
		}
	}
	if !finished {
		t.Fatal("final metadata never completes")
	}
}

func TestLockWalkParentCutoffAndStableCleanup(t *testing.T) {
	pkg := lockWalkBudgetPackage(t)
	fixture := newLockWalkFixture(pkg.Func("witness"))
	pool := proofs.NewSearchBudget(1)
	child := pool.Within(lockStateWorkBudget)
	if ok, reports, orders := fixture.run(child); ok || !child.PoolExhausted() || len(reports) != 0 || orders != 0 {
		t.Fatalf("parent cutoff complete=%v reports=%v edges=%d", ok, reports, orders)
	}
	safe := newLockWalkFixture(pkg.Func("safe"))
	if ok, reports, orders := safe.run(proofs.NewSearchBudget(lockStateWorkBudget)); !ok || len(reports) != 0 || orders != 0 {
		t.Fatalf("stable branch cleanup complete=%v reports=%v edges=%d", ok, reports, orders)
	}
}

type lockWalkFixture struct {
	function    *ssa.Function
	evidence    lifecycle.LocalEvidence
	calleeLocks *calleeLockSearch
	results     *resultfacts.Engine
}

func newLockWalkFixture(function *ssa.Function) *lockWalkFixture {
	return &lockWalkFixture{function: function, calleeLocks: newCalleeLockSearch(), results: resultfacts.NewEngine()}
}

func (fixture *lockWalkFixture) run(budget *proofs.SearchBudget) (bool, []analysis.Diagnostic, int) {
	var reports []analysis.Diagnostic
	fn := fixture.function
	pass := &analysis.Pass{
		Fset: fn.Prog.Fset, Pkg: fn.Pkg.Pkg,
		ResultOf: map[*analysis.Analyzer]any{resultfacts.Analyzer: fixture.results},
		Report:   func(d analysis.Diagnostic) { reports = append(reports, d) },
	}
	orders := newLockOrders()
	exclusive := newExclusiveCallers(pass, collectLockCallers(nil, []*ssa.Function{fn}, nil))
	walk := lockStateWalk{budget: budget}
	complete := walk.analyze(pass, fn, orders, fixture.calleeLocks, &fixture.evidence, nil, exclusive)
	return complete, reports, len(orders.edges)
}

func lockWalkBudgetPackage(t *testing.T) *ssa.Package {
	t.Helper()
	return ssaflowtest.BuildPackage(t, "lockbudget", `package lockbudget
 import "sync"
 var first, second sync.Mutex
 var total int
 var shared struct { mu sync.RWMutex; value int }
 func witness(n int) {
 shared.mu.RLock(); shared.value++; shared.mu.RUnlock()
 first.Lock(); second.Lock()
 `+strings.Repeat("n += 1; total = n\n", 12)+`
 second.Unlock(); first.Unlock()
 }
 func safe(flag bool) { if flag {first.Lock()}; if flag {first.Unlock()} }
 `)
}

func TestLockSuccessorResultInferenceSharesAllowance(t *testing.T) {
	pkg := ssaflowtest.BuildPackage(t, "lockbranches", `package lockbranches
 var total int
 func allowed()bool {`+strings.Repeat("total++\n", 50)+`return true}
 func root(){if allowed(){total++}}
 `)
	fn := pkg.Func("root")
	state := lockFlowState{block: fn.Blocks[0]}
	pass := &analysis.Pass{Fset: fn.Prog.Fset, Pkg: fn.Pkg.Pkg}
	// Measure ordinary successor setup independently of result inference. The
	// extra allowance below fits that setup but cannot fit the padded callee.
	setup := 0
	for ; setup < proofs.QueryBudget; setup++ {
		budget := proofs.NewSearchBudget(setup)
		got := lockSuccessorStates(pass, state, budget)
		if !budget.Exhausted() && len(got) == 2 {
			break
		}
	}
	if setup == proofs.QueryBudget {
		t.Fatal("ordinary branch never completed")
	}
	pass.ResultOf = map[*analysis.Analyzer]any{resultfacts.Analyzer: resultfacts.NewEngine()}
	pool := proofs.NewSearchBudget(10 * proofs.SummaryBudget)
	child := pool.Within(setup + 10)
	if got := lockSuccessorStates(pass, state, child); len(got) != 0 || !child.Exhausted() || pool.Exhausted() {
		t.Fatalf("nested result cutoff kept %d successors; exhausted=%v/%v", len(got), child.Exhausted(), pool.Exhausted())
	}
	fresh := pool.Within(proofs.SummaryBudget)
	got := lockSuccessorStates(pass, state, fresh)
	if fresh.Exhausted() || len(got) != 1 || got[0].block != state.block.Succs[0] {
		t.Fatalf("fresh result branch: %+v", got)
	}
}

func TestLockWalkTerminationResultCutoff(t *testing.T) {
	pkg := ssaflowtest.BuildPackage(t, "locktermination", `package locktermination
 import "sync"
 var mu sync.Mutex
 var total int
 func stop(){`+strings.Repeat("total++\n", 30)+`panic("stop")}
 func root(){mu.Lock();stop()}
 `)
	fixture := newLockWalkFixture(pkg.Func("root"))
	// Cold result inference must share the caller's allowance. Warm summaries
	// may be cheaper, but cannot change the termination guarantee itself.
	limited := proofs.NewSearchBudget(30)
	if ok, reports, edges := fixture.run(limited); ok || !limited.Exhausted() || len(reports) != 0 || edges != 0 {
		t.Fatalf("termination cutoff complete=%v reports=%v edges=%d", ok, reports, edges)
	}
	for range 2 {
		fresh := proofs.NewSearchBudget(lockStateWorkBudget)
		if ok, reports, edges := fixture.run(fresh); !ok || fresh.Exhausted() || len(reports) != 0 || edges != 0 {
			t.Fatalf("fresh terminating path complete=%v reports=%v edges=%d", ok, reports, edges)
		}
	}
}

func TestLockPhiConstantsForgetUnknownInput(t *testing.T) {
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "state.go", `package state
func merge(choose, unknown bool) bool {
    value := false
    if choose { value = unknown }
    return value
}`, 0)
	if err != nil {
		t.Fatal(err)
	}
	pkg, _, err := ssautil.BuildPackage(&types.Config{}, fset, types.NewPackage("state", "state"), []*ast.File{file}, ssa.SanityCheckFunctions)
	if err != nil {
		t.Fatal(err)
	}
	phis := ssaflow.InstructionsOf[*ssa.Phi](pkg.Func("merge"))
	if len(phis) != 1 {
		t.Fatalf("got %d phis, want one", len(phis))
	}
	phi := phis[0]
	for predecessor, incoming := range ssaflow.PhiIncoming(phi) {
		state := lockFlowState{
			block: phi.Block(), predecessor: predecessor,
			constants: []lockScalarConstant{{value: phi, literal: ssa.NewConst(constant.MakeBool(true), types.Typ[types.Bool])}},
		}
		got := lockPhiConstants(state, nil)
		_, literal := incoming.(*ssa.Const)
		truth, known := lockBooleanValue(phi, got)
		if known != literal || truth {
			t.Errorf("incoming %T: got truth=%t known=%t, want false/%t", incoming, truth, known, literal)
		}
		if !constant.BoolVal(state.constants[0].literal.Value) {
			t.Error("entry transfer mutated predecessor state")
		}
	}
}

func TestLockLiteralBranch(t *testing.T) {
	phase := &ssa.Phi{}
	initial := ssa.NewConst(constant.MakeInt64(-1), types.Typ[types.Int])
	other := ssa.NewConst(constant.MakeInt64(2), types.Typ[types.Int])
	bindings := []lockScalarConstant{{value: phase, literal: initial}}
	for _, test := range []struct {
		name  string
		value ssa.Value
		truth bool
		known bool
	}{
		{"equal", &ssa.BinOp{Op: token.EQL, X: phase, Y: initial}, true, true},
		{"unequal", &ssa.BinOp{Op: token.NEQ, X: phase, Y: initial}, false, true},
		{"different", &ssa.BinOp{Op: token.NEQ, X: phase, Y: other}, true, true},
		{"arithmetic", &ssa.BinOp{Op: token.ADD, X: phase, Y: other}, false, false},
		{"unknown", &ssa.BinOp{Op: token.EQL, X: &ssa.Phi{}, Y: initial}, false, false},
		{"ordering", &ssa.BinOp{Op: token.LSS, X: phase, Y: other}, false, false},
	} {
		t.Run(test.name, func(t *testing.T) {
			truth, known := lockBooleanValue(test.value, bindings)
			if truth != test.truth || known != test.known {
				t.Errorf("got %t/%t, want %t/%t", truth, known, test.truth, test.known)
			}
		})
	}
}

func TestLockStateKeyAvailability(t *testing.T) {
	state := lockBudgetState(t)
	want := lockStateKey(state, nil)
	limited := proofs.NewSearchBudget(1)
	if key := lockStateKey(state, limited); key != "" || !limited.Exhausted() {
		t.Fatal("partial key exposed")
	}

	finished := false
	for limit := range 64 {
		pool := proofs.NewSearchBudget(proofs.QueryBudget)
		child := pool.Within(limit)
		expanded := 0
		cfg.WalkStatesWithin([]lockFlowState{state}, func(next lockFlowState) string { return lockStateKey(next, child) },
			func(lockFlowState) ([]lockFlowState, bool) { expanded++; return nil, true }, child)
		if !child.Exhausted() {
			finished = true
			break
		}
		if expanded != 0 || pool.Exhausted() {
			t.Fatalf("cut%d expanded=%d pool exhausted=%v", limit, expanded, pool.Exhausted())
		}
		if got := lockStateKey(state, pool.Within(proofs.QueryBudget)); got != want {
			t.Fatalf("fresh key %q want %q", got, want)
		}
	}
	if !finished {
		t.Fatal("key never completed")
	}
}

func TestLockStateCopyAvailability(t *testing.T) {
	state := lockBudgetState(t)
	want := lockStateKey(state, nil)
	limited := proofs.NewSearchBudget(1)
	if _, ok := cloneLockStateWithin(state, limited); ok || !limited.Exhausted() {
		t.Fatal("partial copy exposed")
	}
	finished := false
	for limit := range 64 {
		pool := proofs.NewSearchBudget(proofs.QueryBudget)
		child := pool.Within(limit)
		copied, ok := cloneLockStateWithin(state, child)
		if ok {
			finished = true
			break
		}
		if !child.Exhausted() || !reflect.DeepEqual(copied, lockFlowState{}) || pool.Exhausted() {
			t.Fatalf("partial copy at%d: %+v", limit, copied)
		}
		fresh, ok := cloneLockStateWithin(state, pool.Within(proofs.QueryBudget))
		if !ok || !reflect.DeepEqual(fresh, state) {
			t.Fatalf("fresh copy at%d differs", limit)
		}
	}
	if !finished {
		t.Fatal("copy never completed")
	}
	copied, ok := cloneLockStateWithin(state, proofs.NewSearchBudget(proofs.QueryBudget))
	if !ok {
		t.Fatal("copy unavailable")
	}
	copied.held[0], copied.readHeld[0], copied.deferred[0] = "other", "other", "other"
	delete(copied.guards, "first")
	delete(copied.origins, "first")
	if got := lockStateKey(state, nil); got != want {
		t.Fatal("successor mutation changed predecessor state")
	}
}

func TestLockPhiAndCycleQueriesShareAllowance(t *testing.T) {
	state := lockBudgetState(t)
	expected := lockPhiConstants(state, nil)
	pool := proofs.NewSearchBudget(proofs.QueryBudget)
	child := pool.Within(1)
	if got := lockPhiConstants(state, child); len(got) != 0 || !child.Exhausted() || pool.Exhausted() {
		t.Fatalf("cut phi retained %+v", got)
	}
	if got := lockPhiConstants(state, pool.Within(proofs.QueryBudget)); !reflect.DeepEqual(got, expected) {
		t.Fatalf("fresh phi differs: %+v want %+v", got, expected)
	}
	pkg := ssaflowtest.BuildPackage(t, "conditions", `package conditions
 func computed()bool
 func once(flag bool)bool{value:=computed();if flag{return value};return !value}
 func repeated(){for computed(){}}
 `)
	for _, name := range []string{"once", "repeated"} {
		value := ssaflow.InstructionsOf[*ssa.Call](pkg.Func(name))[0]
		child = pool.Within(1)
		if identity, known := conditionIdentity(value, child); known || identity != "" || !child.Exhausted() {
			t.Fatalf("cut cycle query %s became stable: %q/%v", name, identity, known)
		}
		_, known := conditionIdentity(value, pool.Within(proofs.QueryBudget))
		if known != (name == "once") || pool.Exhausted() {
			t.Fatalf("fresh %s known=%v", name, known)
		}
	}
}

func lockBudgetState(t *testing.T) lockFlowState {
	t.Helper()
	pkg := ssaflowtest.BuildPackage(t, "statebudget", `package statebudget
 func merge(choose,unknown bool)bool {value:=false;if choose{value=unknown};return value}
 `)
	phi := ssaflow.InstructionsOf[*ssa.Phi](pkg.Func("merge"))[0]
	return lockFlowState{
		block: phi.Block(), predecessor: phi.Block().Preds[0],
		held: []string{"first", "second"}, readHeld: []string{"first"}, deferred: []string{"second"},
		guards:    map[string]lockGuard{"first": {condition: "flag", value: true}},
		origins:   map[string]lockAcquisition{"first": {position: phi.Pos()}, "second": {position: phi.Pos()}},
		constants: []lockScalarConstant{{value: phi, literal: ssa.NewConst(constant.MakeBool(true), types.Typ[types.Bool])}},
	}
}
