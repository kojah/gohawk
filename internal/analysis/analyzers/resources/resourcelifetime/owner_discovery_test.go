package resourcelifetime

import (
	"slices"
	"testing"

	proofs "github.com/kojah/gohawk/internal/engine/proof"
	"github.com/kojah/gohawk/internal/engine/ssaflow"
	"github.com/kojah/gohawk/internal/engine/ssaflow/ssaflowtest"
	"golang.org/x/tools/go/ssa"
)

func TestResourceOwnerDiscoveryAllowance(t *testing.T) {
	pkg := ssaflowtest.BuildPackage(t, "ownerdiscovery", `package ownerdiscovery
type resource struct { n int }
type holder struct { value *resource }
func local(p, other *resource, dest *holder) { h := &holder{p}; h.value = p }
func foreign(p, other *resource, dest *holder) { dest.value = p }
func unrelated(p, other *resource, dest *holder) { h := &holder{other}; h.value = other }
func destination() **resource
func opaque(p, other *resource, dest *holder) { *destination() = p }
`)
	for _, test := range []struct {
		name  string
		count int
	}{{"local", 1}, {"foreign", 0}, {"unrelated", 0}, {"opaque", 1}} {
		t.Run(test.name, func(t *testing.T) {
			fn := pkg.Func(test.name)
			baseline := proveLocalResourceOwnersWithin(fn, fn.Params[0], nil)
			if !baseline.Proven() || len(baseline.Owners) != test.count {
				t.Fatalf("default owner census = %+v; SSA:\n%s", baseline, carriedSSA(t, fn))
			}
			for limit := 0; limit <= proofs.SummaryBudget; limit++ {
				budget := proofs.NewSearchBudget(limit)
				got := proveLocalResourceOwnersWithin(fn, fn.Params[0], budget)
				if resourceFlowExhausted(budget) || limit == 0 {
					if got.Proven() || got.Reason != proofs.EvidenceBudgetExhausted || len(got.Owners) != 0 || len(got.Stores) != 0 {
						t.Fatalf("interrupted census = %+v", got)
					}
					continue
				}
				if !got.Proven() || !slices.Equal(got.Owners, baseline.Owners) {
					t.Fatalf("complete census = %+v, want %+v", got, baseline)
				}
				query := aggregateEscapeAnalysis(fn)
				query.stores = got.Stores
				query.pool = proofs.NewSearchBudget(0)
				for _, store := range ssaflow.InstructionsOf[*ssa.Store](fn) {
					if proof := query.resourceStorage(store); proof != got.Stores[store] || resourceFlowExhausted(query.pool) {
						t.Fatalf("discovered storage disposition repeated work: %+v", proof)
					}
				}
				return
			}
			t.Fatal("owner census never completed")
		})
	}
}

func TestResourceOwnerDiscoveryChildCutoff(t *testing.T) {
	pkg := ownerDiscoveryFixture(t)
	fn := pkg.Func("leak")
	call := ssaflow.InstructionsOf[*ssa.Call](fn)[0]
	query := &resourceAnalysis{function: fn, resource: call}
	pool := proofs.NewSearchBudget(resourcePoolBudget)
	got := query.discoverResourceOwnersWithin(pool.Within(2))
	if got.Proven() || got.Reason != proofs.EvidenceBudgetExhausted || len(got.Owners) != 0 ||
		len(got.Stores) != 0 || resourceFlowExhausted(pool) || len(query.owners) != 0 || len(query.stores) != 0 {
		t.Fatalf("owner census child cutoff = %+v, parent exhausted %v", got, resourceFlowExhausted(pool))
	}
	fresh := query.discoverResourceOwnersWithin(pool.Within(releaseSearchBudget))
	if !fresh.Proven() || len(query.owners) != 1 || len(query.stores) == 0 {
		t.Fatalf("fresh discovery failed to commit complete inputs: %+v", fresh)
	}
	query.pool = proofs.NewSearchBudget(0)
	for _, store := range ssaflow.InstructionsOf[*ssa.Store](fn) {
		if proof := query.resourceStorage(store); proof != fresh.Stores[store] || resourceFlowExhausted(query.pool) {
			t.Fatalf("committed storage disposition repeated work: %+v", proof)
		}
	}
}

func TestResourceOwnerDiscoveryFlow(t *testing.T) {
	pkg := ownerDiscoveryFixture(t)
	evidence, _ := resourceSummaries.Provider(nil).LifecycleEvidence("resourcelifetime", "resourcelifetime/missing-release")
	for _, name := range []string{"leak", "released"} {
		t.Run(name, func(t *testing.T) {
			fn := pkg.Func(name)
			call := ssaflow.InstructionsOf[*ssa.Call](fn)[0]
			got := evaluateResourceFlow(nil, evidence, call, call, resourceContract{cleanup: []string{"Close"}})
			if name == "leak" {
				if got.state != proofs.EvidenceProven || got.leak == nil {
					t.Fatalf("undisposed resource lost leak witness = %+v", got)
				}
			} else if got.state != proofs.EvidenceDisproven || got.leak != nil {
				t.Fatalf("released resource reported = %+v", got)
			}
		})
	}
}

func ownerDiscoveryFixture(t *testing.T) *ssa.Package {
	t.Helper()
	return ssaflowtest.BuildPackage(t, "ownerdiscoveryflow", `package ownerdiscoveryflow
 type resource struct { n int }
 func (*resource) Close() {}
 type holder struct { value *resource }
 func acquire() *resource { return &resource{} }
 func leak() { p := acquire(); _ = &holder{p} }
 func released() { p := acquire(); _ = &holder{p}; p.Close() }
 `)
}
