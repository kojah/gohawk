package path

import (
	"slices"
	"strings"
	"testing"

	proofs "github.com/kojah/gohawk/internal/engine/proof"
	ssaflow "github.com/kojah/gohawk/internal/engine/ssaflow"
	"github.com/kojah/gohawk/internal/engine/ssaflow/ssaflowtest"
	"golang.org/x/tools/go/ssa"
)

func TestBranchValueAllowance(t *testing.T) {
	pkg := ssaflowtest.BuildPackage(t, "literal", `package literal
func subject(a, b bool) bool { condition := a && b; if condition { return true }; return false }
`)
	phi := ssaflow.InstructionsOf[*ssa.Phi](pkg.Func("subject"))[0]
	for predecessor, operand := range ssaflow.PhiIncoming(phi) {
		fresh := proofs.NewSearchBudget(proofs.QueryBudget)
		if got := BranchValueWithin(phi, phi.Block(), predecessor, fresh); got != operand || fresh.Exhausted() {
			t.Fatal("fresh selection must preserve the actual incoming operand")
		}
		for limit := range proofs.QueryBudget - fresh.Remaining() {
			cut := proofs.NewSearchBudget(limit)
			if got := BranchValueWithin(phi, phi.Block(), predecessor, cut); got != nil || !cut.Exhausted() {
				t.Fatal("interrupted phi selection must supply no operand")
			}
		}
	}
	if got := BranchValueWithin(phi, pkg.Func("subject").Blocks[0], phi.Block().Preds[0], proofs.NewSearchBudget(proofs.QueryBudget)); got != phi {
		t.Fatal("a historical phi must not select a different block's incoming path")
	}
}

func TestLiteralBranchAllowance(t *testing.T) {
	pkg := ssaflowtest.BuildPackage(t, "literal", `package literal
func agree(flag bool) bool { if flag { return true }; return true }
func mixed(flag bool) bool { if flag { return true }; return false }
func deferred() (result bool) { defer func() { result = false }(); return true }
func forwarded() bool { return agree(true) }
func pair() (int, bool) { return 1, true }
func count() int { return 3 }
func agreeing(flag bool) { if agree(flag) { println(1) } }
func differing(flag bool) { if mixed(flag) { println(1) } }
func mutation() { if deferred() { println(1) } }
func forwarding() { if forwarded() { println(1) } }
func paired() { _, ok := pair(); if ok { println(1) } }
func compared() { if count() > 0 { println(1) } }
`)
	for _, test := range []struct {
		name  string
		known bool
	}{
		{"agreeing", true},
		{"differing", false},
		{"mutation", false},
		{"forwarding", false},
		{"paired", true},
		{"compared", true},
	} {
		t.Run(test.name, func(t *testing.T) {
			branch := ssaflow.InstructionsOf[*ssa.If](pkg.Func(test.name))[0]
			fresh := proofs.NewSearchBudget(proofs.QueryBudget)
			holds, known := BranchBoolWithin(branch.Cond, branch.Block(), nil, fresh)
			if known != test.known || known && !holds || fresh.Exhausted() {
				t.Fatal("fresh evidence must preserve literal-only return policy")
			}
			for limit := range proofs.QueryBudget - fresh.Remaining() {
				cut := proofs.NewSearchBudget(limit)
				if _, known := BranchBoolWithin(branch.Cond, branch.Block(), nil, cut); known || !cut.Exhausted() {
					t.Fatal("interrupted helper/comparison evidence cannot decide a branch")
				}
			}
			freshBudget := proofs.NewSearchBudget(proofs.QueryBudget)
			if got := FeasibleSuccessorsWithin(branch.Block(), nil, freshBudget); !slices.Equal(got, FeasibleSuccessors(branch.Block(), nil)) {
				t.Fatal("fresh feasibility must retain default successors")
			}
			cut := proofs.NewSearchBudget(0)
			if got := FeasibleSuccessorsWithin(branch.Block(), nil, cut); !slices.Equal(got, branch.Block().Succs) || !cut.Exhausted() {
				t.Fatal("cutoff must retain all successors without pruning")
			}
		})
	}
}

func TestLiteralHelperCapRemainsIndependent(t *testing.T) {
	source := `package literal
func marker() {}
func admitted() bool { ` + strings.Repeat("marker();", 127) + `return true }
func oversized() bool { ` + strings.Repeat("marker();", 128) + `return true }
func first() { if admitted() { marker() } }
func second() { if oversized() { marker() } }
`
	pkg := ssaflowtest.BuildPackage(t, "literal", source)
	for _, test := range []struct {
		name  string
		known bool
	}{{"first", true}, {"second", false}} {
		branch := ssaflow.InstructionsOf[*ssa.If](pkg.Func(test.name))[0]
		budget := proofs.NewSearchBudget(proofs.QueryBudget)
		_, known := BranchBoolWithin(branch.Cond, branch.Block(), nil, budget)
		if known != test.known || budget.Exhausted() {
			t.Fatal("the independent helper cap must not exhaust the caller allowance")
		}
	}
}

func TestLiteralFeasibilityCutoffKeepsFlowUnknown(t *testing.T) {
	source := `package literal
func marker() {}
func cleanup() {}
func helper() bool { ` + strings.Repeat("marker();", 64) + `return true }
func subject() { if helper() { cleanup() } }
`
	pkg := ssaflowtest.BuildPackage(t, "literal", source)
	function := pkg.Func("subject")
	budget := proofs.NewSearchBudget(proofs.QueryBudget)
	flow := ObligationFlow{Budget: budget, Instruction: func(instruction ssa.Instruction) ObligationAction {
		if _, ok := instruction.(*ssa.If); ok {
			// The helper is within its own cap but exceeds the caller's
			// remaining allowance. No classifier answer is interrupted here.
			for budget.Remaining() > 32 {
				budget.Spend()
			}
		}
		if call, ok := instruction.(*ssa.Call); ok && call.Common().StaticCallee() == pkg.Func("cleanup") {
			return ObligationExact
		}
		return ObligationNone
	}}
	if got := EvaluateObligationFromEntry(function, flow); got != ObligationUncertain || !budget.Exhausted() {
		t.Fatal("interrupted helper feasibility cannot prove honored coverage")
	}
	flow.Budget = proofs.NewSearchBudget(proofs.QueryBudget)
	flow.Instruction = func(instruction ssa.Instruction) ObligationAction {
		if call, ok := instruction.(*ssa.Call); ok && call.Common().StaticCallee() == pkg.Func("cleanup") {
			return ObligationExact
		}
		return ObligationNone
	}
	if got := EvaluateObligationFromEntry(function, flow); got != ObligationHonored || flow.Budget.Exhausted() {
		t.Fatal("fresh literal feasibility must retain exact coverage")
	}
}
