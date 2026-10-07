package path

import (
	"slices"
	"strconv"
	"testing"

	proofs "github.com/kojah/gohawk/internal/engine/proof"
	ssaflow "github.com/kojah/gohawk/internal/engine/ssaflow"
	"github.com/kojah/gohawk/internal/engine/ssaflow/ssaflowtest"
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
	for _, instruction := range ssaflow.InstructionsOf[*ssa.If](pkg.Func(name)) {
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

func TestGuardAddressFormatsPreserveInvalidation(t *testing.T) {
	pkg := ssaflowtest.BuildPackage(t, "invalidation", `package invalidation
 type record struct{ flag bool }
 func result() *record { return nil }
 func pair() (*record,bool) { return nil,false }
 func subject(r *record) { r.flag=true; q:=result(); q.flag=false; p,_:=pair(); p.flag=true }
 `)
	fn := pkg.Func("subject")
	var instructions []ssa.Instruction
	for _, store := range ssaflow.InstructionsOf[*ssa.Store](fn) {
		instructions = append(instructions, store)
	}
	for _, call := range ssaflow.InstructionsOf[*ssa.Call](fn) {
		instructions = append(instructions, call)
	}
	for _, extract := range ssaflow.InstructionsOf[*ssa.Extract](fn) {
		instructions = append(instructions, extract)
	}
	for _, instruction := range instructions {
		t.Run(instruction.String(), func(t *testing.T) {
			var identity string
			if store, ok := instruction.(*ssa.Store); ok {
				identity, _ = guardAddressIdentityWithin(store.Addr, nil)
			} else {
				identity = guardOperandIdentity(instruction.(ssa.Value))
			}
			guards := PathGuards{{Identity: "load(" + identity + ")"}, {Identity: "unrelated", Value: true, Stable: true}}
			var formats guardFormats
			got := guards.afterWithFormats(instruction, nil, &formats)
			if !slices.Equal(got, guards[1:]) || len(formats.addresses) == 0 {
				t.Fatal("warm invalidation did not retain exactly the unrelated guard")
			}
			for allowance := range 8 {
				original, cached := proofs.NewSearchBudget(allowance), proofs.NewSearchBudget(allowance)
				want := guards.AfterWithin(instruction, original)
				got := guards.afterWithFormats(instruction, cached, &formats)
				if !slices.Equal(got, want) || (got == nil) != (want == nil) || original.Remaining() != cached.Remaining() ||
					original.Exhausted() != cached.Exhausted() {
					t.Fatalf("memoized invalidation changed allowance %d", allowance)
				}
			}
			got = guards.afterWithFormats(instruction, nil, &formats)
			got[0].Stable = false
			if !guards[1].Stable {
				t.Fatal("formatted invalidation shared writable evidence")
			}
		})
	}
}

func TestEmptyCallInvalidationKeepsNilAndBudget(t *testing.T) {
	pkg := ssaflowtest.BuildPackage(t, "empty", `package empty
 func callee(){}
 func subject(){callee()}
 `)
	call := ssaflow.InstructionsOf[*ssa.Call](pkg.Func("subject"))[0]
	for _, guards := range []PathGuards{nil, {}} {
		budget := proofs.NewSearchBudget(0)
		var formats guardFormats
		if got := guards.afterWithFormats(call, budget, &formats); got != nil || budget.Exhausted() || len(formats.addresses) != 0 {
			t.Fatal("empty invalidation changed nil evidence, charges or memo")
		}
	}
}

func TestGuardAddressFormatsPreserveSharedInvalidationBudget(t *testing.T) {
	pkg := ssaflowtest.BuildPackage(t, "sharedinvalidation", `package sharedinvalidation
 type record struct{ flag bool }
 func subject(r *record){r.flag=true}
 `)
	store := ssaflow.InstructionsOf[*ssa.Store](pkg.Func("subject"))[0]
	identity, _ := guardAddressIdentityWithin(store.Addr, nil)
	guards := PathGuards{{Identity: identity}, {Identity: "unrelated"}}
	var formats guardFormats
	_ = guards.afterWithFormats(store, nil, &formats)
	for allowance := range 6 {
		poolA, poolB := proofs.NewSearchBudget(allowance), proofs.NewSearchBudget(allowance)
		left, right := poolA.Within(6), poolB.Within(6)
		want := guards.AfterWithin(store, left)
		got := guards.afterWithFormats(store, right, &formats)
		if !slices.Equal(got, want) || (got == nil) != (want == nil) || left.Remaining() != right.Remaining() || poolA.Remaining() != poolB.Remaining() ||
			left.Exhausted() != right.Exhausted() || left.PoolExhausted() != right.PoolExhausted() {
			t.Fatalf("memoized invalidation changed shared allowance %d", allowance)
		}
	}
}

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
		budget := proofs.NewSearchBudget(allowance)
		got := guards.withoutIdentityWithin("drop", budget)
		if allowance < len(guards) {
			if got != nil || !budget.Exhausted() {
				t.Fatalf("allowance %d published partial guards: %v", allowance, got)
			}
		} else if len(got) != 2 || budget.Exhausted() || budget.Remaining() != 0 {
			t.Fatal("exact allowance did not charge retained and removed entries")
		}
	}
	pool := proofs.NewSearchBudget(2)
	child := pool.Within(3)
	if got := guards.withoutIdentityWithin("drop", child); got != nil || !child.PoolExhausted() || child.Remaining() != 1 {
		t.Fatal("pool cutoff published partial evidence or changed child charges")
	}
	zero := proofs.NewSearchBudget(0)
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
