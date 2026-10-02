package resourcelifetime

import (
	"testing"

	"github.com/kojah/gohawk/internal/ssaflow"
)

// Shared allowance controls require complete answers to match the default
// proof and every interrupted answer to retain the resource unknown boundary.
func checkResourceProofAllowance(t *testing.T, prove func(*ssaflow.SearchBudget) resourceProof, want bool) {
	t.Helper()
	if got := prove(nil); got.State == ssaflow.EvidenceUnknown || got.Proven() != want {
		t.Fatalf("default resource evidence = %+v, want %v", got, want)
	}
	for limit := 0; limit <= ssaflow.SummaryBudget; limit++ {
		budget := ssaflow.NewSearchBudget(limit)
		got := prove(budget)
		if resourceFlowExhausted(budget) || limit == 0 {
			if got.State != ssaflow.EvidenceUnknown || got.Reason != resourceReasonBudgetExhausted {
				t.Fatalf("allowance %d retained interrupted resource evidence: %+v", limit, got)
			}
			continue
		}
		if got.State == ssaflow.EvidenceUnknown || got.Proven() != want {
			t.Fatalf("complete resource evidence = %+v, want %v", got, want)
		}
		return
	}
	t.Fatal("resource query never completed")
}
