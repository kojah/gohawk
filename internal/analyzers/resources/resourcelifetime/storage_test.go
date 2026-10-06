package resourcelifetime

import (
	"testing"

	proofs "github.com/kojah/gohawk/internal/proof"
	"github.com/kojah/gohawk/internal/ssaflow"
	"github.com/kojah/gohawk/internal/ssaflow/ssaflowtest"
	"golang.org/x/tools/go/ssa"
)

func TestResourceStorageAllowance(t *testing.T) {
	pkg := ssaflowtest.BuildPackage(t, "storebudget", `package storebudget
type resource struct{}
type holder struct { value *resource }
type nested struct { owner *holder }
func foreign(value, other *resource, dst *holder) { dst.value = value }
func local(value, other *resource, dst *holder) { own := &holder{}; own.value = value }
func unrelated(value, other *resource, dst *holder) { dst.value = other }
func contained(value, other *resource, dst *nested) { dst.owner = &holder{value} }
func indirect(value, other *resource, dst *holder) {
 entry := struct { destination **resource }{&dst.value}; *entry.destination = value
}
func replaced(value, other *resource, dst *holder) {
 own := &holder{}
 entry := struct { destination **resource }{&dst.value}
 entry.destination = &own.value; *entry.destination = value
}
func destination() **resource
func opaque(value, other *resource, dst *holder) {
 entry := struct { destination **resource }{destination()}; *entry.destination = value
}
`)
	for _, test := range []struct {
		name  string
		state proofs.EvidenceState
		owner bool
	}{
		{"foreign", proofs.EvidenceProven, true},
		{"local", proofs.EvidenceDisproven, true},
		{"unrelated", proofs.EvidenceDisproven, false},
		{"contained", proofs.EvidenceProven, true},
		{"indirect", proofs.EvidenceProven, true},
		{"replaced", proofs.EvidenceDisproven, true},
		{"opaque", proofs.EvidenceUnknown, true},
	} {
		t.Run(test.name, func(t *testing.T) {
			fn := pkg.Func(test.name)
			stores := ssaflow.InstructionsOf[*ssa.Store](fn)
			store := stores[len(stores)-1]
			baseline := proveResourceStorage(store, fn.Params[0], nil)
			if baseline.State != test.state || (baseline.Owner != nil) != test.owner {
				t.Fatalf("default storage = %+v, want %v/owner %v", baseline, test.state, test.owner)
			}
			for limit := 0; limit <= proofs.SummaryBudget; limit++ {
				pool := proofs.NewSearchBudget(limit)
				budget := pool.Within(proofs.SummaryBudget)
				got := proveResourceStorage(store, fn.Params[0], budget)
				if resourceFlowExhausted(budget) {
					if got.State != proofs.EvidenceUnknown || got.Reason != resourceReasonBudgetExhausted {
						t.Fatalf("allowance %d retained interrupted storage: %+v", limit, got)
					}
					continue
				}
				if got != baseline {
					t.Fatalf("complete storage = %+v, want %+v", got, baseline)
				}
				return
			}
			t.Fatal("storage never completed")
		})
	}
}

func TestResourceStorageCutoffRetryAndReuse(t *testing.T) {
	pkg := ssaflowtest.BuildPackage(t, "storereuse", `package storereuse
type resource struct{}
type holder struct { value *resource }
func foreign(value *resource, dst *holder) { dst.value = value }
`)
	fn := pkg.Func("foreign")
	store := ssaflow.InstructionsOf[*ssa.Store](fn)[0]
	provider := resourceSummaries.Provider(nil)
	evidence, _ := provider.LifecycleEvidence("resourcelifetime", "resourcelifetime/missing-release")
	query := &resourceAnalysis{
		function: fn, resource: fn.Params[0], summaries: provider, evidence: evidence,
		pool: proofs.NewSearchBudget(0),
	}
	if action, reason := query.releasesOrdinaryResource(store); action != actionUnknown || reason != resourceReasonBudgetExhausted {
		t.Fatalf("interrupted release = %v/%v", action, reason)
	}
	if len(query.stores) != 0 {
		t.Fatal("interrupted storage proof was memoized")
	}
	query.pool = proofs.NewSearchBudget(proofs.SummaryBudget)
	if action, reason := query.releasesOrdinaryResource(store); action != actionSettled || reason != resourceReasonSettled {
		t.Fatalf("fresh release = %v/%v", action, reason)
	}
	if len(query.stores) != 1 {
		t.Fatal("completed storage proof was not memoized")
	}
	// Reuse needs no second containment/destination query. An empty replacement
	// pool exposes any repeated work through its exhaustion marker.
	query.pool = proofs.NewSearchBudget(0)
	if got := query.resourceStorage(store); !got.Proven() || query.pool.Exhausted() {
		t.Fatalf("completed proof repeated work: %+v, exhausted %v", got, query.pool.Exhausted())
	}
}
