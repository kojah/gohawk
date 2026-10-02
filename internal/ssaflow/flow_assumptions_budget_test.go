package ssaflow

import (
	"go/types"
	"slices"
	"testing"

	"github.com/kojah/gohawk/internal/ssaflow/ssaflowtest"
	"golang.org/x/tools/go/ssa"
)

func TestBoundConditionAllowance(t *testing.T) {
	pkg := ssaflowtest.BuildPackage(t, "assumptions", `package assumptions
func direct(flag bool) { if flag { println(1) } }
func negated(flag bool) { if !flag { println(1) } }
func nilCheck(p *int) { if p != nil { println(1) } }
func reversed(p *int) { if nil != p { println(1) } }
func odd(flag bool) bool { return !flag }
`)
	for _, test := range []struct {
		name    string
		outcome Outcome
		holds   bool
	}{
		{"direct", OutcomeTrue, true},
		// SSA swaps edges for if !flag; its branch condition is still flag.
		{"negated", OutcomeTrue, true},
		{"nilCheck", OutcomeNonNil, true},
		{"reversed", OutcomeNonNil, true},
	} {
		function := pkg.Func(test.name)
		branch := InstructionsOf[*ssa.If](function)[0]
		fixed := FixedValues{function.Params[0]: test.outcome}
		fresh := NewSearchBudget(QueryBudget)
		holds, known := fixed.HoldsWithin(branch.Cond, fresh)
		if !known || holds != test.holds || fresh.Exhausted() {
			t.Fatalf("%s condition %s: holds=%t known=%t exhausted=%t", test.name, branch.Cond, holds, known, fresh.Exhausted())
		}
		for limit := range QueryBudget - fresh.remaining {
			cut := NewSearchBudget(limit)
			if _, known := fixed.HoldsWithin(branch.Cond, cut); known || !cut.Exhausted() {
				t.Fatal("an interrupted condition cannot decide a branch")
			}
		}
		cut := NewSearchBudget(0)
		if got := fixed.NarrowWithin(branch.Block().Succs, branch.Block(), cut); !slices.Equal(got, branch.Block().Succs) || !cut.Exhausted() {
			t.Fatal("interrupted narrowing must keep the primitive's edges unpruned")
		}
		got := fixed.NarrowWithin(branch.Block().Succs, branch.Block(), NewSearchBudget(QueryBudget))
		if !slices.Equal(got, fixed.Narrow(branch.Block().Succs, branch.Block())) {
			t.Fatal("fresh bound filtering must preserve default edges")
		}
	}
	odd := pkg.Func("odd")
	condition := InstructionsOf[*ssa.Return](odd)[0].Results[0]
	if _, ok := condition.(*ssa.UnOp); !ok {
		t.Fatal("expected an actual returned negation")
	}
	fixed := FixedValues{odd.Params[0]: OutcomeTrue}
	fresh := NewSearchBudget(QueryBudget)
	if holds, known := fixed.HoldsWithin(condition, fresh); holds || !known || fresh.Exhausted() {
		t.Fatal("a fresh odd negation must invert its exact binding")
	}
	for limit := range QueryBudget - fresh.remaining {
		cut := NewSearchBudget(limit)
		if _, known := fixed.HoldsWithin(condition, cut); known || !cut.Exhausted() {
			t.Fatal("interrupted negation cannot establish bound truth")
		}
	}
}

func TestNilFoldAllowanceKeepsBoxingOpaque(t *testing.T) {
	pkg := ssaflowtest.BuildPackage(t, "assumptions", `package assumptions
type pointer *int
func converted() pointer { return pointer((*int)(nil)) }
func boxed() any { return (*int)(nil) }
func mixed(flag bool) *int { var p *int; if flag { p = new(int) }; return p }
`)
	for _, name := range []string{"converted", "boxed", "mixed"} {
		value := InstructionsOf[*ssa.Return](pkg.Func(name))[0].Results[0]
		fresh := NewSearchBudget(QueryBudget)
		if got := DefinitelyNilWithin(value, fresh); got != (name == "converted") || fresh.Exhausted() {
			t.Fatal("nil-fold allowance must preserve boxing and phi policy")
		}
		for limit := range QueryBudget - fresh.remaining {
			cut := NewSearchBudget(limit)
			if DefinitelyNilWithin(value, cut) || !cut.Exhausted() {
				t.Fatal("an interrupted nil fold cannot supply definite nilness")
			}
		}
	}
}

func TestAssumedSuccessorAllowance(t *testing.T) {
	pkg := ssaflowtest.BuildPackage(t, "assumptions", `package assumptions
type owner struct { field *int }
type item struct{}
func direct(p *int) { if p != nil { println(1) } }
func field(p *owner) { if p.field != nil { println(1) } }
func foreign(p, q *owner) { if q.field != nil { println(1) } }
func asserted(p any) { if _, ok := p.(*item); ok { println(1) } }
func unmatched(p any) { if _, ok := p.(int); ok { println(1) } }
`)
	for _, name := range []string{"direct", "field", "foreign", "asserted", "unmatched"} {
		function := pkg.Func(name)
		branch := InstructionsOf[*ssa.If](function)[0]
		var concrete types.Type
		if name == "asserted" || name == "unmatched" {
			concrete = types.NewPointer(pkg.Pkg.Scope().Lookup("item").Type())
		}
		fresh := NewSearchBudget(QueryBudget)
		got := assumedSuccessorsWithin(branch.Block().Succs, branch.Block(), function.Params[0], concrete, fresh)
		want := branch.Block().Succs[:1]
		if name == "foreign" || name == "unmatched" {
			want = branch.Block().Succs
		}
		if !slices.Equal(got, want) || fresh.Exhausted() {
			t.Fatal("fresh assumption must require exact root/field/assertion identity")
		}
		for limit := range QueryBudget - fresh.remaining {
			cut := NewSearchBudget(limit)
			if got := assumedSuccessorsWithin(branch.Block().Succs, branch.Block(), function.Params[0], concrete, cut); got != nil || !cut.Exhausted() {
				t.Fatal("an interrupted assumption cannot supply pruned successors")
			}
		}
	}
}

func TestSuccessorAssumptionConsumersRejectCutoff(t *testing.T) {
	pkg := ssaflowtest.BuildPackage(t, "assumptions", `package assumptions
func bound(flag bool) { if flag { println(1) } }
func assumed(p *int) { if p != nil { println(1) } }
`)
	for _, name := range []string{"bound", "assumed"} {
		t.Run(name, func(t *testing.T) {
			function := pkg.Func(name)
			branch := InstructionsOf[*ssa.If](function)[0]
			policy := SuccessorPolicy{Feasible: func(block, _ *ssa.BasicBlock) []*ssa.BasicBlock { return block.Succs }}
			if name == "bound" {
				policy.Constants = FixedValues{function.Params[0]: OutcomeTrue}
			} else {
				policy.NonNil = function.Params[0]
			}
			cut := NewSearchBudget(0)
			if got := policy.successorsWithin(branch.Block(), nil, cut); got != nil || !cut.Exhausted() {
				t.Fatal("successor policy cannot consume uncharged bound/assumption evidence")
			}
			if got := policy.successorsWithin(branch.Block(), nil, NewSearchBudget(QueryBudget)); !slices.Equal(got, policy.Successors(branch.Block(), nil)) {
				t.Fatal("fresh successor policy must preserve default filtering")
			}
		})
	}
}

func TestAssumptionFlowCutoffClearsWitness(t *testing.T) {
	pkg := ssaflowtest.BuildPackage(t, "assumptions", `package assumptions
func marker() {}
func cleanup() {}
func bound(flag bool) { marker(); if flag { cleanup() } }
func assumed(p *int) { marker(); if p != nil { cleanup() } }
`)
	for _, name := range []string{"bound", "assumed"} {
		function := pkg.Func(name)
		flow := ObligationFlow{
			Start: InstructionsOf[*ssa.Call](function)[0], Budget: NewSearchBudget(QueryBudget),
			Successors: func(block, _ *ssa.BasicBlock) []*ssa.BasicBlock { return block.Succs },
			Instruction: func(instruction ssa.Instruction) ObligationAction {
				if call, ok := instruction.(*ssa.Call); ok && call.Common().StaticCallee() == pkg.Func("cleanup") {
					return ObligationExact
				}
				return ObligationNone
			},
		}
		if name == "bound" {
			flow.Constants = FixedValues{function.Params[0]: OutcomeTrue}
		} else {
			flow.NonNil = function.Params[0]
		}
		if got, witness := EvaluateObligationWitness(flow); got != ObligationHonored || witness != nil || flow.Budget.Exhausted() {
			t.Fatal("fresh assumption must preserve exact return coverage")
		}
		used := QueryBudget - flow.Budget.remaining
		for limit := range used {
			flow.Budget = NewSearchBudget(limit)
			if got, witness := EvaluateObligationWitness(flow); got != ObligationUncertain || witness != nil || !flow.Budget.Exhausted() {
				t.Fatalf("%s limit %d retained incomplete coverage or a witness", name, limit)
			}
		}
	}
}
