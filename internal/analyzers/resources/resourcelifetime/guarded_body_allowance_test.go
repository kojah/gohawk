package resourcelifetime

import (
	"testing"

	"github.com/kojah/gohawk/internal/ssaflow"
)

func TestGuardedBodyAllowance(t *testing.T) {
	pkg := guardedBodyFixture(t)
	for _, name := range []string{"stable", "replaced", "opaqueCell", "opaqueOwner", "mapEscape", "booleanGuard", "otherBody", "fieldReplacement"} {
		t.Run(name, func(t *testing.T) {
			var completed bool
			for limit := 0; limit <= ssaflow.SummaryBudget; limit++ {
				query, call, closure := guardedBodyInputs(t, pkg.Func(name))
				budget := ssaflow.NewSearchBudget(limit)
				got := query.proveGuardedCapturedBodyWithin(call, closure, budget)
				if budget.Exhausted() || limit == 0 {
					if got.State != ssaflow.EvidenceUnknown || got.Reason != resourceReasonBudgetExhausted {
						t.Fatalf("cut%d=%+v", limit, got)
					}
					continue
				}
				want := ssaflow.EvidenceDisproven
				if name == "stable" {
					want = ssaflow.EvidenceUnknown
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
	pool := ssaflow.NewSearchBudget(resourcePoolBudget)
	cut := query.proveGuardedCapturedBodyWithin(call, closure, pool.Within(1))
	if cut.State != ssaflow.EvidenceUnknown || cut.Reason != resourceReasonBudgetExhausted || pool.Exhausted() {
		t.Fatalf("cut=%+v parent exhausted=%v", cut, pool.Exhausted())
	}
	fresh := query.proveGuardedCapturedBodyWithin(call, closure, pool.Within(ssaflow.SummaryBudget))
	if fresh.State != ssaflow.EvidenceUnknown || fresh.Reason != resourceReasonCapturedBodyGuardedCleanup {
		t.Fatalf("fresh=%+v", fresh)
	}
	query.contract.family = resourceFamilyUnknown
	if got := query.proveGuardedCapturedBodyWithin(call, closure, ssaflow.NewSearchBudget(0)); got.State != ssaflow.EvidenceDisproven {
		t.Fatalf("nonHTTP=%+v", got)
	}
}

func TestGuardedBodyFlow(t *testing.T) {
	pkg := guardedBodyFixture(t)
	for _, test := range []struct {
		name string
		want ssaflow.EvidenceState
	}{
		{"stable", ssaflow.EvidenceUnknown},
		{"booleanGuard", ssaflow.EvidenceProven},
		{"otherBody", ssaflow.EvidenceProven},
		{"fieldReplacement", ssaflow.EvidenceProven},
	} {
		t.Run(test.name, func(t *testing.T) {
			query, _, _ := guardedBodyInputs(t, pkg.Func(test.name))
			got := evaluateResourceFlow(nil, query.evidence, query.acquisition, query.resource, query.contract)
			if got.state != test.want || (got.leak != nil) != (test.want == ssaflow.EvidenceProven) {
				t.Fatalf("flow=%+v want%v; SSA:\n%s", got, test.want, carriedSSA(t, query.function))
			}
		})
	}
}
