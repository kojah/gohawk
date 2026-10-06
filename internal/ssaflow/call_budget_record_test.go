package ssaflow

import (
	"strings"
	"testing"
)

func TestRecordExhaustionsNamesTheAskingSiteOnce(t *testing.T) {
	var recorded []Exhaustion
	stop := RecordExhaustions(func(exhaustion Exhaustion) { recorded = append(recorded, exhaustion) })
	budget := NewSearchBudget(1)
	for budget.Spend() {
	}
	budget.Spend()
	pool := NewSearchBudget(1)
	child := pool.Within(5)
	for child.Spend() {
	}
	var detachedPool *SearchBudget
	detached := detachedPool.Within(2)
	for detached.Spend() {
	}
	stop()
	silent := NewSearchBudget(0)
	silent.Spend()

	if len(recorded) != 4 {
		t.Fatalf("recorded %d exhaustions, want 4 (budget, pool, child, detached): %+v", len(recorded), recorded)
	}
	for _, exhaustion := range recorded {
		if !strings.HasSuffix(exhaustion.Site, "TestRecordExhaustionsNamesTheAskingSiteOnce") {
			t.Errorf("site = %q, want the test function", exhaustion.Site)
		}
	}
	if recorded[0].Limit != 1 || recorded[0].Pool {
		t.Errorf("own exhaustion = %+v, want limit 1 without pool", recorded[0])
	}
	if !recorded[2].Pool || recorded[2].Limit != 5 {
		t.Errorf("child exhaustion = %+v, want limit 5 through the pool", recorded[2])
	}
	if recorded[3].Pool || recorded[3].Limit != 2 {
		t.Errorf("detached exhaustion = %+v, want limit 2 without pool", recorded[3])
	}
}
