package lifecycle

import (
	"testing"

	proofs "github.com/kojah/gohawk/internal/engine/proof"
	"github.com/kojah/gohawk/internal/engine/ssaflow"
	"github.com/kojah/gohawk/internal/engine/ssaflow/ssaflowtest"
	"golang.org/x/tools/go/ssa"
)

func TestReturnedOwnershipAllowance(t *testing.T) {
	pkg := ssaflowtest.BuildPackage(t, "ownerbudget", `package ownerbudget
type owner struct { value *int; next *owner }
func direct(p *int) *int { return p }
func stored(p *int) *owner { o := &owner{value:p}; o.next=o; return o }
func copied(p *int) owner { return owner{value:p} }
func initialize(o *owner, p *int) { o.value=p }
func delegated(p *int) *owner { o:=new(owner); initialize(o,p); return o }
func maybe(p *int, fail bool) *owner { if fail { return nil }; return &owner{value:p} }
func allowed(p *int, fail bool) *owner { return maybe(p,fail) }
func mixed(p *int, fail bool) *owner { if fail { return new(owner) }; return &owner{value:p} }
func uncovered(p *int, fail bool) *owner { return mixed(p,fail) }
func callback(p *int) func() { return func(){ println(p) } }
func selected(p *int, flag bool) *owner { o:=new(owner); if flag {o=&owner{value:p}}; return o }
func unrelated(p *int) *owner { return new(owner) }
`)
	for _, test := range []struct {
		name string
		owns bool
	}{
		{"direct", true},
		{"stored", true},
		{"copied", true},
		{"delegated", true},
		{"allowed", true},
		{"uncovered", false},
		{"callback", true},
		{"selected", true},
		{"unrelated", false},
	} {
		t.Run(test.name, func(t *testing.T) {
			fn := pkg.Func(test.name)
			returned := ssaflow.InstructionsOf[*ssa.Return](fn)[0]
			for _, block := range fn.Blocks {
				for _, instruction := range block.Instrs {
					t.Log(instruction.String())
				}
			}
			completed := false
			for limit := 0; limit <= proofs.SummaryBudget; limit++ {
				pool := proofs.NewSearchBudget(limit)
				budget := pool.Within(proofs.SummaryBudget)
				got := ProveReturnedOwnershipWithin(returned, fn.Params[0], nil, budget)
				if budget.Exhausted() || budget.PoolExhausted() {
					if got.State != proofs.EvidenceUnknown || got.Reason != proofs.EvidenceBudgetExhausted {
						t.Fatalf("cutoff %d admitted %+v", limit, got)
					}
					continue
				}
				if limit == 0 || got.Proven() != test.owns || got.State == proofs.EvidenceUnknown {
					t.Fatalf("completed query = %+v, want ownership %t", got, test.owns)
				}
				completed = true
				break
			}
			if !completed {
				t.Fatal("ownership query never completed")
			}
		})
	}
}

func TestReturnedOwnershipSummaryCallbackCutoff(t *testing.T) {
	pkg := ssaflowtest.BuildPackage(t, "ownercallback", `package ownercallback
type owner struct{ p *int }
func opaque(*int) *owner
func returned(p *int) *owner { return opaque(p) }
`)
	fn := pkg.Func("returned")
	returned := ssaflow.InstructionsOf[*ssa.Return](fn)[0]
	pool := proofs.NewSearchBudget(proofs.SummaryBudget)
	budget := pool.Within(proofs.SummaryBudget)
	sibling := pool.Within(proofs.SummaryBudget)
	called := false
	hook := func(*ssa.Function, int) bool {
		called = true
		for sibling.Spend() {
		}
		return true
	}
	got := ProveReturnedOwnershipWithin(returned, fn.Params[0], hook, budget)
	if !called || got.State != proofs.EvidenceUnknown || got.Reason != proofs.EvidenceBudgetExhausted {
		t.Fatalf("callback cutoff = %+v, called %t", got, called)
	}
	fresh := ProveReturnedOwnershipWithin(returned, fn.Params[0], func(*ssa.Function, int) bool { return true }, proofs.NewSearchBudget(proofs.SummaryBudget))
	if !fresh.Proven() {
		t.Fatal("fresh query failed to recover summary ownership")
	}
}

func TestReturnedParameterSharedAllowance(t *testing.T) {
	pkg := ssaflowtest.BuildPackage(t, "identity", `package identity
type wrapper struct { p *int }
func direct(p *int) *int { return p }
func branches(p *int, flag bool) *int { if flag { return p }; return p }
func mixed(p *int, flag bool) *int { q := p; if flag { q = new(int) }; return q }
func boxed(p *int) any { return p }
func wrapped(p *int) wrapper { return wrapper{p} }
func forever(p *int) *int { for {} }
func recovered(p *int) *int { defer func() { recover() }(); return p }
`)
	for _, test := range []struct {
		name string
		want bool
	}{{"direct", true}, {"branches", true}, {"mixed", false}, {"boxed", false}, {"wrapped", false}, {"forever", false}, {"recovered", true}} {
		t.Run(test.name, func(t *testing.T) {
			function := pkg.Func(test.name)
			t.Log(function.String())
			for _, block := range function.Blocks {
				for _, instruction := range block.Instrs {
					t.Log(instruction.String())
				}
			}
			fresh := proofs.NewSearchBudget(proofs.QueryBudget)
			proof := ProveReturnedParameterWithin(function, function.Params[0], 0, fresh)
			if proof.Proven() != test.want || fresh.Exhausted() {
				t.Fatalf("fresh proof = %+v, want proven %v", proof, test.want)
			}
			if ReturnsParameterUnchanged(function, function.Params[0], 0) != test.want {
				t.Fatal("default facade disagrees")
			}
			if !test.want {
				return
			}
			completed := false
			for limit := 0; limit <= proofs.QueryBudget; limit++ {
				cut := proofs.NewSearchBudget(limit)
				got := ProveReturnedParameterWithin(function, function.Params[0], 0, cut)
				if got.Proven() {
					completed = true
					break
				}
				if got.State != proofs.EvidenceUnknown || got.Reason != proofs.EvidenceBudgetExhausted || !cut.Exhausted() {
					t.Fatalf("allowance %d admitted incomplete proof: %+v", limit, got)
				}
			}
			if !completed {
				t.Fatal("no complete allowance found")
			}
			pool := proofs.NewSearchBudget(0)
			got := ProveReturnedParameterWithin(function, function.Params[0], 0, pool.Within(proofs.QueryBudget))
			if got.Proven() || got.Reason != proofs.EvidenceBudgetExhausted || !pool.Exhausted() {
				t.Fatal("exhausted pool admitted identity")
			}
		})
	}
}

func TestReturnedSliceElementOwnership(t *testing.T) {
	pkg := buildTestSSA(t, `package ssaflowtest
type resource struct{}
func stored(value, other *resource) []*resource {
	values := make([]*resource, 3)
	values[1] = value
	return values
}
func differentElement(value, other *resource) []*resource {
	values := make([]*resource, 3)
	values[1] = other
	return values
}
func differentSlice(value, other *resource) []*resource {
	values := make([]*resource, 3)
	values[1] = value
	return make([]*resource, 3)
}
func dynamic(value, other *resource, index int) []*resource {
	values := make([]*resource, 3)
	values[index] = value
	return values
}
`)
	for _, test := range []struct {
		name string
		owns bool
	}{
		{"stored", true},
		{"differentElement", false},
		{"differentSlice", false},
		{"dynamic", true},
	} {
		t.Run(test.name, func(t *testing.T) {
			function := pkg.Func(test.name)
			instruction := findSSAInstruction(t, function, func(instruction ssa.Instruction) bool {
				_, ok := instruction.(*ssa.Return)
				return ok
			})
			if owns := ReturnedValueOwnsValue(instruction.(*ssa.Return), function.Params[0]); owns != test.owns {
				t.Errorf("ReturnedValueOwnsValue = %t, want %t", owns, test.owns)
			}
		})
	}
}
