package resourcemodel

import (
	"slices"
	"testing"

	"github.com/kojah/gohawk/internal/ssaflow"
	"github.com/kojah/gohawk/internal/ssaflow/ssaflowtest"
	"golang.org/x/tools/go/ssa"
)

func TestRelationAvailability(t *testing.T) {
	pkg := ssaflowtest.BuildPackage(t, "relationallowance", `package relationallowance
type resource struct { n int }
type holder struct { value *resource }
func observe(*holder) {}
func exact(p, other *resource) { observe(&holder{p}) }
func unrelated(p, other *resource) { observe(&holder{other}) }
`)
	for _, name := range []string{"exact", "unrelated"} {
		t.Run(name, func(t *testing.T) {
			fn := pkg.Func(name)
			call := ssaflow.InstructionsOf[*ssa.Call](fn)[0]
			owner := call.Common().Args[0]
			for limit := 0; limit <= ssaflow.QueryBudget; limit++ {
				budget := ssaflow.NewSearchBudget(limit)
				proof := ProveRelation(owner, fn.Params[0], call, budget)
				if budget.Exhausted() || budget.PoolExhausted() {
					if proof.Proven() || proof.Reason != ssaflow.EvidenceBudgetExhausted {
						t.Fatalf("interrupted relation at %d = %+v", limit, proof)
					}
					continue
				}
				if proof.Proven() != (name == "exact") || proof.Proven() && !slices.Equal(proof.Relation.Path(), []string{"field:0"}) {
					t.Fatalf("complete relation = %+v", proof)
				}
				return
			}
			t.Fatal("relation never completed")
		})
	}
}
