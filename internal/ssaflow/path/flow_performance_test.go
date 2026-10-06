package path

import (
	"strconv"
	"testing"

	proofs "github.com/kojah/gohawk/internal/proof"
)

func BenchmarkPathGuardKey(b *testing.B) {
	for _, count := range []int{0, 1, 8, 32} {
		b.Run(strconv.Itoa(count), func(b *testing.B) {
			guards := make(PathGuards, count)
			for index := range guards {
				guards[index] = PathGuard{Identity: "value:" + strconv.Itoa(index), Value: index%2 == 0}
			}
			b.ReportAllocs()
			for b.Loop() {
				_ = guards.KeyWithin(nil)
			}
		})
	}
}

func TestPathGuardKeyBytesAndBudget(t *testing.T) {
	zero := proofs.NewSearchBudget(0)
	if got := PathGuards(nil).KeyWithin(zero); got != "" || zero.Exhausted() {
		t.Fatal("an empty key must not spend the visit allowance")
	}
	guards := PathGuards{{Identity: "value:%;=", Value: true}, {Identity: "", Value: false}}
	if got := guards.KeyWithin(nil); got != "value:%;==true;=false" {
		t.Fatalf("unexpected key %q", got)
	}
	if got := guards.KeyWithin(proofs.NewSearchBudget(1)); got != "" {
		t.Fatalf("partial key escaped cutoff: %q", got)
	}
	budget := proofs.NewSearchBudget(2)
	if got := guards.KeyWithin(budget); got != "value:%;==true;=false" || budget.Exhausted() {
		t.Fatalf("exact-budget key %q unavailable", got)
	}
	pool := proofs.NewSearchBudget(1)
	child := pool.Within(2)
	if got := guards.KeyWithin(child); got != "" || !child.PoolExhausted() {
		t.Fatalf("partial pool key %q escaped cutoff", got)
	}
	pool = proofs.NewSearchBudget(2)
	child = pool.Within(2)
	if got := guards.KeyWithin(child); got != "value:%;==true;=false" || child.Exhausted() || pool.Exhausted() {
		t.Fatalf("exact pool key %q unavailable", got)
	}
}

func TestPathGuardCapacityOverflow(t *testing.T) {
	maxInt := int(^uint(0) >> 1)
	if got := pathGuardKeyCapacity(0, maxInt-5, true, false); got != maxInt {
		t.Fatalf("largest fitting key capacity %d", got)
	}
	for _, size := range []int{0, 1, maxInt - 4, maxInt} {
		if got := pathGuardKeyCapacity(size, maxInt-4, true, false); got != -1 {
			t.Fatalf("overflow from size %d returned %d", size, got)
		}
	}
	if got := pathGuardKeyCapacity(maxInt-5, 0, true, true); got != -1 {
		t.Fatalf("separator overflow returned %d", got)
	}
	if got := pathGuardKeyCapacity(-1, 0, false, false); got != -1 {
		t.Fatalf("overflow hint became available: %d", got)
	}
}
