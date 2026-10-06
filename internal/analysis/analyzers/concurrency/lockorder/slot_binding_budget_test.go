package lockorder

import (
	"testing"

	"github.com/kojah/gohawk/internal/engine/heapmodel"
	proofs "github.com/kojah/gohawk/internal/engine/proof"
	"github.com/kojah/gohawk/internal/engine/ssaflow"
	"github.com/kojah/gohawk/internal/engine/ssaflow/ssaflowtest"
	"golang.org/x/tools/go/ssa"
)

func TestSlotBindingCutoffVetoAndFreshRecovery(t *testing.T) {
	pkg := ssaflowtest.BuildPackage(t, "slotbindings", `package slotbindings
 import "sync"
 type owner struct{mu *sync.Mutex}
 var shared sync.Mutex
 func observe(o *owner){}
 func replace(o *owner){o.mu=&shared}
 func plain(o *owner){observe(o);o.mu.Lock()}
 func changed(o *owner){replace(o);o.mu.Lock()}
 `)
	for _, test := range []struct {
		name        string
		replacement bool
	}{{"plain", false}, {"changed", true}} {
		t.Run(test.name, func(t *testing.T) {
			fn := pkg.Func(test.name)
			call := ssaflow.InstructionsOf[*ssa.Call](fn)[0]
			field := ssaflow.InstructionsOf[*ssa.FieldAddr](fn)[0]
			pool := proofs.NewSearchBudget(10 * proofs.QueryBudget)
			cut := pool.Within(0)
			if !boundSlotMutation(call, field, heapmodel.NewStorage(cut), cut) || !cut.Exhausted() || pool.Exhausted() {
				t.Fatalf("cut binding did not veto freshness: exhausted=%v/%v", cut.Exhausted(), pool.Exhausted())
			}
			fresh := pool.Within(proofs.QueryBudget)
			if got := boundSlotMutation(call, field, heapmodel.NewStorage(fresh), fresh); got != test.replacement || fresh.Exhausted() {
				t.Fatalf("fresh replacement=%v want=%v exhausted=%v", got, test.replacement, fresh.Exhausted())
			}
		})
	}
}

func TestFreshOwnerResultCensusCutoff(t *testing.T) {
	pkg := ssaflowtest.BuildPackage(t, "freshresults", `package freshresults
 type owner struct{value int}
 var shared owner
 func fresh()*owner{return new(owner)}
 func borrowed()*owner{return &shared}
 func mixed(flag bool)*owner{if flag{return new(owner)};return &shared}
 func subject(flag bool){fresh();borrowed();mixed(flag)}
 `)
	calls := ssaflow.InstructionsOf[*ssa.Call](pkg.Func("subject"))
	for index, call := range calls {
		pool := proofs.NewSearchBudget(10 * proofs.QueryBudget)
		cut := pool.Within(0)
		if freshOwnerResult(call, cut) || !cut.Exhausted() || pool.Exhausted() {
			t.Fatalf("constructor %d accepted partial census", index)
		}
		fresh := pool.Within(proofs.QueryBudget)
		if got := freshOwnerResult(call, fresh); got != (index == 0) || fresh.Exhausted() {
			t.Fatalf("constructor %d freshness=%v exhausted=%v", index, got, fresh.Exhausted())
		}
	}
}
