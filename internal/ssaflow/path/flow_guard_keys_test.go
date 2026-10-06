package path

import (
	"strconv"
	"strings"
	"testing"

	proofs "github.com/kojah/gohawk/internal/proof"
)

func TestGuardKeyMemoPreservesBytesAndChanges(t *testing.T) {
	var keys guardKeys
	for _, guards := range []PathGuards{
		nil,
		{{Identity: "x", Value: true}},
		{{Identity: "x", Value: true, Stable: true}},
		{{Identity: "x", Value: false}},
		{{Identity: "x", Value: true}, {Identity: "y"}},
		{{Identity: "x=true;y"}},
	} {
		for range 2 {
			if got, want := keys.keyWithin(guards, nil), guards.KeyWithin(nil); got != want {
				t.Fatalf("memo key %q differs from original %q", got, want)
			}
		}
	}
	left := PathGuards{{Identity: "x", Value: true}, {Identity: "y"}}
	right := PathGuards{{Identity: "x=true;y"}}
	if keys.keyWithin(left, nil) != keys.keyWithin(right, nil) {
		t.Fatal("memo changed legacy delimiter-collision equality")
	}
	large := make(PathGuards, GuardLimit+1)
	if got := keys.keyWithin(large, nil); got != large.KeyWithin(nil) {
		t.Fatal("oversized input changed its key")
	}
	mutable := PathGuards{{Identity: "before"}}
	_ = keys.keyWithin(mutable, nil)
	mutable[0].Identity = "after"
	mutable[0].Value = true
	if got := keys.keyWithin(mutable, nil); got != mutable.KeyWithin(nil) {
		t.Fatal("input mutation reused a stale key")
	}
}

func TestGuardKeyMemoPreservesBudget(t *testing.T) {
	guards := PathGuards{{Identity: "x"}, {Identity: "y", Value: true}, {Identity: "z"}}
	var keys guardKeys
	warm := keys.keyWithin(guards, nil)
	for allowance := range 4 {
		original, cached := proofs.NewSearchBudget(allowance), proofs.NewSearchBudget(allowance)
		if left, right := guards.KeyWithin(original), keys.keyWithin(guards, cached); left != right ||
			original.Remaining() != cached.Remaining() || original.Exhausted() != cached.Exhausted() {
			t.Fatalf("memo changed local allowance %d", allowance)
		}
		poolA, poolB := proofs.NewSearchBudget(allowance), proofs.NewSearchBudget(allowance)
		childA, childB := poolA.Within(4), poolB.Within(4)
		if left, right := guards.KeyWithin(childA), keys.keyWithin(guards, childB); left != right ||
			childA.Remaining() != childB.Remaining() || poolA.Remaining() != poolB.Remaining() ||
			childA.Exhausted() != childB.Exhausted() || childA.PoolExhausted() != childB.PoolExhausted() {
			t.Fatalf("memo changed shared allowance %d", allowance)
		}
		if keys.key != warm {
			t.Fatal("partial key poisoned the memo")
		}
	}
	zero := proofs.NewSearchBudget(0)
	if got := keys.keyWithin(nil, zero); got != "" || zero.Exhausted() {
		t.Fatal("empty key spent allowance")
	}
}

func BenchmarkGuardKeyMemo(b *testing.B) {
	for _, cached := range []bool{false, true} {
		name := "uncached"
		if cached {
			name = "cached"
		}
		b.Run(name, func(b *testing.B) {
			guards := make(PathGuards, GuardLimit)
			for index := range guards {
				guards[index].Identity = strings.Repeat("field:type;", 16) + strconv.Itoa(index)
			}
			var memo guardKeys
			var keys *guardKeys
			if cached {
				keys = &memo
			}
			_ = keys.keyWithin(guards, nil)
			b.ReportAllocs()
			b.ResetTimer()
			for b.Loop() {
				_ = keys.keyWithin(guards, nil)
			}
		})
	}
}
