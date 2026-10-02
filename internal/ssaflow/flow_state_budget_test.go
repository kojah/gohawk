package ssaflow

import (
	"testing"

	"github.com/kojah/gohawk/internal/ssaflow/ssaflowtest"
	"golang.org/x/tools/go/ssa"
)

func TestWalkStatesBudgetBeforeKeyAndRevisits(t *testing.T) {
	keys, steps := 0, 0
	zero := NewSearchBudget(0)
	WalkStatesWithin([]int{0}, func(n int) int { keys++; return n }, func(int) ([]int, bool) { steps++; return nil, true }, zero)
	if keys != 0 || steps != 0 || !zero.Exhausted() {
		t.Fatal("queued visits must spend before constructing keys")
	}
	fresh := NewSearchBudget(3)
	WalkStatesWithin([]int{0}, func(n int) int { return n % 2 }, func(n int) ([]int, bool) {
		steps++
		return []int{n + 1}, true
	}, fresh)
	if steps != 2 || fresh.Exhausted() || fresh.Spend() {
		t.Fatal("revisited keys must spend without expanding a third state")
	}
	for _, phase := range []string{"key", "step"} {
		budget := NewSearchBudget(3)
		keys, steps = 0, 0
		WalkStatesWithin([]int{0}, func(n int) int {
			keys++
			if phase == "key" {
				for budget.Spend() {
				}
			}
			return n
		}, func(n int) ([]int, bool) {
			steps++
			for budget.Spend() {
			}
			return []int{n + 1}, true
		}, budget)
		wantSteps := 1
		if phase == "key" {
			wantSteps = 0
		}
		if keys != 1 || steps != wantSteps || !budget.Exhausted() {
			t.Fatal("interrupted keys/successors must stop before admission")
		}
	}
}

func TestGuardStateBudgetKeepsDefaultPolicy(t *testing.T) {
	pkg := ssaflowtest.BuildPackage(t, "guards", `package guards
 type owner struct { flag bool }
 func subject(p *owner) { if p.flag { p.flag=false } }
 func stable(flag bool) { if flag { subject(nil) } }
`)
	function := pkg.Func("subject")
	branch := InstructionsOf[*ssa.If](function)[0]
	identity, _, _, ok := GuardCondition(branch.Cond)
	if !ok {
		t.Fatal("expected an actual loaded guard")
	}
	guards := PathGuards{{Identity: identity, Value: true, Stable: false}}
	store := InstructionsOf[*ssa.Store](function)[0]
	zero := NewSearchBudget(0)
	if kept := guards.AfterWithin(store, zero); kept != nil || !zero.Exhausted() {
		t.Fatal("unknown store identity cannot retain an active guard")
	}
	fresh := NewSearchBudget(QueryBudget)
	if kept := guards.AfterWithin(store, fresh); len(kept) != 0 || len(guards.After(store)) != 0 || fresh.Exhausted() {
		t.Fatal("fresh mutation must forget the same guard")
	}
	zero = NewSearchBudget(0)
	if key := guards.KeyWithin(zero); key != "" || !zero.Exhausted() {
		t.Fatal("partial state-key rendering must be unavailable")
	}
	if guards.KeyWithin(NewSearchBudget(QueryBudget)) != guards.Key() {
		t.Fatal("fresh guard key must preserve default identity")
	}
	zero = NewSearchBudget(0)
	if edges := (SuccessorPolicy{}).EdgesWithin(branch.Block(), nil, guards, zero); edges != nil || !zero.Exhausted() {
		t.Fatal("edge census must spend before successor selection")
	}
	// Taking the other branch contradicts the loaded guard only uncertainly.
	_, loaded := guards.ExtendWithin(branch.Block(), branch.Block().Succs[1], nil, NewSearchBudget(QueryBudget))
	guards[0].Stable = true
	// Stability comes from the decoded condition, not an asserted held flag.
	_, sameLoaded := guards.Extend(branch.Block(), branch.Block().Succs[1], nil)
	if loaded != GuardLoadedContradiction || sameLoaded != loaded {
		t.Fatal("bounded extension must preserve loaded contradiction semantics")
	}
	stableBranch := InstructionsOf[*ssa.If](pkg.Func("stable"))[0]
	stableIdentity, _, stable, ok := GuardCondition(stableBranch.Cond)
	if !ok || !stable {
		t.Fatal("expected an actual stable parameter guard")
	}
	stableGuards := PathGuards{{Identity: stableIdentity, Value: true, Stable: true}}
	_, contradiction := stableGuards.ExtendWithin(stableBranch.Block(), stableBranch.Block().Succs[1], nil, NewSearchBudget(QueryBudget))
	if contradiction != GuardStableContradiction {
		t.Fatal("bounded extension must preserve stable path pruning")
	}
}

func TestObligationRejectsExhaustedCallbacks(t *testing.T) {
	pkg := ssaflowtest.BuildPackage(t, "callbacks", `package callbacks
 func marker(n int) {}
 func subject(flag bool) { marker(0); marker(1); if flag { marker(2) } }
`)
	function := pkg.Func("subject")
	start := InstructionsOf[*ssa.Call](function)[0]
	for _, phase := range []string{"instruction", "return", "edge", "successors", "terminator"} {
		t.Run(phase, func(t *testing.T) {
			budget := NewSearchBudget(QueryBudget)
			called := false
			cut := func() {
				called = true
				for budget.Spend() {
				}
			}
			flow := ObligationFlow{Start: start, Budget: budget, Instruction: func(ssa.Instruction) ObligationAction { return ObligationNone }}
			switch phase {
			case "instruction":
				flow.Instruction = func(ssa.Instruction) ObligationAction { cut(); return ObligationExact }
			case "return":
				flow.Return = func(*ssa.Return) ObligationAction { cut(); return ObligationNone }
			case "edge":
				flow.Edge = func(*ssa.BasicBlock, *ssa.BasicBlock) ObligationAction { cut(); return ObligationExact }
			case "successors":
				flow.Successors = func(block, _ *ssa.BasicBlock) []*ssa.BasicBlock { cut(); return block.Succs }
			case "terminator":
				flow.Terminates = func(*ssa.Call) bool { cut(); return true }
			}
			outcome, witness := EvaluateObligationWitness(flow)
			if !called || !budget.Exhausted() || outcome != ObligationUncertain || witness != nil {
				t.Fatal("an interrupted callback cannot settle, violate or terminate a path")
			}
		})
	}
}
