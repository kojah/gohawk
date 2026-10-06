package lifecycle

import (
	"testing"

	proofs "github.com/kojah/gohawk/internal/proof"
	"github.com/kojah/gohawk/internal/ssaflow"
	"github.com/kojah/gohawk/internal/ssaflow/ssaflowtest"
	"golang.org/x/tools/go/ssa"
)

func TestObservedContainmentAllowance(t *testing.T) {
	pkg := ssaflowtest.BuildPackage(t, "observedowner", `package observedowner
type holder struct { value *int }
func observe(any)
func direct(p, other *int) { observe(p) }
func nested(p, other *int) { observe(&holder{p}) }
func unrelated(p, other *int) { observe(&holder{other}) }
func closure(p, other *int) { observe(func(){ println(p) }) }
func fill(o *holder, p *int) { o.value = p }
func later(p, other *int) { o := &holder{}; observe(o); fill(o,p) }
`)
	for _, test := range []struct {
		name string
		want bool
	}{
		{"direct", true},
		{"nested", true},
		{"unrelated", false},
		{"closure", true},
		// The existing structural may-search includes later visible stores.
		// Only the graph fallback is observed; this cannot establish cleanup.
		{"later", true},
	} {
		t.Run(test.name, func(t *testing.T) {
			fn := pkg.Func(test.name)
			call := ssaflow.InstructionsOf[*ssa.Call](fn)[0]
			owner := call.Common().Args[0]
			for _, at := range []ssa.Instruction{call, nil} {
				baseline := ProveMayContainValueAtWithin(owner, fn.Params[0], at, nil).Proven()
				if baseline != test.want {
					t.Fatalf("default containment = %v, want %v", baseline, test.want)
				}
				completed := false
				for limit := 0; limit <= proofs.SummaryBudget; limit++ {
					pool := proofs.NewSearchBudget(limit)
					budget := pool.Within(proofs.SummaryBudget)
					got := ProveMayContainValueAtWithin(owner, fn.Params[0], at, budget)
					if budget.Exhausted() || budget.PoolExhausted() {
						if got.State != proofs.EvidenceUnknown || got.Reason != proofs.EvidenceBudgetExhausted {
							t.Fatalf("allowance %d retained interrupted containment: %+v", limit, got)
						}
						continue
					}
					if limit == 0 || got.State == proofs.EvidenceUnknown || got.Proven() != baseline {
						t.Fatalf("complete containment = %+v, want %v", got, baseline)
					}
					completed = true
					break
				}
				if !completed {
					t.Fatal("containment never completed")
				}
			}
		})
	}
}
