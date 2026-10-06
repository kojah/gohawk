package lifecycle

import (
	"testing"

	proofs "github.com/kojah/gohawk/internal/engine/proof"
	"github.com/kojah/gohawk/internal/engine/ssaflow"
	"github.com/kojah/gohawk/internal/engine/ssaflow/ssaflowtest"
	"golang.org/x/tools/go/ssa"
)

func TestReturnedOwnershipAllowance(t *testing.T) {
	pkg := ssaflowtest.BuildPackage(t, "ownerbudget", `package ownerbudget
type owner struct { value *int; next *owner }
func direct(p *int) *int { return p }
func stored(p *int) *owner { o := &owner{value:p}; o.next=o; return o }
func copied(p *int) owner { return owner{value:p} }
func initialize(o *owner, p *int) { o.value=p }
func delegated(p *int) *owner { o:=new(owner); initialize(o,p); return o }
func maybe(p *int, fail bool) *owner { if fail { return nil }; return &owner{value:p} }
func allowed(p *int, fail bool) *owner { return maybe(p,fail) }
func mixed(p *int, fail bool) *owner { if fail { return new(owner) }; return &owner{value:p} }
func uncovered(p *int, fail bool) *owner { return mixed(p,fail) }
func callback(p *int) func() { return func(){ println(p) } }
func selected(p *int, flag bool) *owner { o:=new(owner); if flag {o=&owner{value:p}}; return o }
func unrelated(p *int) *owner { return new(owner) }
`)
	for _, test := range []struct {
		name string
		owns bool
	}{
		{"direct", true},
		{"stored", true},
		{"copied", true},
		{"delegated", true},
		{"allowed", true},
		{"uncovered", false},
		{"callback", true},
		{"selected", true},
		{"unrelated", false},
	} {
		t.Run(test.name, func(t *testing.T) {
			fn := pkg.Func(test.name)
			returned := ssaflow.InstructionsOf[*ssa.Return](fn)[0]
			for _, block := range fn.Blocks {
				for _, instruction := range block.Instrs {
					t.Log(instruction.String())
				}
			}
			completed := false
			for limit := 0; limit <= proofs.SummaryBudget; limit++ {
				pool := proofs.NewSearchBudget(limit)
				budget := pool.Within(proofs.SummaryBudget)
				got := ProveReturnedOwnershipWithin(returned, fn.Params[0], nil, budget)
				if budget.Exhausted() || budget.PoolExhausted() {
					if got.State != proofs.EvidenceUnknown || got.Reason != proofs.EvidenceBudgetExhausted {
						t.Fatalf("cutoff %d admitted %+v", limit, got)
					}
					continue
				}
				if limit == 0 || got.Proven() != test.owns || got.State == proofs.EvidenceUnknown {
					t.Fatalf("completed query = %+v, want ownership %t", got, test.owns)
				}
				completed = true
				break
			}
			if !completed {
				t.Fatal("ownership query never completed")
			}
		})
	}
}

func TestReturnedOwnershipSummaryCallbackCutoff(t *testing.T) {
	pkg := ssaflowtest.BuildPackage(t, "ownercallback", `package ownercallback
type owner struct{ p *int }
func opaque(*int) *owner
func returned(p *int) *owner { return opaque(p) }
`)
	fn := pkg.Func("returned")
	returned := ssaflow.InstructionsOf[*ssa.Return](fn)[0]
	pool := proofs.NewSearchBudget(proofs.SummaryBudget)
	budget := pool.Within(proofs.SummaryBudget)
	sibling := pool.Within(proofs.SummaryBudget)
	called := false
	hook := func(*ssa.Function, int) bool {
		called = true
		for sibling.Spend() {
		}
		return true
	}
	got := ProveReturnedOwnershipWithin(returned, fn.Params[0], hook, budget)
	if !called || got.State != proofs.EvidenceUnknown || got.Reason != proofs.EvidenceBudgetExhausted {
		t.Fatalf("callback cutoff = %+v, called %t", got, called)
	}
	fresh := ProveReturnedOwnershipWithin(returned, fn.Params[0], func(*ssa.Function, int) bool { return true }, proofs.NewSearchBudget(proofs.SummaryBudget))
	if !fresh.Proven() {
		t.Fatal("fresh query failed to recover summary ownership")
	}
}
