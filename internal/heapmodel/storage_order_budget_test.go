package heapmodel

import (
	"strings"
	"testing"

	"github.com/kojah/gohawk/internal/ssaflow"
	"github.com/kojah/gohawk/internal/ssaflow/ssaflowtest"
	"golang.org/x/tools/go/ssa"
)

func TestStoreFollowingKeepsFreshAllocationBoundary(t *testing.T) {
	pkg := ssaflowtest.BuildPackage(t, "storeorder", `package storeorder
 func observe(**int){}
 func after(p,q *int){x:=p;observe(&x);x=q}
 func before(p,q *int){x:=p;x=q;observe(&x)}
 func branch(p,q *int,flag bool){x:=p;observe(&x);if flag{x=q}}
 func fresh(p *int,flag bool){for flag{x:=p;if *p>0{observe(&x)}}}
 func reused(p *int,flag bool){var x *int;for flag{x=p;if *p>0{observe(&x)}}}
 `)
	for _, test := range []struct {
		name    string
		follows bool
		stable  bool
	}{
		{"after", true, false}, {"before", false, true}, {"branch", true, false}, {"fresh", false, true}, {"reused", true, false},
	} {
		t.Run(test.name, func(t *testing.T) {
			fn := pkg.Func(test.name)
			call := ssaflow.InstructionsOf[*ssa.Call](fn)[0]
			cell := call.Common().Args[0]
			var last *ssa.Store
			for _, store := range ssaflow.InstructionsOf[*ssa.Store](fn) {
				if store.Addr == cell {
					last = store
				}
			}
			if last == nil {
				t.Fatal("missing observed cell store")
			}
			var dump strings.Builder
			if _, err := fn.WriteTo(&dump); err != nil {
				t.Fatal(err)
			}
			t.Log(dump.String())
			checkStoreFollowingAllowance(t, cell, call, last, test.follows)
			checkStableContentsAllowance(t, cell, call, test.stable)
		})
	}
}

func checkStoreFollowingAllowance(t *testing.T, cell ssa.Value, call *ssa.Call, last *ssa.Store, want bool) {
	t.Helper()
	cut := ssaflow.NewSearchBudget(1)
	if StoreMayFollowWithin(cell, call, last, cut) || !cut.Exhausted() {
		t.Fatal("store-order query bypassed caller allowance")
	}
	for limit := 1; limit <= ssaflow.QueryBudget; limit++ {
		budget := ssaflow.NewSearchBudget(limit)
		follows := StoreMayFollowWithin(cell, call, last, budget)
		if budget.Exhausted() {
			if follows {
				t.Fatalf("cutoff %d published following store", limit)
			}
			continue
		}
		if follows != want {
			t.Fatalf("following store=%v, want %v", follows, want)
		}
		return
	}
	t.Fatal("store ordering did not recover")
}

func checkStableContentsAllowance(t *testing.T, cell ssa.Value, call *ssa.Call, want bool) {
	t.Helper()
	for limit := 1; limit <= 10*ssaflow.QueryBudget; limit++ {
		budget := ssaflow.NewSearchBudget(limit)
		proof := NewStorage(budget).StableContent(cell, call)
		if budget.Exhausted() {
			if proof.Proven() {
				t.Fatalf("cutoff %d published stable contents", limit)
			}
			continue
		}
		if proof.Proven() != want {
			t.Fatalf("stable contents=%+v, want %v", proof, want)
		}
		return
	}
	t.Fatal("stable storage did not recover")
}
