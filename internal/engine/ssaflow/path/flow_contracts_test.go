package path_test

import (
	"slices"
	"testing"

	proofs "github.com/kojah/gohawk/internal/engine/proof"
	"github.com/kojah/gohawk/internal/engine/ssaflow"
	ssacall "github.com/kojah/gohawk/internal/engine/ssaflow/calls"
	cfg "github.com/kojah/gohawk/internal/engine/ssaflow/cfg"
	ssapath "github.com/kojah/gohawk/internal/engine/ssaflow/path"
	"github.com/kojah/gohawk/internal/engine/ssaflow/ssaflowtest"
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

func callsTo(name string) func(ssa.Instruction) ssapath.ObligationAction {
	return func(instruction ssa.Instruction) ssapath.ObligationAction {
		call, ok := instruction.(*ssa.Call)
		if !ok {
			return ssapath.ObligationNone
		}
		switch ssaflow.CallName(call.Common()) {
		case name:
			return ssapath.ObligationExact
		case "opaque":
			return ssapath.ObligationUnknown
		}
		return ssapath.ObligationNone
	}
}

// WalkStates expands each key once, so a block is revisited only under a new
// key, and step can end the walk early.
func TestWalkStatesKeysAndTermination(t *testing.T) {
	var expanded []int
	cfg.WalkStates([]int{0}, func(n int) int { return n }, func(n int) ([]int, bool) {
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
	cfg.WalkStates([]int{0}, func(n int) int { return n % 2 }, func(n int) ([]int, bool) {
		expanded = append(expanded, n)
		return []int{n + 1}, true
	})
	if !slices.Equal(expanded, []int{0, 1}) {
		t.Errorf("parity key expanded %v, want one state per key", expanded)
	}
	expanded = nil
	cfg.WalkStates([]int{0, 1, 2}, func(n int) int { return n }, func(n int) ([]int, bool) {
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
	inBody := cfg.InstructionsReachableAfter(callNamed(t, function, "use"))
	if slices.Contains(inBody, ssa.Instruction(callNamed(t, function, "after"))) {
		t.Error("from the loop body, the call after the loop must not be reachable")
	}
	for _, instruction := range inBody {
		if _, ok := instruction.(*ssa.Phi); ok {
			t.Errorf("from the loop body, the header phi %v must not be reachable", instruction)
		}
	}
	fromEntry := cfg.InstructionsReachableAfter(function.Blocks[0].Instrs[0])
	if !slices.Contains(fromEntry, ssa.Instruction(callNamed(t, function, "after"))) {
		t.Error("from entry, the call after the loop must be reachable")
	}
}

// EvaluateObligation returns the weakest coverage on any feasible path, with
// the violating return as witness.
func TestEvaluateObligationOutcomes(t *testing.T) {
	pkg := obligationPackage(t)
	for name, want := range map[string]ssapath.ObligationOutcome{
		"honored":   ssapath.ObligationHonored,
		"uncertain": ssapath.ObligationUncertain,
		"violated":  ssapath.ObligationViolated,
		// The b-false then !b-false path is infeasible: b is a stable guard.
		"stable": ssapath.ObligationHonored,
		// p.ready is loaded twice, so the second read may differ: the
		// contradicting path is uncertain, not pruned.
		"loaded": ssapath.ObligationUncertain,
	} {
		function := pkg.Func(name)
		outcome, witness := ssapath.EvaluateObligationWitness(ssapath.ObligationFlow{
			Start: callNamed(t, function, "start"), Instruction: callsTo("own"),
		})
		if outcome != want {
			t.Errorf("%s: outcome %d, want %d", name, outcome, want)
		}
		if (witness != nil) != (want == ssapath.ObligationViolated) {
			t.Errorf("%s: witness %v with outcome %d", name, witness, outcome)
		}
	}
}

// Extending path guards across an edge that takes the other arm of a branch
// already taken on the path is a contradiction: stable for a parameter,
// loaded for a value read from memory again.
func TestPathGuardsContradictionPerEdge(t *testing.T) {
	pkg := obligationPackage(t)
	for name, want := range map[string]ssapath.GuardContradiction{
		"stable": ssapath.GuardStableContradiction,
		"loaded": ssapath.GuardLoadedContradiction,
	} {
		branches := ssaflow.InstructionsOf[*ssa.If](pkg.Func(name))
		if len(branches) != 2 {
			t.Fatalf("%s: %d branches", name, len(branches))
		}
		first, second := branches[0].Block(), branches[1].Block()
		// Take the first branch's else arm, then reach the second branch.
		guards, contradiction := ssapath.PathGuards(nil).ExtendWithin(first, first.Succs[1], nil, nil)
		if contradiction != ssapath.GuardConsistent {
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
		if _, consistent := guards.ExtendWithin(second, second.Succs[1], nil, nil); consistent != ssapath.GuardConsistent {
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
	if ssapath.UnownedReturn(ssapath.UnownedReturnQuery{After: callNamed(t, violated, "start"), Owns: owns}) == nil {
		t.Error("after start: the b-false return is unowned")
	}
	if ssapath.UnownedReturn(ssapath.UnownedReturnQuery{Entry: violated, Owns: owns}) == nil {
		t.Error("from entry: the b-false return is unowned")
	}
	allowAll := func(*ssa.Return) bool { return true }
	if ssapath.UnownedReturn(ssapath.UnownedReturnQuery{Entry: violated, Owns: owns, AllowReturn: allowAll}) != nil {
		t.Error("a return the rule allows needs no action")
	}
	branch := ssaflow.InstructionsOf[*ssa.If](violated)[0].Block()
	elseEdge := func(from, to *ssa.BasicBlock) bool { return from == branch && to == branch.Succs[1] }
	if ssapath.UnownedReturn(ssapath.UnownedReturnQuery{Entry: violated, Owns: owns, OwnsEdge: elseEdge}) != nil {
		t.Error("an owning edge on the else arm covers the b-false return")
	}
	if ssapath.UnownedReturn(ssapath.UnownedReturnQuery{Entry: pkg.Func("honored"), Owns: owns}) != nil {
		t.Error("honored: every return is owned")
	}
	if ssapath.UnownedReturn(ssapath.UnownedReturnQuery{Owns: owns}) != nil {
		t.Error("a query with no start has no paths")
	}
}

const reachabilityFixture = `package reachability
func mark(value int) {}
func branch(yes bool) {
	mark(1)
	mark(2)
	if yes { mark(3) } else { mark(4) }
	mark(5)
}
func cycle(yes bool) { for yes { mark(6) }; mark(7) }
func other() { mark(8) }
`

func TestReachabilitySeedAndInstructionOrder(t *testing.T) {
	t.Parallel()
	pkg := ssaflowtest.BuildPackage(t, "reachability", reachabilityFixture)
	branch := pkg.Func("branch")
	entry := branch.Blocks[0]
	left, right := entry.Succs[0], entry.Succs[1]
	before, after := entry.Instrs[0], entry.Instrs[1]
	if !cfg.BlockReachable(entry, entry) || cfg.BlockInCycle(entry) {
		t.Error("acyclic entry must reach itself without being in a cycle")
	}
	if !cfg.InstructionMayFollow(before, after) || cfg.InstructionMayFollow(after, before) {
		t.Error("instructions in one block must retain source order")
	}
	if !cfg.InstructionMayFollow(before, left.Instrs[0]) || cfg.InstructionMayFollow(left.Instrs[0], right.Instrs[0]) {
		t.Error("entry reaches each arm; sibling arms cannot reach each other")
	}
	other := pkg.Func("other").Blocks[0]
	if cfg.BlockReachable(entry, other) || cfg.BlockReachable(nil, entry) || cfg.BlockReachable(entry, nil) ||
		cfg.InstructionMayFollow(before, other.Instrs[0]) || cfg.InstructionMayFollow(nil, after) {
		t.Error("missing or cross-function starts must not establish reachability")
	}
}

func TestCycleReachabilityPreservesInstructionsAndSuccessors(t *testing.T) {
	t.Parallel()
	pkg := ssaflowtest.BuildPackage(t, "reachability", reachabilityFixture)
	cycle := pkg.Func("cycle")
	if cfg.BlockInCycle(cycle.Blocks[0]) {
		t.Error("entering a loop does not put its acyclic entry in the cycle")
	}
	foundCycle := false
	for _, block := range cycle.Blocks {
		seeds := slices.Clone(block.Succs)
		inCycle := cfg.BlockInCycle(block)
		foundCycle = foundCycle || inCycle
		if inCycle && len(block.Instrs) > 1 {
			first, last := block.Instrs[0], block.Instrs[len(block.Instrs)-1]
			if !cfg.InstructionMayFollow(first, last) || cfg.InstructionMayFollow(last, first) {
				t.Errorf("cycle reversed instruction order in block %d", block.Index)
			}
		}
		if !cfg.BlockReachable(cycle.Blocks[0], block) {
			t.Errorf("entry cannot reach fixture block %d", block.Index)
		}
		if !slices.Equal(seeds, block.Succs) {
			t.Errorf("reachability changed successors of block %d", block.Index)
		}
	}
	if !foundCycle {
		t.Fatal("loop fixture produced no cycle")
	}
}

func TestBlockReachableWithinAllowance(t *testing.T) {
	t.Parallel()
	pkg := ssaflowtest.BuildPackage(t, "reachability", reachabilityFixture)
	entry := pkg.Func("branch").Blocks[0]
	left, right := entry.Succs[0], entry.Succs[1]
	for _, test := range []struct {
		name         string
		from, target int
		want         bool
	}{
		{"reachable", entry.Index, left.Index, true},
		{"sibling", left.Index, right.Index, false},
	} {
		t.Run(test.name, func(t *testing.T) {
			fn := pkg.Func("branch")
			from, target := fn.Blocks[test.from], fn.Blocks[test.target]
			pool := proofs.NewSearchBudget(100)
			cut := pool.Within(0)
			if cfg.BlockReachableWithin(from, target, cut) || !cut.Exhausted() || pool.Exhausted() {
				t.Fatal("child cutoff must leave reachability unavailable and parent available")
			}
			fresh := pool.Within(100)
			if got := cfg.BlockReachableWithin(from, target, fresh); got != test.want || fresh.Exhausted() {
				t.Fatalf("fresh reachability=%v, want %v; exhausted %v", got, test.want, fresh.Exhausted())
			}
		})
	}
}

func TestReturnAndActionWitnesses(t *testing.T) {
	t.Parallel()
	pkg := ssaflowtest.BuildPackage(t, "witnesses", `package witnesses
func mark(value int) {}
func missingAction() {}
func missingReturn() { mark(0); for {} }
func earlyAction() { mark(1); mark(2) }
func optionalAction(flag bool) { if flag { mark(3) } }
`)
	mark := func(instruction ssa.Instruction) bool {
		call, ok := instruction.(*ssa.Call)
		return ok && call.Common().StaticCallee() == pkg.Func("mark")
	}
	for name, want := range map[string]bool{
		"missingAction": false, "missingReturn": false, "earlyAction": true, "optionalAction": true,
	} {
		if got := ssaflow.HasReturnAndAction(pkg.Func(name).Blocks, mark); got != want {
			t.Errorf("%s: got witnesses %v, want %v", name, got, want)
		}
	}
	optional := pkg.Func("optionalAction")
	blocks := ssapath.ReachableBlocksAssumingWithin(optional, ssacall.FixedValues{optional.Params[0]: ssacall.OutcomeFalse}, nil)
	if ssaflow.HasReturnAndAction(blocks, mark) || ssaflow.HasReturnAndAction(nil, mark) {
		t.Error("filtered or absent blocks supplied an action witness")
	}
	// A first action must not stop the return scan or cause further predicate
	// calls. This matters when the predicate spends a shared query budget.
	queries := 0
	if !ssaflow.HasReturnAndAction(pkg.Func("earlyAction").Blocks, func(instruction ssa.Instruction) bool {
		queries++
		return mark(instruction)
	}) || queries != 1 {
		t.Errorf("early action: predicate queries %d, want one with both witnesses", queries)
	}
}
