package ssaflow

import (
	"testing"

	proofs "github.com/kojah/gohawk/internal/proof"
	"github.com/kojah/gohawk/internal/ssaflow/ssaflowtest"
	"golang.org/x/tools/go/ssa"
)

const loadedGuardFixture = `package guardformats
type record struct{ Payload *struct{ Values map[string][]*struct{ First,Second,Third,Fourth string } } }
func probe(r *record) bool { if r.Payload==nil { return true }; return false }
func unequal(r *record) bool { if r.Payload!=nil { return true }; return false }
func flag(p *bool) bool { if !*p { return true }; return false }
func unknown(p []*bool,i int) bool { if *p[i] { return true }; return false }
`

func loadedGuardCondition(tb testing.TB, name string) ssa.Value {
	tb.Helper()
	pkg := ssaflowtest.BuildPackage(tb, "guardformats", loadedGuardFixture)
	for _, instruction := range InstructionsOf[*ssa.If](pkg.Func(name)) {
		return instruction.Cond
	}
	tb.Fatal("fixture has no branch")
	return nil
}

func TestGuardFormatsPreserveBytesAndBudget(t *testing.T) {
	condition := loadedGuardCondition(t, "probe")
	var formats guardFormats
	want, negated, stable, ok := guardConditionWithin(condition, nil)
	if got, neg, fixed, found := guardConditionWithFormats(condition, nil, &formats); got != want || neg != negated || fixed != stable || found != ok {
		t.Fatal("memoized formatting changed the decoded guard")
	}
	if len(formats.loaded) != 1 {
		t.Fatal("resolved loaded guard was not memoized")
	}
	for allowance := range 16 {
		original, cached := proofs.NewSearchBudget(allowance), proofs.NewSearchBudget(allowance)
		left, ln, ls, lo := guardConditionWithin(condition, original)
		right, rn, rs, ro := guardConditionWithFormats(condition, cached, &formats)
		if left != right || ln != rn || ls != rs || lo != ro || original.Remaining() != cached.Remaining() || original.Exhausted() != cached.Exhausted() {
			t.Fatalf("warm memo changed local allowance %d", allowance)
		}
	}
}

func TestGuardFormatsPreserveSharedBudget(t *testing.T) {
	condition := loadedGuardCondition(t, "probe")
	var formats guardFormats
	_, _, _, _ = guardConditionWithFormats(condition, nil, &formats)
	for allowance := range 16 {
		poolA, poolB := proofs.NewSearchBudget(allowance), proofs.NewSearchBudget(allowance)
		childA, childB := poolA.Within(16), poolB.Within(16)
		left, ln, ls, lo := guardConditionWithin(condition, childA)
		right, rn, rs, ro := guardConditionWithFormats(condition, childB, &formats)
		if left != right || ln != rn || ls != rs || lo != ro || childA.Remaining() != childB.Remaining() || poolA.Remaining() != poolB.Remaining() ||
			childA.Exhausted() != childB.Exhausted() || childA.PoolExhausted() != childB.PoolExhausted() {
			t.Fatalf("warm memo changed shared allowance %d", allowance)
		}
	}
}

func TestGuardFormatsDoNotMemoizeUnresolvedAddresses(t *testing.T) {
	condition := loadedGuardCondition(t, "unknown")
	var formats guardFormats
	_, _, _, _ = guardConditionWithFormats(condition, nil, &formats)
	if len(formats.loaded) != 0 || len(formats.addresses) != 0 {
		t.Fatal("an opaque address populated the loaded identity memo")
	}
}

func TestGuardFormatsPreserveLoadedForms(t *testing.T) {
	for _, name := range []string{"probe", "unequal", "flag"} {
		condition := loadedGuardCondition(t, name)
		var formats guardFormats
		want, wn, ws, wo := guardConditionWithin(condition, nil)
		for range 2 {
			got, gn, gs, goOK := guardConditionWithFormats(condition, nil, &formats)
			if got != want || gn != wn || gs != ws || goOK != wo {
				t.Fatalf("memo changed loaded form %s", name)
			}
		}
	}
}

func BenchmarkGuardFormats(b *testing.B) {
	condition := loadedGuardCondition(b, "probe")
	for _, cached := range []bool{false, true} {
		name := "uncached"
		if cached {
			name = "cached"
		}
		b.Run(name, func(b *testing.B) {
			var memo guardFormats
			var formats *guardFormats
			if cached {
				formats = &memo
			}
			_, _, _, _ = guardConditionWithFormats(condition, nil, formats)
			b.ReportAllocs()
			b.ResetTimer()
			for b.Loop() {
				_, _, _, _ = guardConditionWithFormats(condition, nil, formats)
			}
		})
	}
}
