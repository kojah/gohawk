package lockorder

import (
	"go/constant"
	"go/types"
	"reflect"
	"testing"

	"github.com/kojah/gohawk/internal/ssaflow"
	"github.com/kojah/gohawk/internal/ssaflow/ssaflowtest"
	"golang.org/x/tools/go/ssa"
)

func TestLockStateKeyAvailability(t *testing.T) {
	state := lockBudgetState(t)
	want := lockStateKey(state, nil)
	limited := ssaflow.NewSearchBudget(1)
	if key := lockStateKey(state, limited); key != "" || !limited.Exhausted() {
		t.Fatal("partial key exposed")
	}

	finished := false
	for limit := range 64 {
		pool := ssaflow.NewSearchBudget(ssaflow.QueryBudget)
		child := pool.Within(limit)
		expanded := 0
		ssaflow.WalkStatesWithin([]lockFlowState{state}, func(next lockFlowState) string { return lockStateKey(next, child) },
			func(lockFlowState) ([]lockFlowState, bool) { expanded++; return nil, true }, child)
		if !child.Exhausted() {
			finished = true
			break
		}
		if expanded != 0 || pool.Exhausted() {
			t.Fatalf("cut%d expanded=%d pool exhausted=%v", limit, expanded, pool.Exhausted())
		}
		if got := lockStateKey(state, pool.Within(ssaflow.QueryBudget)); got != want {
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
	limited := ssaflow.NewSearchBudget(1)
	if _, ok := cloneLockStateWithin(state, limited); ok || !limited.Exhausted() {
		t.Fatal("partial copy exposed")
	}
	finished := false
	for limit := range 64 {
		pool := ssaflow.NewSearchBudget(ssaflow.QueryBudget)
		child := pool.Within(limit)
		copied, ok := cloneLockStateWithin(state, child)
		if ok {
			finished = true
			break
		}
		if !child.Exhausted() || !reflect.DeepEqual(copied, lockFlowState{}) || pool.Exhausted() {
			t.Fatalf("partial copy at%d: %+v", limit, copied)
		}
		fresh, ok := cloneLockStateWithin(state, pool.Within(ssaflow.QueryBudget))
		if !ok || !reflect.DeepEqual(fresh, state) {
			t.Fatalf("fresh copy at%d differs", limit)
		}
	}
	if !finished {
		t.Fatal("copy never completed")
	}
	copied, ok := cloneLockStateWithin(state, ssaflow.NewSearchBudget(ssaflow.QueryBudget))
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
	pool := ssaflow.NewSearchBudget(ssaflow.QueryBudget)
	child := pool.Within(1)
	if got := lockPhiConstants(state, child); len(got) != 0 || !child.Exhausted() || pool.Exhausted() {
		t.Fatalf("cut phi retained %+v", got)
	}
	if got := lockPhiConstants(state, pool.Within(ssaflow.QueryBudget)); !reflect.DeepEqual(got, expected) {
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
		_, known := conditionIdentity(value, pool.Within(ssaflow.QueryBudget))
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
