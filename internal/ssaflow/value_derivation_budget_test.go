package ssaflow_test

import (
	"testing"

	"github.com/kojah/gohawk/internal/ssaflow"
	"github.com/kojah/gohawk/internal/ssaflow/ssaflowtest"
	"golang.org/x/tools/go/ssa"
)

func TestDerivationAllowance(t *testing.T) {
	pkg := ssaflowtest.BuildPackage(t, "derivationbudget", `package derivationbudget
func opaque(*int) *int
func direct(p *int) *int { return p }
func called(p *int) *int { return opaque(p) }
func stored(p *int) *int { cell := new(*int); *cell = p; return *cell }
type inner struct { p *int }
type aggregate struct { child inner }
func whole(p aggregate) *int { copy := p; return copy.child.p }
func replaced(p aggregate) *int { copy := p; copy.child.p = new(int); return copy.child.p }
func cycle(p *int, flag bool) *int { for flag { p = opaque(p) }; return p }
`)
	same := func(left, right ssa.Value) bool { return left == right }
	for _, name := range []string{"direct", "called", "stored", "whole", "cycle"} {
		t.Run(name, func(t *testing.T) {
			fn := pkg.Func(name)
			value := ssaflow.InstructionsOf[*ssa.Return](fn)[0].Results[0]
			for _, block := range fn.Blocks {
				for _, instruction := range block.Instrs {
					t.Log(instruction.String())
				}
			}
			completed := false
			for limit := 0; limit <= ssaflow.SummaryBudget; limit++ {
				pool := ssaflow.NewSearchBudget(limit)
				budget := pool.Within(ssaflow.SummaryBudget)
				got := ssaflow.DerivesFromWithin(value, fn.Params[0], same, budget)
				if limit == 0 && !budget.Exhausted() {
					t.Fatal("empty allowance did not stop derivation")
				}
				if budget.Exhausted() {
					if got {
						t.Fatalf("allowance %d admitted interrupted derivation", limit)
					}
					continue
				}
				if !got {
					t.Fatalf("completed derivation lost source at allowance %d", limit)
				}
				completed = true
				break
			}
			if !completed {
				t.Fatal("derivation never completed")
			}
		})
	}
	fn := pkg.Func("replaced")
	value := ssaflow.InstructionsOf[*ssa.Return](fn)[0].Results[0]
	if ssaflow.DerivesFromWithin(value, fn.Params[0], same, ssaflow.NewSearchBudget(ssaflow.SummaryBudget)) {
		t.Fatal("replaced nested field must not cross the whole-aggregate store")
	}
}

func TestDerivationRejectsCallbackCutoff(t *testing.T) {
	source := &ssa.Alloc{}
	budget := ssaflow.NewSearchBudget(1)
	matched := func(_, _ ssa.Value) bool { budget.Spend(); return true }
	if ssaflow.DerivesFromWithin(source, source, matched, budget) || !budget.Exhausted() {
		t.Fatal("callback exhaustion admitted positive derivation")
	}
	if !ssaflow.DerivesFromWithin(source, source, func(left, right ssa.Value) bool { return left == right }, ssaflow.NewSearchBudget(1)) {
		t.Fatal("fresh query did not recover direct identity")
	}
}

func TestDerivationRejectsSiblingPoolCutoff(t *testing.T) {
	source := &ssa.Alloc{}
	pool := ssaflow.NewSearchBudget(1)
	budget := pool.Within(10)
	sibling := pool.Within(10)
	matched := func(_, _ ssa.Value) bool { sibling.Spend(); return true }
	if ssaflow.DerivesFromWithin(source, source, matched, budget) || !budget.PoolExhausted() {
		t.Fatal("sibling exhaustion admitted positive derivation")
	}
}
