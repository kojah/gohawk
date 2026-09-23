package ssaflow

import "testing"

// A child budget stops at its own limit or when the pool runs out, whichever
// comes first, and reports which one it was.
func TestBudgetWithinPool(t *testing.T) {
	pool := NewSearchBudget(5)
	first := pool.Within(3)
	for range 3 {
		if !first.Spend() {
			t.Fatal("first child stopped before its own limit")
		}
	}
	if first.Spend() || first.PoolExhausted() {
		t.Fatal("first child should stop at its own limit, not the pool's")
	}
	second := pool.Within(10)
	if !second.Spend() || !second.Spend() {
		t.Fatal("pool still had two instructions")
	}
	if second.Spend() || !second.PoolExhausted() || !pool.Exhausted() {
		t.Fatal("second child should stop because the pool ran out")
	}
	if !NewSearchBudget(1).Within(2).Spend() {
		t.Fatal("a plain budget spends")
	}
	var none *SearchBudget
	if !none.Within(1).Spend() {
		t.Fatal("a nil pool yields a plain budget")
	}
}
