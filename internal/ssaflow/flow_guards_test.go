package ssaflow_test

import (
	"testing"

	"github.com/kojah/gohawk/internal/ssaflow"
	"github.com/kojah/gohawk/internal/ssaflow/ssaflowtest"
	"golang.org/x/tools/go/ssa"
)

// branchConditions returns the conditions of a function's If instructions in
// block order.
func branchConditions(function *ssa.Function) []ssa.Value {
	var conditions []ssa.Value
	for _, branch := range ssaflow.InstructionsOf[*ssa.If](function) {
		conditions = append(conditions, branch.Cond)
	}
	return conditions
}

// Negation held in a value and repeated checks of one call result share a stable identity;
// a call result recomputed in a loop does not.
func TestGuardConditionIdentity(t *testing.T) {
	pkg := ssaflowtest.BuildPackage(t, "guards", `package guards
func check(n int) error { return nil }
func negated(flag bool) int { if flag { return 1 }; if !flag { return 2 }; return 3 }
func storedNot(flag bool) int { x := !flag; if flag { return 1 }; if x { return 2 }; return 3 }
func result(n int) int { err := check(n); if err != nil { return 1 }; if err == nil { return 2 }; return 3 }
func looped(n int) int {
	total := 0
	for i := 0; i < n; i++ { if check(i) != nil { total++ } }
	return total
}
`)
	for name, want := range map[string]struct {
		same, stable, flipped bool
	}{
		// The builder swaps the arms for an inline !, so both are flag.
		"negated": {same: true, stable: true, flipped: false},
		// A negation held in a value is peeled back to flag.
		"storedNot": {same: true, stable: true, flipped: true},
		"result":    {same: true, stable: true, flipped: true},
	} {
		conditions := branchConditions(pkg.Func(name))
		if len(conditions) != 2 {
			t.Fatalf("%s: %d conditions", name, len(conditions))
		}
		first, firstNegated, firstStable, ok1 := ssaflow.GuardCondition(conditions[0])
		second, secondNegated, secondStable, ok2 := ssaflow.GuardCondition(conditions[1])
		if !ok1 || !ok2 || (first == second) != want.same || firstStable != want.stable || secondStable != want.stable ||
			(firstNegated != secondNegated) != want.flipped {
			t.Errorf("%s: identities %q/%q negated %v/%v stable %v/%v", name, first, second, firstNegated, secondNegated, firstStable, secondStable)
		}
	}
	for _, condition := range branchConditions(pkg.Func("looped")) {
		if _, _, stable, ok := ssaflow.GuardCondition(condition); ok && stable {
			if call, isCall := condition.(*ssa.BinOp); isCall {
				if _, fromCall := call.X.(*ssa.Call); fromCall {
					t.Errorf("a call result recomputed in a loop is not stable: %v", condition)
				}
			}
		}
	}
}
