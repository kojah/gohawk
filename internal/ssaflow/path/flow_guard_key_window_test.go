package path

import (
	"strconv"
	"strings"
	"testing"

	proofs "github.com/kojah/gohawk/internal/proof"
)

func BenchmarkGuardKeyInterleaved(b *testing.B) {
	for _, count := range []int{1, 2, 4, 8} {
		b.Run(strconv.Itoa(count), func(b *testing.B) {
			lists := make([]PathGuards, count)
			for index := range lists {
				lists[index] = make(PathGuards, GuardLimit)
				for entry := range lists[index] {
					lists[index][entry].Identity = strings.Repeat("field:type;", 16) + strconv.Itoa(entry)
				}
				lists[index][0].Identity += strconv.Itoa(index)
			}
			var keys guardKeys
			for _, guards := range lists {
				_ = keys.keyWithin(guards, nil)
			}
			index := 0
			b.ReportAllocs()
			b.ResetTimer()
			for b.Loop() {
				_ = keys.keyWithin(lists[index], nil)
				index = (index + 1) % len(lists)
			}
		})
	}
}

func TestGuardKeyWindowKeepsOwnershipAndEviction(t *testing.T) {
	var keys guardKeys
	mutable := PathGuards{{Identity: "first", Value: true}}
	first := keys.keyWithin(mutable, nil)
	if keys.history != nil {
		t.Fatal("one-list memo allocated a history window")
	}
	mutable[0].Identity = "second"
	_ = keys.keyWithin(mutable, nil)
	mutable[0].Identity = "first"
	if got := keys.keyWithin(mutable, nil); got != first {
		t.Fatal("input mutation changed retained key evidence")
	}
	for index := range guardKeyMemoLimit * 3 {
		guards := PathGuards{{Identity: strconv.Itoa(index)}}
		if got, want := keys.keyWithin(guards, nil), guards.KeyWithin(nil); got != want {
			t.Fatal("window replacement changed the authoritative bytes")
		}
	}
	if got := keys.keyWithin(mutable, nil); got != first {
		t.Fatal("evicted key did not render its original bytes")
	}
}

func TestGuardKeyWindowChargesHistoricalHits(t *testing.T) {
	left := PathGuards{{Identity: "x"}, {Identity: "y", Value: true}, {Identity: "z"}}
	right := PathGuards{{Identity: "other"}}
	for allowance := range 4 {
		for _, pooled := range []bool{false, true} {
			var keys guardKeys
			_ = keys.keyWithin(left, nil)
			last := keys.keyWithin(right, nil)
			poolA, poolB := proofs.NewSearchBudget(allowance), proofs.NewSearchBudget(allowance)
			original, cached := poolA, poolB
			if pooled {
				original, cached = poolA.Within(4), poolB.Within(4)
			}
			want, got := left.KeyWithin(original), keys.keyWithin(left, cached)
			if got != want || original.Remaining() != cached.Remaining() || poolA.Remaining() != poolB.Remaining() ||
				original.Exhausted() != cached.Exhausted() || original.PoolExhausted() != cached.PoolExhausted() {
				t.Fatalf("historical hit changed allowance %d, pooled=%v", allowance, pooled)
			}
			if cached.Exhausted() && keys.key != last {
				t.Fatal("cutoff promoted partial historical evidence")
			}
		}
	}
}
