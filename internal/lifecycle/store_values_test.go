package lifecycle

import (
	"testing"

	proofs "github.com/kojah/gohawk/internal/proof"
	"github.com/kojah/gohawk/internal/ssaflow"
	"github.com/kojah/gohawk/internal/ssaflow/ssaflowtest"
	"golang.org/x/tools/go/ssa"
)

func storedOwnerFixture(t *testing.T) (*ssa.Function, ssa.Value) {
	t.Helper()
	pkg := ssaflowtest.BuildPackage(t, "storedowner", `package storedowner
type owner struct { direct *int; indexed [2]*int; slot **int; next *owner }
func aggregate(value, other *int) *owner {
	box := new(owner)
	box.direct = value
	box.indexed[1] = other
	box.slot = new(*int)
	*box.slot = value
	box.next = box
	return box
}
`)
	fn := pkg.Func("aggregate")
	for _, block := range fn.Blocks {
		for _, instruction := range block.Instrs {
			t.Log(instruction.String())
		}
	}
	return fn, ssaflow.InstructionsOf[*ssa.Return](fn)[0].Results[0]
}

func TestStoredValueTraversalAllowance(t *testing.T) {
	fn, root := storedOwnerFixture(t)
	var complete []ssa.Value
	for value := range StoredInto(root) {
		complete = append(complete, value)
	}
	counts := map[ssa.Value]int{}
	for _, value := range complete {
		counts[value]++
	}
	if len(complete) != 5 || counts[fn.Params[0]] != 2 || counts[fn.Params[1]] != 1 || counts[root] != 1 {
		t.Fatalf("field, index, loaded slot and cyclic owner contents = %v", counts)
	}
	finished := false
	for limit := 0; limit <= proofs.SummaryBudget; limit++ {
		pool := proofs.NewSearchBudget(limit)
		budget := pool.Within(proofs.SummaryBudget)
		got := map[ssa.Value]int{}
		for value := range StoredIntoWithin(root, budget) {
			got[value]++
		}
		if budget.Exhausted() {
			for value, count := range got {
				if count > counts[value] {
					t.Fatalf("cutoff invented stored value %v", value)
				}
			}
			continue
		}
		if limit == 0 || len(got) != len(counts) {
			t.Fatalf("complete traversal = %v, want %v", got, counts)
		}
		for value, count := range counts {
			if got[value] != count {
				t.Fatalf("complete traversal lost %v: %v", value, got)
			}
		}
		finished = true
		break
	}
	if !finished {
		t.Fatal("stored-value traversal never completed")
	}
}

func TestStoredValueTraversalEarlyStop(t *testing.T) {
	_, root := storedOwnerFixture(t)
	budget := proofs.NewSearchBudget(proofs.SummaryBudget)
	visits := 0
	for range StoredIntoWithin(root, budget) {
		visits++
		break
	}
	if visits != 1 || budget.Exhausted() {
		t.Fatal("early stopping must complete one yield without exhausting")
	}
	pool := proofs.NewSearchBudget(proofs.SummaryBudget)
	child := pool.Within(proofs.SummaryBudget)
	sibling := pool.Within(proofs.SummaryBudget)
	visits = 0
	for range StoredIntoWithin(root, child) {
		visits++
		for sibling.Spend() {
		}
	}
	if visits != 1 || !child.PoolExhausted() {
		t.Fatal("callback pool cutoff must stop further yields")
	}
}

func TestStoredValueTraversalOpaqueForms(t *testing.T) {
	pkg := ssaflowtest.BuildPackage(t, "storedopaque", `package storedopaque
type slot **int
func hidden(box **int, value *int) { *box = value }
func called(value *int) **int { box := new(*int); hidden(box, value); return box }
func converted(value *int) **int { box := new(*int); *slot(box) = value; return box }
func merged(value *int, flag bool) **int {
	box := new(*int)
	x := box
	if flag { x = new(*int) }
	*x = value
	return box
}
`)
	for _, name := range []string{"called", "converted", "merged"} {
		fn := pkg.Func(name)
		root := ssaflow.InstructionsOf[*ssa.Return](fn)[0].Results[0]
		budget := proofs.NewSearchBudget(proofs.SummaryBudget)
		for value := range StoredIntoWithin(root, budget) {
			t.Fatalf("%s crossed an opaque call, conversion or phi to %v", name, value)
		}
		if budget.Exhausted() {
			t.Fatalf("%s failed to complete the bounded query", name)
		}
	}
}
