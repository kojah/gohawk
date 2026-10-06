package heapmodel

import (
	"testing"

	proofs "github.com/kojah/gohawk/internal/engine/proof"
	"github.com/kojah/gohawk/internal/engine/ssaflow"
	"github.com/kojah/gohawk/internal/engine/ssaflow/ssaflowtest"
	"golang.org/x/tools/go/ssa"
)

func TestStorageOrderingAndDominanceCutoff(t *testing.T) {
	pkg := ssaflowtest.BuildPackage(t, "order", `package order
 func observe(a,b *int) {}
 func read(p **int) { _ = *p }
 func probe(a *int) { x:=a; read(&x); observe(x,a) }
`)
	function := pkg.Func("probe")
	call := heapObservation(t, function)
	load := call.Common().Args[0].(*ssa.UnOp)
	cell := load.X.(*ssa.Alloc)
	stores := ssaflow.InstructionsOf[*ssa.Store](function)
	if len(stores) != 1 {
		t.Fatal("expected a sole actual SSA initializer")
	}
	storage := NewStorage(proofs.NewSearchBudget(1))
	if proof := storage.reachingContent(storageLocation{root: cell}, load, stores); proof.Proven() || proof.Reason != proofs.EvidenceBudgetExhausted {
		t.Fatalf("unavailable initializer dominance must stay unknown: %+v", proof)
	}
	fresh := NewStorage(proofs.NewSearchBudget(proofs.QueryBudget))
	if proof := fresh.reachingContent(storageLocation{root: cell}, load, stores); !proof.Proven() || proof.Value != call.Common().Args[1] {
		t.Fatal("fresh initializer dominance lost exact contents")
	}
	storage = NewStorage(proofs.NewSearchBudget(1))
	var collected []*ssa.Store
	_, ok := storage.collect(cell, load, &collected, false)
	if ok || !storage.Budget().Exhausted() || len(collected) != 0 {
		t.Fatal("a referrer must not be admitted after its order query cuts off")
	}
}
