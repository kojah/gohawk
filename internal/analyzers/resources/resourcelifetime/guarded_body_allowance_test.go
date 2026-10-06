package resourcelifetime

import (
	"testing"

	proofs "github.com/kojah/gohawk/internal/proof"
)

func TestGuardedBodyAllowance(t *testing.T) {
	pkg := guardedBodyFixture(t)
	for _, name := range []string{"stable", "replaced", "opaqueCell", "opaqueOwner", "mapEscape", "booleanGuard", "otherBody", "fieldReplacement"} {
		t.Run(name, func(t *testing.T) {
			var completed bool
			for limit := 0; limit <= proofs.SummaryBudget; limit++ {
				query, call, closure := guardedBodyInputs(t, pkg.Func(name))
				budget := proofs.NewSearchBudget(limit)
				got := query.proveGuardedCapturedBodyWithin(call, closure, budget)
				if budget.Exhausted() || limit == 0 {
					if got.State != proofs.EvidenceUnknown || got.Reason != resourceReasonBudgetExhausted {
						t.Fatalf("cut%d=%+v", limit, got)
					}
					continue
				}
				want := proofs.EvidenceDisproven
				if name == "stable" {
					want = proofs.EvidenceUnknown
				}
				if got.State != want || got.Reason == resourceReasonBudgetExhausted {
					t.Fatalf("complete%d=%+v, want%v; SSA:\n%s", limit, got, want, carriedSSA(t, query.function))
				}
				completed = true
				break
			}
			if !completed {
				t.Fatal("query never completed")
			}
		})
	}
}

func TestGuardedBodyChildAndFresh(t *testing.T) {
	query, call, closure := guardedBodyInputs(t, guardedBodyFixture(t).Func("stable"))
	pool := proofs.NewSearchBudget(resourcePoolBudget)
	cut := query.proveGuardedCapturedBodyWithin(call, closure, pool.Within(1))
	if cut.State != proofs.EvidenceUnknown || cut.Reason != resourceReasonBudgetExhausted || pool.Exhausted() {
		t.Fatalf("cut=%+v parent exhausted=%v", cut, pool.Exhausted())
	}
	fresh := query.proveGuardedCapturedBodyWithin(call, closure, pool.Within(proofs.SummaryBudget))
	if fresh.State != proofs.EvidenceUnknown || fresh.Reason != resourceReasonCapturedBodyGuardedCleanup {
		t.Fatalf("fresh=%+v", fresh)
	}
	query.contract.family = resourceFamilyUnknown
	if got := query.proveGuardedCapturedBodyWithin(call, closure, proofs.NewSearchBudget(0)); got.State != proofs.EvidenceDisproven {
		t.Fatalf("nonHTTP=%+v", got)
	}
}

func TestGuardedBodyFlow(t *testing.T) {
	pkg := guardedBodyFixture(t)
	for _, test := range []struct {
		name string
		want proofs.EvidenceState
	}{
		{"stable", proofs.EvidenceUnknown},
		{"booleanGuard", proofs.EvidenceProven},
		{"otherBody", proofs.EvidenceProven},
		{"fieldReplacement", proofs.EvidenceProven},
	} {
		t.Run(test.name, func(t *testing.T) {
			query, _, _ := guardedBodyInputs(t, pkg.Func(test.name))
			got := evaluateResourceFlow(nil, query.evidence, query.acquisition, query.resource, query.contract)
			if got.state != test.want || (got.leak != nil) != (test.want == proofs.EvidenceProven) {
				t.Fatalf("flow=%+v want%v; SSA:\n%s", got, test.want, carriedSSA(t, query.function))
			}
		})
	}
}
