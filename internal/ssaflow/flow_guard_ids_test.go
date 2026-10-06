package ssaflow

import (
	"strings"
	"testing"

	proofs "github.com/kojah/gohawk/internal/proof"
)

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
