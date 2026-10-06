package path

import (
	"go/types"
	"slices"
	"testing"

	proofs "github.com/kojah/gohawk/internal/engine/proof"
	ssaflow "github.com/kojah/gohawk/internal/engine/ssaflow"
	ssacall "github.com/kojah/gohawk/internal/engine/ssaflow/calls"
	"github.com/kojah/gohawk/internal/engine/ssaflow/ssaflowtest"
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
		outcome ssacall.Outcome
		holds   bool
	}{
		{"direct", ssacall.OutcomeTrue, true},
		// SSA swaps edges for if !flag; its branch condition is still flag.
		{"negated", ssacall.OutcomeTrue, true},
		{"nilCheck", ssacall.OutcomeNonNil, true},
		{"reversed", ssacall.OutcomeNonNil, true},
	} {
		function := pkg.Func(test.name)
		branch := ssaflow.InstructionsOf[*ssa.If](function)[0]
		fixed := ssacall.FixedValues{function.Params[0]: test.outcome}
		fresh := proofs.NewSearchBudget(proofs.QueryBudget)
		holds, known := fixed.HoldsWithin(branch.Cond, fresh)
		if !known || holds != test.holds || fresh.Exhausted() {
			t.Fatalf("%s condition %s: holds=%t known=%t exhausted=%t", test.name, branch.Cond, holds, known, fresh.Exhausted())
		}
		for limit := range proofs.QueryBudget - fresh.Remaining() {
			cut := proofs.NewSearchBudget(limit)
			if _, known := fixed.HoldsWithin(branch.Cond, cut); known || !cut.Exhausted() {
				t.Fatal("an interrupted condition cannot decide a branch")
			}
		}
		cut := proofs.NewSearchBudget(0)
		if got := fixed.NarrowWithin(branch.Block().Succs, branch.Block(), cut); !slices.Equal(got, branch.Block().Succs) || !cut.Exhausted() {
			t.Fatal("interrupted narrowing must keep the primitive's edges unpruned")
		}
		got := fixed.NarrowWithin(branch.Block().Succs, branch.Block(), proofs.NewSearchBudget(proofs.QueryBudget))
		if !slices.Equal(got, fixed.Narrow(branch.Block().Succs, branch.Block())) {
			t.Fatal("fresh bound filtering must preserve default edges")
		}
	}
	odd := pkg.Func("odd")
	condition := ssaflow.InstructionsOf[*ssa.Return](odd)[0].Results[0]
	if _, ok := condition.(*ssa.UnOp); !ok {
		t.Fatal("expected an actual returned negation")
	}
	fixed := ssacall.FixedValues{odd.Params[0]: ssacall.OutcomeTrue}
	fresh := proofs.NewSearchBudget(proofs.QueryBudget)
	if holds, known := fixed.HoldsWithin(condition, fresh); holds || !known || fresh.Exhausted() {
		t.Fatal("a fresh odd negation must invert its exact binding")
	}
	for limit := range proofs.QueryBudget - fresh.Remaining() {
		cut := proofs.NewSearchBudget(limit)
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
		value := ssaflow.InstructionsOf[*ssa.Return](pkg.Func(name))[0].Results[0]
		fresh := proofs.NewSearchBudget(proofs.QueryBudget)
		if got := ssaflow.DefinitelyNilWithin(value, fresh); got != (name == "converted") || fresh.Exhausted() {
			t.Fatal("nil-fold allowance must preserve boxing and phi policy")
		}
		for limit := range proofs.QueryBudget - fresh.Remaining() {
			cut := proofs.NewSearchBudget(limit)
			if ssaflow.DefinitelyNilWithin(value, cut) || !cut.Exhausted() {
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
		branch := ssaflow.InstructionsOf[*ssa.If](function)[0]
		var concrete types.Type
		if name == "asserted" || name == "unmatched" {
			concrete = types.NewPointer(pkg.Pkg.Scope().Lookup("item").Type())
		}
		fresh := proofs.NewSearchBudget(proofs.QueryBudget)
		got := assumedSuccessorsWithin(branch.Block().Succs, branch.Block(), function.Params[0], concrete, fresh)
		want := branch.Block().Succs[:1]
		if name == "foreign" || name == "unmatched" {
			want = branch.Block().Succs
		}
		if !slices.Equal(got, want) || fresh.Exhausted() {
			t.Fatal("fresh assumption must require exact root/field/assertion identity")
		}
		for limit := range proofs.QueryBudget - fresh.Remaining() {
			cut := proofs.NewSearchBudget(limit)
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
			branch := ssaflow.InstructionsOf[*ssa.If](function)[0]
			policy := SuccessorPolicy{Feasible: func(block, _ *ssa.BasicBlock) []*ssa.BasicBlock { return block.Succs }}
			if name == "bound" {
				policy.Constants = ssacall.FixedValues{function.Params[0]: ssacall.OutcomeTrue}
			} else {
				policy.NonNil = function.Params[0]
			}
			cut := proofs.NewSearchBudget(0)
			if got := policy.SuccessorsWithin(branch.Block(), nil, cut); got != nil || !cut.Exhausted() {
				t.Fatal("successor policy cannot consume uncharged bound/assumption evidence")
			}
			freshBudget := proofs.NewSearchBudget(proofs.QueryBudget)
			if got := policy.SuccessorsWithin(branch.Block(), nil, freshBudget); !slices.Equal(got, policy.Successors(branch.Block(), nil)) {
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
			Start: ssaflow.InstructionsOf[*ssa.Call](function)[0], Budget: proofs.NewSearchBudget(proofs.QueryBudget),
			Successors: func(block, _ *ssa.BasicBlock) []*ssa.BasicBlock { return block.Succs },
			Instruction: func(instruction ssa.Instruction) ObligationAction {
				if call, ok := instruction.(*ssa.Call); ok && call.Common().StaticCallee() == pkg.Func("cleanup") {
					return ObligationExact
				}
				return ObligationNone
			},
		}
		if name == "bound" {
			flow.Constants = ssacall.FixedValues{function.Params[0]: ssacall.OutcomeTrue}
		} else {
			flow.NonNil = function.Params[0]
		}
		if got, witness := EvaluateObligationWitness(flow); got != ObligationHonored || witness != nil || flow.Budget.Exhausted() {
			t.Fatal("fresh assumption must preserve exact return coverage")
		}
		used := proofs.QueryBudget - flow.Budget.Remaining()
		for limit := range used {
			flow.Budget = proofs.NewSearchBudget(limit)
			if got, witness := EvaluateObligationWitness(flow); got != ObligationUncertain || witness != nil || !flow.Budget.Exhausted() {
				t.Fatalf("%s limit %d retained incomplete coverage or a witness", name, limit)
			}
		}
	}
}
