package ssaflow

import (
	"slices"
	"testing"

	"github.com/kojah/gohawk/internal/ssaflow/ssaflowtest"
	"golang.org/x/tools/go/ssa"
)

func TestGuardAddressFormatsPreserveInvalidation(t *testing.T) {
	pkg := ssaflowtest.BuildPackage(t, "invalidation", `package invalidation
 type record struct{ flag bool }
 func result() *record { return nil }
 func pair() (*record,bool) { return nil,false }
 func subject(r *record) { r.flag=true; q:=result(); q.flag=false; p,_:=pair(); p.flag=true }
 `)
	fn := pkg.Func("subject")
	var instructions []ssa.Instruction
	for _, store := range InstructionsOf[*ssa.Store](fn) {
		instructions = append(instructions, store)
	}
	for _, call := range InstructionsOf[*ssa.Call](fn) {
		instructions = append(instructions, call)
	}
	for _, extract := range InstructionsOf[*ssa.Extract](fn) {
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
				original, cached := NewSearchBudget(allowance), NewSearchBudget(allowance)
				want := guards.AfterWithin(instruction, original)
				got := guards.afterWithFormats(instruction, cached, &formats)
				if !slices.Equal(got, want) || (got == nil) != (want == nil) || original.remaining != cached.remaining ||
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
	call := InstructionsOf[*ssa.Call](pkg.Func("subject"))[0]
	for _, guards := range []PathGuards{nil, {}} {
		budget := NewSearchBudget(0)
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
	store := InstructionsOf[*ssa.Store](pkg.Func("subject"))[0]
	identity, _ := guardAddressIdentityWithin(store.Addr, nil)
	guards := PathGuards{{Identity: identity}, {Identity: "unrelated"}}
	var formats guardFormats
	_ = guards.afterWithFormats(store, nil, &formats)
	for allowance := range 6 {
		poolA, poolB := NewSearchBudget(allowance), NewSearchBudget(allowance)
		left, right := poolA.Within(6), poolB.Within(6)
		want := guards.AfterWithin(store, left)
		got := guards.afterWithFormats(store, right, &formats)
		if !slices.Equal(got, want) || (got == nil) != (want == nil) || left.remaining != right.remaining || poolA.remaining != poolB.remaining ||
			left.Exhausted() != right.Exhausted() || left.PoolExhausted() != right.PoolExhausted() {
			t.Fatalf("memoized invalidation changed shared allowance %d", allowance)
		}
	}
}
