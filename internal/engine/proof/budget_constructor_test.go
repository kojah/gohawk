package proof

import "testing"

func BenchmarkLocalSearchBudget(b *testing.B) {
	b.ReportAllocs()
	for b.Loop() {
		budget := NewSearchBudget(2)
		budget.Spend()
		budget.Spend()
		budget.Spend()
	}
}

func BenchmarkPooledSearchBudget(b *testing.B) {
	b.ReportAllocs()
	for b.Loop() {
		pool := NewSearchBudget(2)
		child := pool.Within(3)
		child.Spend()
		child.Spend()
		child.Spend()
	}
}

func BenchmarkRetainedSearchBudget(b *testing.B) {
	budgets := make([]*SearchBudget, 128)
	index := 0
	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		budgets[index%len(budgets)] = NewSearchBudget(2)
		index++
	}
}
