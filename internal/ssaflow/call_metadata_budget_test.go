package ssaflow

import (
	"slices"
	"testing"

	proofs "github.com/kojah/gohawk/internal/proof"
	"github.com/kojah/gohawk/internal/ssaflow/ssaflowtest"
	"golang.org/x/tools/go/ssa"
)

func TestCallBindingMetadataBudget(t *testing.T) {
	pkg := ssaflowtest.BuildPackage(t, "metadata", `package metadata
func subject(first, second chan int) { go func(arg chan int) { <-arg; <-first; <-second }(first) }
`)
	spawn := InstructionsOf[*ssa.Go](pkg.Func("subject"))[0]
	function, closure := DirectCallee(spawn.Common())
	if len(function.Params) != 1 || len(function.FreeVars) != 2 {
		t.Fatal("expected one argument and two actual lexical captures")
	}
	want := []CallBinding{
		{Local: function.Params[0], Supplied: spawn.Common().Args[0]},
		{Local: function.FreeVars[0], Supplied: closure.Bindings[0], Captured: true},
		{Local: function.FreeVars[1], Supplied: closure.Bindings[1], Captured: true},
	}
	fresh := proofs.NewSearchBudget(3)
	got := slices.Collect(CallBindingsWithin(spawn.Common(), function, closure, fresh))
	if !slices.Equal(got, want) || fresh.Exhausted() || fresh.Spend() || !slices.Equal(CallBindings(spawn.Common(), function, closure), want) {
		t.Fatal("fresh enumeration must preserve argument/capture order with exactly three visits")
	}
	pool := proofs.NewSearchBudget(1)
	limited := pool.Within(3)
	got = slices.Collect(CallBindingsWithin(spawn.Common(), function, closure, limited))
	if !slices.Equal(got, want[:1]) || !limited.PoolExhausted() {
		t.Fatal("partial metadata must retain pool cutoff availability")
	}
	early := proofs.NewSearchBudget(1)
	for binding := range CallBindingsWithin(spawn.Common(), function, closure, early) {
		if binding != want[0] {
			t.Fatal("early witness changed binding order")
		}
		break
	}
	if early.Exhausted() || early.Spend() {
		t.Fatal("early stop must spend exactly one metadata visit")
	}
	captures := proofs.NewSearchBudget(1)
	pairs := slices.Collect(ClosureBindingPairsWithin(function, closure, captures))
	if len(pairs) != 1 || pairs[0].Free != function.FreeVars[0] || pairs[0].Binding != closure.Bindings[0] || !captures.Exhausted() {
		t.Fatal("capture cutoff must retain only the inspected prefix")
	}
	if pairs := ClosureBindingPairs(function, closure); len(pairs) != 2 || pairs[1].Binding != closure.Bindings[1] {
		t.Fatal("default capture collection lost a lexical binding")
	}
}

func TestCallResultMetadataBudget(t *testing.T) {
	pkg := ssaflowtest.BuildPackage(t, "results", `package results
func triple() (int,int,int) { return 1,2,3 }
func single() int { return 4 }
func subject() (int,int,int,int) { a,b,c:=triple(); return a,b,c,single() }
`)
	calls := InstructionsOf[*ssa.Call](pkg.Func("subject"))
	tuple := calls[0]
	refs := *tuple.Referrers()
	if len(refs) != 3 {
		t.Fatal("expected three actual tuple-result extracts")
	}
	last := refs[2].(*ssa.Extract)
	cutoff := proofs.NewSearchBudget(2)
	if CallResultWithin(tuple, last.Index, cutoff) != nil || !cutoff.Exhausted() {
		t.Fatal("an unvisited result must be unavailable at cutoff")
	}
	fresh := proofs.NewSearchBudget(3)
	if CallResultWithin(tuple, last.Index, fresh) != last || fresh.Exhausted() || fresh.Spend() || CallResult(tuple, last.Index) != last {
		t.Fatal("fresh result lookup must select the exact last result with three visits")
	}
	first := refs[0].(*ssa.Extract)
	early := proofs.NewSearchBudget(1)
	if CallResultWithin(tuple, first.Index, early) != first || early.Exhausted() || early.Spend() {
		t.Fatal("result lookup must stop at its first exact witness")
	}
	absent := proofs.NewSearchBudget(3)
	if CallResultWithin(tuple, 7, absent) != nil || absent.Exhausted() {
		t.Fatal("a completed absent-result scan must differ from cutoff")
	}
	zero := proofs.NewSearchBudget(0)
	if CallResultWithin(calls[1], -1, zero) != nil || !zero.Exhausted() {
		t.Fatal("single-result lookup must charge its allowance")
	}
	if CallResultWithin(calls[1], -1, proofs.NewSearchBudget(1)) != calls[1] || CallResult(calls[1], -1) != calls[1] {
		t.Fatal("single-result representation must stay the call itself")
	}
}
