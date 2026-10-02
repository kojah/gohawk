package ssaflow

import (
	"testing"

	"github.com/kojah/gohawk/internal/ssaflow/ssaflowtest"
	"golang.org/x/tools/go/ssa"
)

func TestReachingFoldBudgetBranchesAndEarlyWitness(t *testing.T) {
	pkg := ssaflowtest.BuildPackage(t, "folds", `package folds
func merge(flag bool, x,y int) int { v:=x; if flag {v=y}; return v }
`)
	function := pkg.Func("merge")
	value := InstructionsOf[*ssa.Return](function)[0].Results[0]
	if _, ok := value.(*ssa.Phi); !ok {
		t.Fatal("expected actual SSA phi")
	}
	leaf := func(ReachingWalk, ssa.Value) bool { return true }
	queries := []struct {
		name  string
		query func(ReachingWalk) bool
	}{
		{"every", func(walk ReachingWalk) bool { return walk.Every(value, leaf) }},
		{"resolve", func(walk ReachingWalk) bool {
			_, ok := ResolveReachingValue(walk, value, func(ReachingWalk, ssa.Value) (int, bool) { return 7, true }, func(v int) int { return v })
			return ok
		}},
	}
	for _, query := range queries {
		t.Run(query.name, func(t *testing.T) {
			cutoff := NewSearchBudget(1)
			if query.query(NewReachingWalk(0).Within(cutoff)) || !cutoff.Exhausted() {
				t.Fatal("phi root alone cannot prove its branches")
			}
			// Root plus both alternatives spend one shared allowance even
			// though must-fold branches have independent visited sets.
			fresh := NewSearchBudget(3)
			if !query.query(NewReachingWalk(0).Within(fresh)) || fresh.Exhausted() || fresh.Spend() {
				t.Fatal("must branches must share exactly three visits")
			}
			pool := NewSearchBudget(2)
			budget := pool.Within(10)
			if query.query(NewReachingWalk(0).Within(budget)) || !budget.PoolExhausted() {
				t.Fatal("branches must retain candidate-pool availability")
			}
		})
	}
	early := NewSearchBudget(2)
	if !NewReachingWalk(0).Within(early).Any(value, leaf) || early.Exhausted() || early.Spend() {
		t.Fatal("Any must stop at its first witness after two visits")
	}
	all := NewSearchBudget(1)
	if NewReachingWalk(0).Within(all).EveryOf([]ssa.Value{function.Params[1], function.Params[2]}, leaf) || !all.Exhausted() {
		t.Fatal("EveryOf branches must share their allowance")
	}
}

func TestReachingWrapperBudgetAndOpaqueForms(t *testing.T) {
	pkg := ssaflowtest.BuildPackage(t, "wrappers", `package wrappers
func boxed(x int) any { return x }
`)
	function := pkg.Func("boxed")
	value := InstructionsOf[*ssa.MakeInterface](function)[0]
	leaf := func(_ ReachingWalk, value ssa.Value) bool { return value == function.Params[0] }
	cutoff := NewSearchBudget(1)
	if NewReachingWalk(TransparentMakeInterface).Within(cutoff).Any(value, leaf) || !cutoff.Exhausted() {
		t.Fatal("a wrapper must spend before visiting its operand")
	}
	fresh := NewSearchBudget(2)
	if !NewReachingWalk(TransparentMakeInterface).Within(fresh).Any(value, leaf) || fresh.Exhausted() {
		t.Fatal("fresh wrapper traversal lost its witness")
	}
	opaque := NewSearchBudget(1)
	if NewReachingWalk(0).Within(opaque).Any(value, leaf) || opaque.Exhausted() {
		t.Fatal("budget must not broaden the caller's transparent forms")
	}
	marked := NewSearchBudget(2)
	walk := NewReachingWalk(0).Within(marked)
	if !walk.Mark(value) || walk.Mark(value) || marked.Exhausted() || marked.Spend() {
		t.Fatal("Mark must charge revisits without treating them as evidence")
	}
}

func TestReachingFoldRejectsExhaustedLeafEvidence(t *testing.T) {
	pkg := ssaflowtest.BuildPackage(t, "nested", `package nested
func identity(x int) int { return x }
`)
	value := pkg.Func("identity").Params[0]
	for _, query := range []string{"any", "every", "resolve"} {
		t.Run(query, func(t *testing.T) {
			budget := NewSearchBudget(1)
			walk := NewReachingWalk(0).Within(budget)
			leaf := func(ReachingWalk, ssa.Value) bool {
				budget.Spend()
				return true
			}
			var proved bool
			switch query {
			case "any":
				proved = walk.Any(value, leaf)
			case "every":
				proved = walk.Every(value, leaf)
			case "resolve":
				_, proved = ResolveReachingValue(walk, value, func(walk ReachingWalk, value ssa.Value) (int, bool) {
					return 7, leaf(walk, value)
				}, func(v int) int { return v })
			}
			if proved || !budget.Exhausted() {
				t.Fatal("an exhausted nested query cannot supply fold evidence")
			}
		})
	}
}
