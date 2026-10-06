package lifecycle

import (
	"testing"

	proofs "github.com/kojah/gohawk/internal/engine/proof"
	ssapath "github.com/kojah/gohawk/internal/engine/ssaflow/path"
	"golang.org/x/tools/go/ssa"
)

// The obligation walk is exercised through tiny functions whose calls stand
// in for classifier labels: settle is exact, opaque is unknown, anything else
// is none. Every function opens with start(), the instruction the obligation
// is attached to.
const obligationFixture = `package ssaflowtest
func start() {}
func settle() {}
func opaque() {}
func exactEverywhere(flag bool) { start(); if flag { settle(); return }; settle() }
func opaqueOneBranch(flag bool) { start(); if flag { opaque(); return }; settle() }
func earlyReturnUncovered(flag bool) { start(); if flag { return }; opaque() }
func opaqueEverywhere(flag bool) { start(); if flag { opaque(); return }; opaque() }
func exactAfterOpaque() { start(); opaque(); settle() }
func selectArms(done <-chan struct{}, other <-chan struct{}) { start(); select { case <-done: return; case <-other: return } }
func returnsOwner() *int { start(); return new(int) }
func guarded(cancel func()) { start(); if cancel != nil { cancel() } }
`

func obligationStart(t *testing.T, function *ssa.Function) ssa.Instruction {
	t.Helper()
	return findSSAInstruction(t, function, func(instruction ssa.Instruction) bool {
		call, ok := instruction.(*ssa.Call)
		return ok && call.Common().StaticCallee() != nil && call.Common().StaticCallee().Name() == "start"
	})
}

func labelledCall(instruction ssa.Instruction) ssapath.ObligationAction {
	call, ok := instruction.(*ssa.Call)
	if !ok || call.Common().StaticCallee() == nil {
		return ssapath.ObligationNone
	}
	switch call.Common().StaticCallee().Name() {
	case "settle":
		return ssapath.ObligationExact
	case "opaque":
		return ssapath.ObligationUnknown
	}
	return ssapath.ObligationNone
}

func TestEvaluateObligationKeepsUncertaintyOnItsPath(t *testing.T) {
	pkg := buildTestSSA(t, obligationFixture)
	want := map[string]ssapath.ObligationOutcome{
		"exactEverywhere": ssapath.ObligationHonored,
		"opaqueOneBranch": ssapath.ObligationUncertain,
		// An opaque handoff on the other branch must not excuse this early
		// return: nothing on its path touched the obligation.
		"earlyReturnUncovered": ssapath.ObligationViolated,
		"opaqueEverywhere":     ssapath.ObligationUncertain,
		"exactAfterOpaque":     ssapath.ObligationHonored,
	}
	for name, expected := range want {
		function := pkg.Func(name)
		start := obligationStart(t, function)
		got := ssapath.EvaluateObligation(ssapath.ObligationFlow{Start: start, Instruction: labelledCall})
		if got != expected {
			t.Errorf("%s: outcome %d, want %d", name, got, expected)
		}
		// The Boolean family is the same walk on a two-level lattice, so it
		// must agree about violation exactly.
		owns := func(instruction ssa.Instruction) bool { return labelledCall(instruction) != ssapath.ObligationNone }
		if unowned := ssapath.UnownedReturn(ssapath.UnownedReturnQuery{After: start, Owns: owns}) != nil; unowned != (expected == ssapath.ObligationViolated) {
			t.Errorf("%s: UnownedReturn = %v disagrees with outcome %d", name, unowned, expected)
		}
	}
}

func TestEvaluateObligationBudgetCutIsUncertain(t *testing.T) {
	pkg := buildTestSSA(t, obligationFixture)
	for _, name := range []string{"exactEverywhere", "earlyReturnUncovered"} {
		function := pkg.Func(name)
		start := obligationStart(t, function)
		got := ssapath.EvaluateObligation(ssapath.ObligationFlow{Start: start, Instruction: labelledCall, Budget: proofs.NewSearchBudget(0)})
		if got != ssapath.ObligationUncertain {
			t.Errorf("%s: budget cut outcome = %d, want uncertain", name, got)
		}
	}
}

func TestEvaluateObligationEdgeActionsStayOnTheirSuccessor(t *testing.T) {
	pkg := buildTestSSA(t, obligationFixture)
	function := pkg.Func("selectArms")
	start := obligationStart(t, function)
	joinedOn := func(channel ssa.Value) func(from, to *ssa.BasicBlock) ssapath.ObligationAction {
		return func(from, to *ssa.BasicBlock) ssapath.ObligationAction {
			if received, ok := ssapath.SelectedReceiveOnEdge(from, to); ok && received == channel {
				return ssapath.ObligationExact
			}
			return ssapath.ObligationNone
		}
	}
	firstArm := ssapath.ObligationFlow{Start: start, Instruction: labelledCall, Edge: joinedOn(function.Params[0])}
	if got := ssapath.EvaluateObligation(firstArm); got != ssapath.ObligationViolated {
		t.Errorf("a join on one select arm covered the other arm: outcome %d", got)
	}
	both := func(from, to *ssa.BasicBlock) ssapath.ObligationAction {
		return max(joinedOn(function.Params[0])(from, to), joinedOn(function.Params[1])(from, to))
	}
	if got := ssapath.EvaluateObligation(ssapath.ObligationFlow{Start: start, Instruction: labelledCall, Edge: both}); got != ssapath.ObligationHonored {
		t.Errorf("joins on every select arm were not honored: outcome %d", got)
	}
}

func TestEvaluateObligationReturnAndNonNilPolicy(t *testing.T) {
	pkg := buildTestSSA(t, obligationFixture)
	returns := pkg.Func("returnsOwner")
	start := obligationStart(t, returns)
	transferred := func(*ssa.Return) ssapath.ObligationAction { return ssapath.ObligationExact }
	transferFlow := ssapath.ObligationFlow{Start: start, Instruction: labelledCall, Return: transferred}
	if got := ssapath.EvaluateObligation(transferFlow); got != ssapath.ObligationHonored {
		t.Errorf("a transferring return was not honored: outcome %d", got)
	}
	if got := ssapath.EvaluateObligation(ssapath.ObligationFlow{Start: start, Instruction: labelledCall}); got != ssapath.ObligationViolated {
		t.Errorf("a plain return with no action was not a violation: outcome %d", got)
	}

	guarded := pkg.Func("guarded")
	start = obligationStart(t, guarded)
	cancel := guarded.Params[0]
	invokes := func(instruction ssa.Instruction) ssapath.ObligationAction {
		if call, ok := instruction.(*ssa.Call); ok && call.Common().Value == cancel {
			return ssapath.ObligationExact
		}
		return ssapath.ObligationNone
	}
	if got := ssapath.EvaluateObligation(ssapath.ObligationFlow{Start: start, NonNil: cancel, Instruction: invokes}); got != ssapath.ObligationHonored {
		t.Errorf("the nil branch was walked despite the non-nil assumption: outcome %d", got)
	}
	if got := ssapath.EvaluateObligation(ssapath.ObligationFlow{Start: start, Instruction: invokes}); got != ssapath.ObligationViolated {
		t.Errorf("the nil branch was pruned without a non-nil assumption: outcome %d", got)
	}
}

// A caller-supplied feasibility view prunes the early return the default
// view cannot, and the non-nil assumption still applies on top of it.
func TestEvaluateObligationUsesSuppliedSuccessors(t *testing.T) {
	pkg := buildTestSSA(t, obligationFixture)
	function := pkg.Func("earlyReturnUncovered")
	start := obligationStart(t, function)
	flow := ssapath.ObligationFlow{Start: start, Instruction: labelledCall}
	if got := ssapath.EvaluateObligation(flow); got != ssapath.ObligationViolated {
		t.Fatalf("default feasibility: outcome %d, want violated", got)
	}
	flow.Successors = func(block, predecessor *ssa.BasicBlock) []*ssa.BasicBlock {
		successors := ssapath.FeasibleSuccessorsWithin(block, predecessor, nil)
		if len(successors) == 2 {
			return successors[1:]
		}
		return successors
	}
	if got := ssapath.EvaluateObligation(flow); got != ssapath.ObligationUncertain {
		t.Fatalf("supplied feasibility: outcome %d, want uncertain (only the opaque path remains)", got)
	}
}
