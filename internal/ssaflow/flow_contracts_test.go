package ssaflow_test

import (
	"slices"
	"testing"

	"github.com/kojah/gohawk/internal/ssaflow"
	"github.com/kojah/gohawk/internal/ssaflow/ssaflowtest"
	"golang.org/x/tools/go/ssa"
)

// Contracts of the flow engine, stated in its own vocabulary so an engine bug
// cannot hide behind an analyzer's conservative decline.

const obligationSource = `package obligations
type box struct{ ready bool }
func start() {}
func own()   {}
func opaque() {}
func after() {}
func use(int) {}
func honored(b bool)   { start(); if b { own() } else { own() } }
func uncertain(b bool) { start(); if b { own() } else { opaque() } }
func violated(b bool)  { start(); if b { own() } }
func stable(b bool)    { start(); if b { own() }; if !b { own() } }
func loaded(p *box)    { start(); if p.ready { own() }; if !p.ready { own() } }
func looping(n int)    { for i := 0; i < n; i++ { use(i) }; after() }
`

func obligationPackage(t *testing.T) *ssa.Package {
	t.Helper()
	return ssaflowtest.BuildPackage(t, "obligations", obligationSource)
}

func callNamed(t *testing.T, function *ssa.Function, name string) *ssa.Call {
	t.Helper()
	for _, call := range ssaflow.InstructionsOf[*ssa.Call](function) {
		if ssaflow.CallName(call.Common()) == name {
			return call
		}
	}
	t.Fatalf("%s: no call to %s", function.Name(), name)
	return nil
}

func callsTo(name string) func(ssa.Instruction) ssaflow.ObligationAction {
	return func(instruction ssa.Instruction) ssaflow.ObligationAction {
		call, ok := instruction.(*ssa.Call)
		if !ok {
			return ssaflow.ObligationNone
		}
		switch ssaflow.CallName(call.Common()) {
		case name:
			return ssaflow.ObligationExact
		case "opaque":
			return ssaflow.ObligationUnknown
		}
		return ssaflow.ObligationNone
	}
}

// WalkStates expands each key once, so a block is revisited only under a new
// key, and step can end the walk early.
func TestWalkStatesKeysAndTermination(t *testing.T) {
	var expanded []int
	ssaflow.WalkStates([]int{0}, func(n int) int { return n }, func(n int) ([]int, bool) {
		expanded = append(expanded, n)
		if n == 5 {
			return nil, true
		}
		return []int{n + 1, n + 1}, true
	})
	if !slices.Equal(expanded, []int{0, 1, 2, 3, 4, 5}) {
		t.Errorf("identity key expanded %v, want each state once", expanded)
	}
	expanded = nil
	ssaflow.WalkStates([]int{0}, func(n int) int { return n % 2 }, func(n int) ([]int, bool) {
		expanded = append(expanded, n)
		return []int{n + 1}, true
	})
	if !slices.Equal(expanded, []int{0, 1}) {
		t.Errorf("parity key expanded %v, want one state per key", expanded)
	}
	expanded = nil
	ssaflow.WalkStates([]int{0, 1, 2}, func(n int) int { return n }, func(n int) ([]int, bool) {
		expanded = append(expanded, n)
		return nil, n != 1
	})
	if !slices.Equal(expanded, []int{0, 1}) {
		t.Errorf("early stop expanded %v, want the walk to end at 1", expanded)
	}
}

// InstructionsReachableAfter does not cross a loop back edge: from inside the
// body it never returns to the header, so what follows the loop is reachable
// only from before it.
func TestInstructionsReachableAfterStopsAtBackEdges(t *testing.T) {
	function := obligationPackage(t).Func("looping")
	inBody := ssaflow.InstructionsReachableAfter(callNamed(t, function, "use"))
	if slices.Contains(inBody, ssa.Instruction(callNamed(t, function, "after"))) {
		t.Error("from the loop body, the call after the loop must not be reachable")
	}
	for _, instruction := range inBody {
		if _, ok := instruction.(*ssa.Phi); ok {
			t.Errorf("from the loop body, the header phi %v must not be reachable", instruction)
		}
	}
	fromEntry := ssaflow.InstructionsReachableAfter(function.Blocks[0].Instrs[0])
	if !slices.Contains(fromEntry, ssa.Instruction(callNamed(t, function, "after"))) {
		t.Error("from entry, the call after the loop must be reachable")
	}
}

// EvaluateObligation returns the weakest coverage on any feasible path, with
// the violating return as witness.
func TestEvaluateObligationOutcomes(t *testing.T) {
	pkg := obligationPackage(t)
	for name, want := range map[string]ssaflow.ObligationOutcome{
		"honored":   ssaflow.ObligationHonored,
		"uncertain": ssaflow.ObligationUncertain,
		"violated":  ssaflow.ObligationViolated,
		// The b-false then !b-false path is infeasible: b is a stable guard.
		"stable": ssaflow.ObligationHonored,
		// p.ready is loaded twice, so the second read may differ: the
		// contradicting path is uncertain, not pruned.
		"loaded": ssaflow.ObligationUncertain,
	} {
		function := pkg.Func(name)
		outcome, witness := ssaflow.EvaluateObligationWitness(ssaflow.ObligationFlow{
			Start: callNamed(t, function, "start"), Instruction: callsTo("own"),
		})
		if outcome != want {
			t.Errorf("%s: outcome %d, want %d", name, outcome, want)
		}
		if (witness != nil) != (want == ssaflow.ObligationViolated) {
			t.Errorf("%s: witness %v with outcome %d", name, witness, outcome)
		}
	}
}

// Extending path guards across an edge that takes the other arm of a branch
// already taken on the path is a contradiction: stable for a parameter,
// loaded for a value read from memory again.
func TestPathGuardsContradictionPerEdge(t *testing.T) {
	pkg := obligationPackage(t)
	for name, want := range map[string]ssaflow.GuardContradiction{
		"stable": ssaflow.GuardStableContradiction,
		"loaded": ssaflow.GuardLoadedContradiction,
	} {
		branches := ssaflow.InstructionsOf[*ssa.If](pkg.Func(name))
		if len(branches) != 2 {
			t.Fatalf("%s: %d branches", name, len(branches))
		}
		first, second := branches[0].Block(), branches[1].Block()
		// Take the first branch's else arm, then reach the second branch.
		guards, contradiction := ssaflow.PathGuards(nil).ExtendWithin(first, first.Succs[1], nil, nil)
		if contradiction != ssaflow.GuardConsistent {
			t.Fatalf("%s: first edge %d", name, contradiction)
		}
		// The builder lowers the inline !b to a branch on b with its arms
		// swapped, so the second branch's first successor is the b-true arm,
		// which contradicts the b-false arm already taken, and its second
		// successor agrees with it.
		_, contradiction = guards.ExtendWithin(second, second.Succs[0], nil, nil)
		if contradiction != want {
			t.Errorf("%s: contradiction %d, want %d", name, contradiction, want)
		}
		if _, consistent := guards.ExtendWithin(second, second.Succs[1], nil, nil); consistent != ssaflow.GuardConsistent {
			t.Errorf("%s: the agreeing arm reported %d", name, consistent)
		}
	}
}

// UnownedReturn's query starts the obligation after an instruction, at entry,
// or with an edge or return rule, and reports the unowned return or nil.
func TestUnownedReturnQuery(t *testing.T) {
	pkg := obligationPackage(t)
	owns := func(instruction ssa.Instruction) bool {
		call, ok := instruction.(*ssa.Call)
		return ok && ssaflow.CallName(call.Common()) == "own"
	}
	violated := pkg.Func("violated")
	if ssaflow.UnownedReturn(ssaflow.UnownedReturnQuery{After: callNamed(t, violated, "start"), Owns: owns}) == nil {
		t.Error("after start: the b-false return is unowned")
	}
	if ssaflow.UnownedReturn(ssaflow.UnownedReturnQuery{Entry: violated, Owns: owns}) == nil {
		t.Error("from entry: the b-false return is unowned")
	}
	allowAll := func(*ssa.Return) bool { return true }
	if ssaflow.UnownedReturn(ssaflow.UnownedReturnQuery{Entry: violated, Owns: owns, AllowReturn: allowAll}) != nil {
		t.Error("a return the rule allows needs no action")
	}
	branch := ssaflow.InstructionsOf[*ssa.If](violated)[0].Block()
	elseEdge := func(from, to *ssa.BasicBlock) bool { return from == branch && to == branch.Succs[1] }
	if ssaflow.UnownedReturn(ssaflow.UnownedReturnQuery{Entry: violated, Owns: owns, OwnsEdge: elseEdge}) != nil {
		t.Error("an owning edge on the else arm covers the b-false return")
	}
	if ssaflow.UnownedReturn(ssaflow.UnownedReturnQuery{Entry: pkg.Func("honored"), Owns: owns}) != nil {
		t.Error("honored: every return is owned")
	}
	if ssaflow.UnownedReturn(ssaflow.UnownedReturnQuery{Owns: owns}) != nil {
		t.Error("a query with no start has no paths")
	}
}
