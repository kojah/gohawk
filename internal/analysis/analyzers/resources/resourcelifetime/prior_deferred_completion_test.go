package resourcelifetime

import (
	"strings"
	"testing"

	proofs "github.com/kojah/gohawk/internal/engine/proof"
	"github.com/kojah/gohawk/internal/engine/ssaflow"
	"github.com/kojah/gohawk/internal/engine/ssaflow/ssaflowtest"
	"golang.org/x/tools/go/ssa"
)

func TestPriorDeferredCompletionAllowance(t *testing.T) {
	pkg := priorCleanupFixture(t)
	for _, test := range []struct {
		name string
		want bool
	}{
		{"captured", true}, {"unrelated", false}, {"byValue", false}, {"registered", false}, {"later", false}, {"leak", false},
	} {
		t.Run(test.name, func(t *testing.T) {
			query, call := priorCleanupAnalysis(t, pkg.Func(test.name))
			checkResourceProofAllowance(t, func(budget *proofs.SearchBudget) resourceProof {
				return query.proveDeferredBeforeAcquisitionWithin(call, budget)
			}, test.want)
		})
	}
}

func TestPriorDeferredCompletionPoolCutoff(t *testing.T) {
	pkg := ssaflowtest.BuildPackage(t, "priordeferred", `package priordeferred
 type resource struct{ n int }
 func (*resource) Close(){}
 func acquire()*resource{return &resource{}}
 func multiple(old,other *resource,yes bool){
  p:=old
  defer other.Close()
  defer func(){if yes {p.Close()}}()
  p=acquire()
 }
 `)
	query, call := priorCleanupAnalysis(t, pkg.Func("multiple"))
	pool := proofs.NewSearchBudget(resourcePoolBudget)
	got := query.proveDeferredBeforeAcquisitionWithin(call, pool.Within(2))
	if got.State != proofs.EvidenceUnknown || got.Reason != resourceReasonBudgetExhausted || pool.Exhausted() {
		t.Fatalf("prior-defer child cutoff = %+v; parent exhausted %v", got, pool.Exhausted())
	}
	fresh := query.proveDeferredBeforeAcquisitionWithin(call, pool.Within(releaseSearchBudget))
	if !fresh.Proven() || fresh.Reason != resourceReasonPriorDeferMayRelease {
		t.Fatalf("fresh prior-defer completion = %+v; SSA:\n%s", fresh, carriedSSA(t, call.Parent()))
	}
}

func TestPriorDeferredCompletionSharesNestedBudget(t *testing.T) {
	source := `package nesteddeferred
 type resource struct{ n int }
 func (*resource) Close(){}
 func acquire()*resource{return &resource{}}
 func noop(){}
 func pending(old *resource){p:=old;defer func(){
 ` + strings.Repeat("noop()\n", 50) + `p.Close()}();p=acquire()}
 `
	pkg := ssaflowtest.BuildPackage(t, "nesteddeferred", source)
	query, call := priorCleanupAnalysis(t, pkg.Func("pending"))
	// This allowance covers the entire caller census but not its deferred body.
	// A fresh independent completion allowance would incorrectly preserve a witness.
	limit := len(ssaflow.InstructionsOf[ssa.Instruction](call.Parent())) + 1
	pool := proofs.NewSearchBudget(resourcePoolBudget)
	got := query.proveDeferredBeforeAcquisitionWithin(call, pool.Within(limit))
	if got.State != proofs.EvidenceUnknown || got.Reason != resourceReasonBudgetExhausted || pool.Exhausted() {
		t.Fatalf("nested completion escaped allowance = %+v; SSA:\n%s", got, carriedSSA(t, call.Parent()))
	}
	if fresh := query.proveDeferredBeforeAcquisitionWithin(call, pool.Within(releaseSearchBudget)); !fresh.Proven() {
		t.Fatalf("fresh deferred completion = %+v", fresh)
	}
}
