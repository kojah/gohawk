package ssaflow_test

import (
	"slices"
	"testing"

	proofs "github.com/kojah/gohawk/internal/proof"
	"github.com/kojah/gohawk/internal/ssaflow"
	"github.com/kojah/gohawk/internal/ssaflow/ssaflowtest"
	"golang.org/x/tools/go/ssa"
)

func TestStrictDominatingInstructions(t *testing.T) {
	pkg := ssaflowtest.BuildPackage(t, "dominators", `package dominators
 func mark(int){}
 func straight(){mark(1);mark(2);mark(3)}
 func diamond(flag bool){mark(1);if flag {mark(2)}else{mark(3)};mark(4)}
 func loop(flag bool){mark(1);for flag {mark(2);mark(3)};mark(4)}
 `)
	for _, name := range []string{"straight", "diamond", "loop"} {
		fn := pkg.Func(name)
		for pivot := range ssaflow.InstructionsWithin(fn, nil) {
			var want []ssa.Instruction
			for candidate := range ssaflow.InstructionsWithin(fn, nil) {
				if candidate != pivot && ssaflow.InstructionDominates(candidate, pivot) {
					want = append(want, candidate)
				}
			}
			got := slices.Collect(ssaflow.InstructionsStrictlyDominatingWithin(pivot, nil))
			if !slices.Equal(got, want) {
				t.Fatalf("%s %s: strict dominance differs", name, pivot)
			}
			checkDominatingAllowance(t, pivot, want)
		}
	}
	if got := slices.Collect(ssaflow.InstructionsStrictlyDominatingWithin(nil, nil)); len(got) != 0 {
		t.Fatal("nil pivot yielded instructions")
	}
}

func checkDominatingAllowance(t *testing.T, pivot ssa.Instruction, want []ssa.Instruction) {
	t.Helper()
	pool := proofs.NewSearchBudget(proofs.SummaryBudget)
	for limit := range 1000 {
		child := pool.Within(limit)
		got := slices.Collect(ssaflow.InstructionsStrictlyDominatingWithin(pivot, child))
		if !child.Exhausted() {
			if !slices.Equal(got, want) {
				t.Fatalf("completed census: %v want %v", got, want)
			}
			return
		}
		if pool.Exhausted() || len(got) > len(want) || !slices.Equal(got, want[:len(got)]) {
			t.Fatal("cutoff changed prefix or exhausted parent")
		}
		fresh := slices.Collect(ssaflow.InstructionsStrictlyDominatingWithin(pivot, pool.Within(proofs.SummaryBudget)))
		if !slices.Equal(fresh, want) {
			t.Fatal("fresh child did not recover")
		}
	}
	t.Fatal("census never completed")
}
