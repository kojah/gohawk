package ssaflow_test

import (
	"testing"

	proofs "github.com/kojah/gohawk/internal/engine/proof"
	"github.com/kojah/gohawk/internal/engine/ssaflow"
	"github.com/kojah/gohawk/internal/engine/ssaflow/ssaflowtest"
	"golang.org/x/tools/go/ssa"
)

// Derivation includes computations and opaque call operands. It supplies
// possible provenance, not an exact identity or ownership guarantee.
func TestDerivationBoundaries(t *testing.T) {
	pkg := ssaflowtest.BuildPackage(t, "derivation", `package derivation
func opaque(*int) *int
func called(p *int) *int { return opaque(p) }
func boxed(p *int) any { return p }
func arithmetic(n int) int { return n + 1 }
func unrelated(p *int) *int { return new(int) }
func stored(p *int) *int {
	x := new(*int)
	*x = p
	return *x
}
func cyclic(p *int, n int) *int {
	x := p
	for i := 0; i < n; i++ { if i%2 == 0 { x = new(int) } }
	return x
}
func unrelatedCycle(p *int, n int) *int {
	x := new(int)
	for i := 0; i < n; i++ { if i%2 == 0 { x = new(int) } }
	return x
}
`)
	for _, test := range []struct {
		name string
		want bool
	}{
		{"called", true},
		{"boxed", true},
		{"arithmetic", true},
		{"stored", true},
		{"unrelated", false},
		{"cyclic", true},
		{"unrelatedCycle", false},
	} {
		t.Run(test.name, func(t *testing.T) {
			function := pkg.Func(test.name)
			returns := ssaflow.InstructionsOf[*ssa.Return](function)
			if len(returns) != 1 || len(returns[0].Results) != 1 {
				t.Fatal("expected one return with one result")
			}
			got := ssaflow.DerivesFromWithin(returns[0].Results[0], function.Params[0], ssaflow.StructurallySame, nil)
			if got != test.want {
				t.Errorf("DerivesFromWithin() = %t, want %t", got, test.want)
			}
		})
	}
}

func TestDerivationIdentityBeforeOperands(t *testing.T) {
	same := func(left, right ssa.Value) bool { return left == right }
	source := &ssa.Alloc{}
	empty := &ssa.Phi{}
	cycle := &ssa.Phi{}
	cycle.Edges = []ssa.Value{cycle, source}
	unrelated := &ssa.Phi{}
	unrelated.Edges = []ssa.Value{unrelated}
	for _, test := range []struct {
		name   string
		value  ssa.Value
		source ssa.Value
		want   bool
	}{
		{"empty phi identity", empty, empty, true},
		{"cyclic phi identity", unrelated, unrelated, true},
		{"source beside cycle", cycle, source, true},
		{"cycle without source", unrelated, source, false},
		{"missing value", nil, source, false},
		{"missing source", source, nil, false},
	} {
		t.Run(test.name, func(t *testing.T) {
			if got := ssaflow.DerivesFromWithin(test.value, test.source, same, nil); got != test.want {
				t.Errorf("DerivesFromWithin() = %t, want %t", got, test.want)
			}
		})
	}
}

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
			for limit := 0; limit <= proofs.SummaryBudget; limit++ {
				pool := proofs.NewSearchBudget(limit)
				budget := pool.Within(proofs.SummaryBudget)
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
	if ssaflow.DerivesFromWithin(value, fn.Params[0], same, proofs.NewSearchBudget(proofs.SummaryBudget)) {
		t.Fatal("replaced nested field must not cross the whole-aggregate store")
	}
}

func TestDerivationRejectsCallbackCutoff(t *testing.T) {
	source := &ssa.Alloc{}
	budget := proofs.NewSearchBudget(1)
	matched := func(_, _ ssa.Value) bool { budget.Spend(); return true }
	if ssaflow.DerivesFromWithin(source, source, matched, budget) || !budget.Exhausted() {
		t.Fatal("callback exhaustion admitted positive derivation")
	}
	if !ssaflow.DerivesFromWithin(source, source, func(left, right ssa.Value) bool { return left == right }, proofs.NewSearchBudget(1)) {
		t.Fatal("fresh query did not recover direct identity")
	}
}

func TestDerivationRejectsSiblingPoolCutoff(t *testing.T) {
	source := &ssa.Alloc{}
	pool := proofs.NewSearchBudget(1)
	budget := pool.Within(10)
	sibling := pool.Within(10)
	matched := func(_, _ ssa.Value) bool { sibling.Spend(); return true }
	if ssaflow.DerivesFromWithin(source, source, matched, budget) || !budget.PoolExhausted() {
		t.Fatal("sibling exhaustion admitted positive derivation")
	}
}

func TestWholeWrittenCellDistinguishesSelectedUses(t *testing.T) {
	pkg := ssaflowtest.BuildPackage(t, "wholeuses", `package wholeuses
 type inner struct{p *int}
 type aggregate struct{items [2]inner}
 func opaque(*inner)
 func read(p aggregate)*int{copy:=p;return copy.items[0].p}
 func replaced(p aggregate)*int{copy:=p;copy.items[0].p=new(int);return copy.items[0].p}
 func escaped(p aggregate)*int{copy:=p;opaque(&copy.items[0]);return copy.items[0].p}
 `)
	for _, test := range []struct {
		name string
		want bool
	}{{"read", true}, {"replaced", false}, {"escaped", false}} {
		t.Run(test.name, func(t *testing.T) {
			fn := pkg.Func(test.name)
			cells := ssaflow.InstructionsOf[*ssa.Alloc](fn)
			if len(cells) == 0 {
				t.Fatal("SSA has no aggregate cell")
			}
			cell := cells[0]
			for limit := 0; limit <= proofs.SummaryBudget; limit++ {
				pool := proofs.NewSearchBudget(10 * proofs.SummaryBudget)
				child := pool.Within(limit)
				got := ssaflow.WholeWrittenCellWithin(cell, child)
				if child.Exhausted() {
					if got || pool.Exhausted() {
						t.Fatalf("cutoff %d admitted cell", limit)
					}
					if fresh := ssaflow.WholeWrittenCellWithin(cell, pool.Within(proofs.SummaryBudget)); fresh != test.want {
						t.Fatalf("fresh after %d=%v", limit, fresh)
					}
					continue
				}
				if got != test.want {
					t.Fatalf("completed allowance %d=%v want %v", limit, got, test.want)
				}
				return
			}
			t.Fatal("cell query never completed")
		})
	}
}
