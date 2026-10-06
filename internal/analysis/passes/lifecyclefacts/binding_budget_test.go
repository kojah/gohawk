package lifecyclefacts

import (
	"fmt"
	"strings"
	"testing"

	proofs "github.com/kojah/gohawk/internal/engine/proof"
	"github.com/kojah/gohawk/internal/engine/ssaflow"
	"github.com/kojah/gohawk/internal/engine/ssaflow/ssaflowtest"
	"golang.org/x/tools/go/ssa"
)

func TestReturnedViewBindingAllowance(t *testing.T) {
	pkg := ssaflowtest.BuildPackage(t, "viewbinding", `package viewbinding
type holder struct { value *int }
func view(*int) *holder
func aggregate(*holder) *holder
func direct(p, other *int, choose bool) *holder { return view(p) }
func unrelated(p, other *int, choose bool) *holder { return view(other) }
func ambiguous(p, other *int, choose bool) *holder {
 x := p; if choose { x = other }; return view(x)
}
func contained(p, other *int, choose bool) *holder { return aggregate(&holder{p}) }
`)
	fact := Fact{Must: MustClaims{ReturnedView: 1}}
	for _, test := range []struct {
		name string
		want bool
	}{{"direct", true}, {"unrelated", false}, {"ambiguous", false}, {"contained", true}} {
		t.Run(test.name, func(t *testing.T) {
			fn := pkg.Func(test.name)
			call := ssaflow.InstructionsOf[*ssa.Call](fn)[0]
			if got := fact.ReturnsView(call, fn.Params[0]); got != test.want {
				t.Fatalf("default binding = %v, want %v", got, test.want)
			}
			for limit := 0; limit <= proofs.SummaryBudget; limit++ {
				pool := proofs.NewSearchBudget(limit)
				budget := pool.Within(proofs.SummaryBudget)
				got := fact.ProveReturnsViewWithin(call, fn.Params[0], budget)
				if budget.Exhausted() {
					if got.State != proofs.EvidenceUnknown || got.Reason != proofs.EvidenceBudgetExhausted {
						t.Fatalf("allowance %d retained interrupted binding: %+v", limit, got)
					}
					continue
				}
				if got.State == proofs.EvidenceUnknown || got.Proven() != test.want {
					t.Fatalf("complete binding at allowance %d: %+v", limit, got)
				}
				return
			}
			t.Fatal("binding never completed")
		})
	}
}

func TestReturnedViewBindingPreservesStorageCutoff(t *testing.T) {
	var source strings.Builder
	source.WriteString("package viewcap\ntype a *int; type b *int\nfunc view(a) *int\nfunc long(p a) *int {\n")
	previous := "p"
	for index := range proofs.QueryBudget + 10 {
		target := "b"
		if index%2 != 0 {
			target = "a"
		}
		name := fmt.Sprintf("x%d", index)
		fmt.Fprintf(&source, "%s := %s(%s)\n", name, target, previous)
		previous = name
	}
	fmt.Fprintf(&source, "return view(%s)\n}\n", previous)
	pkg := ssaflowtest.BuildPackage(t, "viewcap", source.String())
	fn := pkg.Func("long")
	if count := len(ssaflow.InstructionsOf[*ssa.ChangeType](fn)); count <= proofs.QueryBudget {
		t.Fatalf("fixture has only %d SSA conversions", count)
	}
	call := ssaflow.InstructionsOf[*ssa.Call](fn)[0]
	fact := Fact{Must: MustClaims{ReturnedView: 1}}
	budget := proofs.NewSearchBudget(10 * proofs.SummaryBudget)
	got := fact.ProveReturnsViewWithin(call, fn.Params[0], budget)
	if got.State != proofs.EvidenceUnknown || got.Reason != proofs.EvidenceBudgetExhausted || budget.Exhausted() {
		t.Fatalf("local storage cutoff lost availability: %+v, parent exhausted %v", got, budget.Exhausted())
	}
}
