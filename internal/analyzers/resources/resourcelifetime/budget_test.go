package resourcelifetime

import "testing"

// Every query a candidate asks draws from the same pool, so the proof as a
// whole is bounded and not only each question.
func TestCandidateQueriesShareOnePool(t *testing.T) {
	analysis := &resourceAnalysis{}
	first := analysis.budget(1)
	if !first.Spend() || first.Spend() {
		t.Fatal("a query keeps its own limit")
	}
	second := analysis.budget(resourcePoolBudget)
	for range resourcePoolBudget - 1 {
		if !second.Spend() {
			t.Fatal("the pool had allowance left")
		}
	}
	if second.Spend() || !second.PoolExhausted() {
		t.Fatal("the pool, not the query, should have run out")
	}
}
