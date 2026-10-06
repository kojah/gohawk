package resourcelifetime

import (
	"testing"

	proofs "github.com/kojah/gohawk/internal/proof"
)

// Shared allowance controls require complete answers to match the default
// proof and every interrupted answer to retain the resource unknown boundary.
func checkResourceProofAllowance(t *testing.T, prove func(*proofs.SearchBudget) resourceProof, want bool) {
	t.Helper()
	if got := prove(nil); got.State == proofs.EvidenceUnknown || got.Proven() != want {
		t.Fatalf("default resource evidence = %+v, want %v", got, want)
	}
	for limit := 0; limit <= proofs.SummaryBudget; limit++ {
		budget := proofs.NewSearchBudget(limit)
		got := prove(budget)
		if resourceFlowExhausted(budget) || limit == 0 {
			if got.State != proofs.EvidenceUnknown || got.Reason != resourceReasonBudgetExhausted {
				t.Fatalf("allowance %d retained interrupted resource evidence: %+v", limit, got)
			}
			continue
		}
		if got.State == proofs.EvidenceUnknown || got.Proven() != want {
			t.Fatalf("complete resource evidence = %+v, want %v", got, want)
		}
		return
	}
	t.Fatal("resource query never completed")
}
