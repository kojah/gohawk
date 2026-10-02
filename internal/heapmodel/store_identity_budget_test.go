package heapmodel

import (
	"go/token"
	"testing"

	"github.com/kojah/gohawk/internal/ssaflow"
	"github.com/kojah/gohawk/internal/ssaflow/ssaflowtest"
	"golang.org/x/tools/go/ssa"
)

func TestStorageStructuralIdentityBudget(t *testing.T) {
	pkg := ssaflowtest.BuildPackage(t, "identity", `package identity
 type owner struct { field int }
 func observe(a,b *int) {}
 func probe(p *owner) { observe(&p.field,&p.field) }
`)
	args := heapObservation(t, pkg.Func("probe")).Common().Args
	if args[0] == args[1] {
		t.Fatal("expected distinct SSA field selections")
	}
	for _, writesOnly := range []bool{false, true} {
		observed := 0
		budget := ssaflow.NewSearchBudget(1).Observed(func(reason string, _ token.Pos, _ map[string]string) {
			if reason != ssaflow.EvidenceBudgetExhausted.String() {
				t.Errorf("unexpected cutoff reason: %s", reason)
			}
			observed++
		})
		storage := NewStorage(budget)
		storage.writesOnly = writesOnly
		proof := storage.Same(args[0], args[1])
		if proof.State != ssaflow.EvidenceUnknown || proof.Reason != ssaflow.EvidenceBudgetExhausted || observed != 1 {
			t.Fatalf("cutoff must remain observed unknown: %+v, observed %d", proof, observed)
		}
		fresh := NewStorage(ssaflow.NewSearchBudget(ssaflow.QueryBudget))
		fresh.writesOnly = writesOnly
		if !fresh.Same(args[0], args[1]).Proven() || fresh.Budget().Exhausted() {
			t.Fatal("fresh structural equality must retain ordinary and writes-only policy")
		}
	}
	zero := NewStorage(ssaflow.NewSearchBudget(0))
	if proof := zero.Same(args[0], args[0]); proof.Proven() || proof.Reason != ssaflow.EvidenceBudgetExhausted {
		t.Fatal("direct same-value evidence must spend the storage allowance")
	}
	pool := ssaflow.NewSearchBudget(1)
	shared := pool.Within(ssaflow.QueryBudget)
	if proof := NewStorage(shared).Same(args[0], args[1]); proof.Proven() || !shared.PoolExhausted() {
		t.Fatal("storage structural queries must retain candidate-pool availability")
	}
}

func TestStorageContentCutoffBeforeGraphFallback(t *testing.T) {
	pkg := ssaflowtest.BuildPackage(t, "content", `package content
 func observe(a,b *int) {}
 func read(p **int) { _ = *p }
 func probe(a *int) { x:=a; read(&x); observe(x,a) }
`)
	call := heapObservation(t, pkg.Func("probe"))
	load, ok := call.Common().Args[0].(*ssa.UnOp)
	if !ok {
		t.Fatal("expected actual captured/addressable cell load")
	}
	cutoff := NewStorage(ssaflow.NewSearchBudget(1))
	if proof := cutoff.Content(load.X, load); proof.Proven() || proof.Reason != ssaflow.EvidenceBudgetExhausted {
		t.Fatalf("graph fallback must not rescue cutoff: %+v", proof)
	}
	fresh := NewStorage(ssaflow.NewSearchBudget(ssaflow.QueryBudget))
	if proof := fresh.Content(load.X, load); !proof.Proven() || proof.Value != call.Common().Args[1] || fresh.Budget().Exhausted() {
		t.Fatalf("fresh reaching-write query lost exact contents: %+v", proof)
	}
	// Exhaustion stays authoritative even when a graph was already warmed.
	if !DefinitelySame(load, call.Common().Args[1]) {
		t.Fatal("expected exact identity from the warmed function graph")
	}
	cutoff = NewStorage(ssaflow.NewSearchBudget(1))
	if proof := cutoff.Content(load.X, load); proof.Proven() || proof.Reason != ssaflow.EvidenceBudgetExhausted {
		t.Fatal("cached graph evidence must not override caller availability")
	}
}
