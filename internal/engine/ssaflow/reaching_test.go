package ssaflow

import (
	"slices"
	"testing"

	proofs "github.com/kojah/gohawk/internal/engine/proof"
	cfg "github.com/kojah/gohawk/internal/engine/ssaflow/cfg"
	"github.com/kojah/gohawk/internal/engine/ssaflow/ssaflowtest"
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
			cutoff := proofs.NewSearchBudget(1)
			if query.query(NewReachingWalk(0).Within(cutoff)) || !cutoff.Exhausted() {
				t.Fatal("phi root alone cannot prove its branches")
			}
			// Root plus both alternatives spend one shared allowance even
			// though must-fold branches have independent visited sets.
			fresh := proofs.NewSearchBudget(3)
			if !query.query(NewReachingWalk(0).Within(fresh)) || fresh.Exhausted() || fresh.Spend() {
				t.Fatal("must branches must share exactly three visits")
			}
			pool := proofs.NewSearchBudget(2)
			budget := pool.Within(10)
			if query.query(NewReachingWalk(0).Within(budget)) || !budget.PoolExhausted() {
				t.Fatal("branches must retain candidate-pool availability")
			}
		})
	}
	early := proofs.NewSearchBudget(2)
	if !NewReachingWalk(0).Within(early).Any(value, leaf) || early.Exhausted() || early.Spend() {
		t.Fatal("Any must stop at its first witness after two visits")
	}
	all := proofs.NewSearchBudget(1)
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
	cutoff := proofs.NewSearchBudget(1)
	if NewReachingWalk(TransparentMakeInterface).Within(cutoff).Any(value, leaf) || !cutoff.Exhausted() {
		t.Fatal("a wrapper must spend before visiting its operand")
	}
	fresh := proofs.NewSearchBudget(2)
	if !NewReachingWalk(TransparentMakeInterface).Within(fresh).Any(value, leaf) || fresh.Exhausted() {
		t.Fatal("fresh wrapper traversal lost its witness")
	}
	opaque := proofs.NewSearchBudget(1)
	if NewReachingWalk(0).Within(opaque).Any(value, leaf) || opaque.Exhausted() {
		t.Fatal("budget must not broaden the caller's transparent forms")
	}
	marked := proofs.NewSearchBudget(2)
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
			budget := proofs.NewSearchBudget(1)
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

func TestOpaquePhiFoldsPreserveIdentityBoundary(t *testing.T) {
	pkg := ssaflowtest.BuildPackage(t, "opaque", `package opaque
func merge(first, second int, choose bool) int64 {
    value := first
    if choose { value = second }
    return int64(value)
}
`)
	function := pkg.Func("merge")
	phis := InstructionsOf[*ssa.Phi](function)
	if len(phis) != 1 {
		t.Fatal("expected one merge")
	}
	result := InstructionsOf[*ssa.Return](function)[0].Results[0]
	first := function.Params[0]
	if !NewReachingWalk(TransparentConvert).Any(result, func(_ ReachingWalk, value ssa.Value) bool { return value == first }) {
		t.Fatal("default fold lost a reaching alternative")
	}
	for _, test := range []struct {
		name string
		fold func(ReachingWalk, ssa.Value) bool
	}{
		{"any", func(walk ReachingWalk, target ssa.Value) bool {
			return walk.Any(result, func(_ ReachingWalk, value ssa.Value) bool { return value == target })
		}},
		{"every", func(walk ReachingWalk, target ssa.Value) bool {
			return walk.EveryOf([]ssa.Value{result}, func(_ ReachingWalk, value ssa.Value) bool { return value == target })
		}},
		{"resolve", func(walk ReachingWalk, target ssa.Value) bool {
			_, ok := ResolveReachingValue(walk, result,
				func(_ ReachingWalk, value ssa.Value) (ssa.Value, bool) { return value, value == target },
				func(value ssa.Value) ssa.Value { return value })
			return ok
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			walk := func() ReachingWalk { return NewReachingWalk(TransparentConvert).OpaquePhis() }
			if test.fold(walk(), first) {
				t.Fatal("opaque merge borrowed identity from an alternative")
			}
			if !test.fold(walk(), phis[0]) {
				t.Fatal("opaque merge was not available to the leaf predicate")
			}
			if test.fold(walk().Within(proofs.NewSearchBudget(0)), phis[0]) {
				t.Fatal("exhausted fold supplied identity evidence")
			}
		})
	}
}

func TestReachingRevisitObservationPreservesResults(t *testing.T) {
	pkg := ssaflowtest.BuildPackage(t, "revisits", `package revisits
 func value(x int)int{return x}
 func siblings(flag bool,x int)int64{var r int64;if flag{r=int64(x)}else{r=int64(x)};return r}
 `)
	value := pkg.Func("value").Params[0]
	leaf := func(_ ReachingWalk, candidate ssa.Value) bool { return candidate == value }
	for _, mode := range []string{"any", "every", "mark", "resolve"} {
		observed := 0
		walk := NewReachingWalk(TransparentNone).OnRevisit(func() { observed++ })
		ask := func() bool {
			switch mode {
			case "any":
				return walk.Any(value, leaf)
			case "every":
				return walk.Every(value, leaf)
			case "mark":
				return walk.Mark(value)
			default:
				_, ok := ResolveReachingValue(walk, value,
					func(_ ReachingWalk, candidate ssa.Value) (ssa.Value, bool) { return candidate, candidate == value },
					func(candidate ssa.Value) string { return candidate.Name() })
				return ok
			}
		}
		if !ask() || observed != 0 || ask() || observed != 1 {
			t.Fatalf("%s guard/observation changed: revisits=%d", mode, observed)
		}
	}
	fn := pkg.Func("siblings")
	result := InstructionsOf[*ssa.Return](fn)[0].Results[0]
	observed := 0
	walk := NewReachingWalk(TransparentConvert).OnRevisit(func() { observed++ })
	if !walk.Every(result, func(_ ReachingWalk, value ssa.Value) bool { return value == fn.Params[1] }) || observed != 0 {
		t.Fatal("independent sibling origins were conflated")
	}
	walk = NewReachingWalk(TransparentConvert).OnRevisit(func() { observed++ })
	walk.Mark(fn.Params[1])
	if walk.Every(result, func(_ ReachingWalk, value ssa.Value) bool { return value == fn.Params[1] }) || observed != 1 {
		t.Fatal("sibling fold lost its revisit observer")
	}
}

func TestReachabilityBudgetAvailability(t *testing.T) {
	pkg := ssaflowtest.BuildPackage(t, "reachability", `package reachability
func marker() {}
func subject(branch bool) { marker(); if branch { marker() }; marker() }
`)
	start := InstructionsOf[*ssa.Call](pkg.Func("subject"))[0]
	cutoff := proofs.NewSearchBudget(0)
	if got := cfg.InstructionsReachableAfterWithin(start, cutoff); len(got) != 0 || !cutoff.Exhausted() {
		t.Fatalf("cutoff = %v, exhausted=%v", got, cutoff.Exhausted())
	}
	partial := proofs.NewSearchBudget(2)
	if got := cfg.InstructionsReachableAfterWithin(start, partial); len(got) == 0 || !partial.Exhausted() {
		t.Fatalf("expected partial census, got %v exhausted=%v", got, partial.Exhausted())
	}
	fresh := proofs.NewSearchBudget(proofs.QueryBudget)
	got := cfg.InstructionsReachableAfterWithin(start, fresh)
	if fresh.Exhausted() || !slices.Equal(got, cfg.InstructionsReachableAfter(start)) {
		t.Fatal("fresh census must share the default reachability policy")
	}
}
