package path

import (
	"strconv"
	"strings"
	"testing"

	proofs "github.com/kojah/gohawk/internal/engine/proof"
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

func TestGuardIDsPreserveByteEqualityAndCutoff(t *testing.T) {
	var ids guardIDs
	left := PathGuards{{Identity: "x", Value: true}, {Identity: "y"}}
	right := PathGuards{{Identity: "x=true;y"}}
	first := ids.within(left, nil)
	if first == 0 || ids.within(right, nil) != first {
		t.Fatal("interning changed legacy byte equality")
	}
	if ids.within(PathGuards{{Identity: "other"}}, nil) == first || ids.within(left, nil) != first {
		t.Fatal("interleaved keys lost distinctness or identity")
	}
	before := len(ids.known)
	cut := proofs.NewSearchBudget(1)
	if got := ids.within(left, cut); got != 0 || !cut.Exhausted() || len(ids.known) != before {
		t.Fatal("cutoff published or interned a partial key")
	}
	if ids.within(nil, proofs.NewSearchBudget(0)) != 0 {
		t.Fatal("empty guard key must retain the zero ID")
	}
}

func TestGuardIDsPreserveLocalAndSharedCharges(t *testing.T) {
	guards := PathGuards{{Identity: "x"}, {Identity: "y", Value: true}, {Identity: "z"}}
	var ids guardIDs
	warm := ids.within(guards, nil)
	for allowance := range 4 {
		for _, pooled := range []bool{false, true} {
			original, cached := proofs.NewSearchBudget(allowance), proofs.NewSearchBudget(allowance)
			left, right := original, cached
			if pooled {
				left, right = original.Within(4), cached.Within(4)
			}
			encoded, id := guards.KeyWithin(left), ids.within(guards, right)
			if left.Remaining() != right.Remaining() || original.Remaining() != cached.Remaining() ||
				left.Exhausted() != right.Exhausted() || left.PoolExhausted() != right.PoolExhausted() {
				t.Fatalf("interning changed allowance %d, pooled=%v", allowance, pooled)
			}
			if (encoded == "") != (id == 0) || (id != 0 && id != warm) {
				t.Fatal("interning changed complete-key availability")
			}
		}
	}
}

func BenchmarkGuardVisitedHash(b *testing.B) {
	type stringKey struct {
		location FlowLocationKey
		covered  ObligationAction
	}
	guards := PathGuards{{Identity: strings.Repeat("field:type;", 64), Value: true}}
	for _, compact := range []bool{false, true} {
		name := "strings"
		if compact {
			name = "ids"
		}
		b.Run(name, func(b *testing.B) {
			var ids guardIDs
			var keys guardKeys
			key := stringKey{location: FlowLocationKey{guards: keys.keyWithin(guards, nil)}, covered: ObligationNone}
			id := ids.within(guards, nil)
			stringsSeen := map[stringKey]bool{key: true}
			idsSeen := map[obligationKey]bool{{guards: id}: true}
			b.ReportAllocs()
			b.ResetTimer()
			for b.Loop() {
				if compact {
					if !idsSeen[obligationKey{guards: ids.within(guards, nil)}] {
						b.Fatal("ID lost its visited entry")
					}
				} else {
					key.location.guards = keys.keyWithin(guards, nil)
					if !stringsSeen[key] {
						b.Fatal("key lost its visited entry")
					}
				}
			}
		})
	}
}

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
