package ssaflow

import (
	"slices"
	"strconv"
	"testing"
)

func TestGuardFilteringPreservesEvidenceAndOwnership(t *testing.T) {
	guards := PathGuards{{Identity: "keep:0", Value: true, Stable: true}, {Identity: "eq(drop,0)"}, {Identity: "keep:1", Stable: true}}
	before := slices.Clone(guards)
	want := PathGuards{guards[0], guards[2]}
	got := guards.withoutIdentityWithin("drop", nil)
	if !slices.Equal(got, want) {
		t.Fatalf("filtered guards = %v, want %v", got, want)
	}
	got[0].Value = false
	if !slices.Equal(guards, before) {
		t.Fatal("changing filtered output changed input evidence")
	}
	got = guards.withoutIdentityWithin("unrelated", nil)
	got[0].Stable = false
	if !slices.Equal(guards, before) {
		t.Fatal("an unchanged filter shared writable input storage")
	}
	if got := guards.withoutIdentityWithin("", nil); got != nil {
		t.Fatal("removing every guard must return nil")
	}
	oversized := make(PathGuards, GuardLimit+1)
	for index := range oversized {
		oversized[index] = PathGuard{Identity: "keep:" + strconv.Itoa(index)}
	}
	got = oversized.withoutIdentityWithin("drop", nil)
	if !slices.Equal(got, oversized) {
		t.Fatal("oversized input changed its filtering policy")
	}
	got[0].Identity = "changed"
	if oversized[0].Identity != "keep:0" {
		t.Fatal("oversized output shared input storage")
	}
}

func TestGuardFilteringChargesEveryEntry(t *testing.T) {
	guards := PathGuards{{Identity: "keep:0"}, {Identity: "drop"}, {Identity: "keep:1"}}
	for allowance := range 4 {
		budget := NewSearchBudget(allowance)
		got := guards.withoutIdentityWithin("drop", budget)
		if allowance < len(guards) {
			if got != nil || !budget.Exhausted() {
				t.Fatalf("allowance %d published partial guards: %v", allowance, got)
			}
		} else if len(got) != 2 || budget.Exhausted() || budget.remaining != 0 {
			t.Fatal("exact allowance did not charge retained and removed entries")
		}
	}
	pool := NewSearchBudget(2)
	child := pool.Within(3)
	if got := guards.withoutIdentityWithin("drop", child); got != nil || !child.PoolExhausted() || child.remaining != 1 {
		t.Fatal("pool cutoff published partial evidence or changed child charges")
	}
	zero := NewSearchBudget(0)
	if got := PathGuards(nil).withoutIdentityWithin("drop", zero); got != nil || zero.Exhausted() {
		t.Fatal("empty filtering spent the budget")
	}
}

func BenchmarkGuardFiltering(b *testing.B) {
	for _, kept := range []int{0, 1, 4, GuardLimit} {
		b.Run(strconv.Itoa(kept), func(b *testing.B) {
			guards := make(PathGuards, GuardLimit)
			for index := range guards {
				guards[index].Identity = "drop"
				if index < kept {
					guards[index].Identity = "keep:" + strconv.Itoa(index)
				}
			}
			b.ReportAllocs()
			b.ResetTimer()
			for b.Loop() {
				_ = guards.withoutIdentityWithin("drop", nil)
			}
		})
	}
}
