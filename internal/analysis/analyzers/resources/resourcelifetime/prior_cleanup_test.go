package resourcelifetime

import (
	"testing"

	proofs "github.com/kojah/gohawk/internal/engine/proof"
	"github.com/kojah/gohawk/internal/engine/ssaflow"
	"github.com/kojah/gohawk/internal/engine/ssaflow/ssaflowtest"
	"golang.org/x/tools/go/ssa"
)

func TestPriorCleanupAllowance(t *testing.T) {
	pkg := priorCleanupFixture(t)
	for _, test := range []struct {
		name string
		want bool
	}{{"captured", true}, {"registered", true}, {"unrelated", false}, {"byValue", false}, {"later", false}, {"leak", false}} {
		t.Run(test.name, func(t *testing.T) {
			query, call := priorCleanupAnalysis(t, pkg.Func(test.name))
			checkResourceProofAllowance(t, func(budget *proofs.SearchBudget) resourceProof {
				got := query.provePriorCleanupWithin(call, budget)
				if got.Proven() && got.Instruction == nil {
					t.Fatal("complete prior-cleanup witness has no instruction")
				}
				return got.resourceProof
			}, test.want)
		})
	}
}

func TestCapturedCellCleanupAllowance(t *testing.T) {
	pkg := priorCleanupFixture(t)
	for _, name := range []string{"captured", "unrelated", "byValue"} {
		t.Run(name, func(t *testing.T) {
			query, _ := priorCleanupAnalysis(t, pkg.Func(name))
			deferred := ssaflow.InstructionsOf[*ssa.Defer](query.function)[0]
			checkResourceProofAllowance(t, func(budget *proofs.SearchBudget) resourceProof {
				return query.proveCapturedCellCleanupWithin(deferred, budget)
			}, name == "captured")
		})
	}
}

func TestPriorCleanupChildCutoff(t *testing.T) {
	query, call := priorCleanupAnalysis(t, priorCleanupFixture(t).Func("captured"))
	pool := proofs.NewSearchBudget(resourcePoolBudget)
	got := query.provePriorCleanupWithin(call, pool.Within(2))
	if got.State != proofs.EvidenceUnknown || got.Reason != resourceReasonBudgetExhausted || got.Instruction != nil || pool.Exhausted() {
		t.Fatalf("interrupted prior registration = %+v; parent exhausted %v", got, pool.Exhausted())
	}
	if fresh := query.provePriorCleanupWithin(call, pool.Within(releaseSearchBudget)); !fresh.Proven() || fresh.Instruction == nil {
		t.Fatalf("fresh prior registration = %+v", fresh)
	}
}

func TestCapturedCellClassifierCutoff(t *testing.T) {
	query, _ := priorCleanupAnalysis(t, priorCleanupFixture(t).Func("captured"))
	deferred := ssaflow.InstructionsOf[*ssa.Defer](query.function)[0]
	closure := deferred.Common().Value.(*ssa.MakeClosure)
	query.pool = proofs.NewSearchBudget(0)
	if reason, opaque := query.opaqueClosureCall(deferred, closure, false); !opaque || reason != resourceReasonBudgetExhausted {
		t.Fatalf("interrupted captured cleanup classifier = %v/%v", reason, opaque)
	}
	query.pool = proofs.NewSearchBudget(resourcePoolBudget)
	if reason, opaque := query.opaqueClosureCall(deferred, closure, false); !opaque || reason != resourceReasonCapturedCellMayCleanup {
		t.Fatalf("fresh captured cleanup classifier = %v/%v", reason, opaque)
	}
}

func TestPriorCleanupFlow(t *testing.T) {
	pkg := priorCleanupFixture(t)
	for _, name := range []string{"captured", "registered", "leak", "released"} {
		t.Run(name, func(t *testing.T) {
			query, call := priorCleanupAnalysis(t, pkg.Func(name))
			got := evaluateResourceFlow(nil, query.evidence, call, call, query.contract)
			switch name {
			case "captured", "registered":
				if got.state != proofs.EvidenceUnknown || got.leak != nil {
					t.Fatalf("prior cleanup lost uncertainty = %+v", got)
				}
			case "leak":
				if got.state != proofs.EvidenceProven || got.leak == nil {
					t.Fatalf("leak witness lost = %+v", got)
				}
			case "released":
				if got.state != proofs.EvidenceDisproven || got.leak != nil {
					t.Fatalf("exact cleanup reported = %+v", got)
				}
			}
		})
	}
}

func priorCleanupAnalysis(t *testing.T, fn *ssa.Function) (*resourceAnalysis, *ssa.Call) {
	t.Helper()
	for _, call := range ssaflow.InstructionsOf[*ssa.Call](fn) {
		if ssaflow.CallName(call.Common()) == "acquire" {
			provider := resourceSummaries.Provider(nil)
			evidence, _ := provider.LifecycleEvidence("resourcelifetime", "resourcelifetime/missing-release")
			return &resourceAnalysis{
				function: fn, resource: call, summaries: provider, evidence: evidence,
				contract: resourceContract{cleanup: []string{"Close"}},
			}, call
		}
	}
	t.Fatal("missing acquisition")
	return nil, nil
}

func priorCleanupFixture(t *testing.T) *ssa.Package {
	t.Helper()
	return ssaflowtest.BuildPackage(t, "priorcleanup", `package priorcleanup
import "testing"
type resource struct { n int }
func (*resource) Close() {}
func acquire() *resource { return &resource{} }
func captured(old *resource, yes bool) { p := old; defer func(){ if yes { p.Close() } }(); p = acquire() }
func registered(t *testing.T, old *resource, yes bool) { p := old; t.Cleanup(func(){ if yes { p.Close() } }); p = acquire() }
func unrelated(old *resource) { defer func(){old.Close()}(); _ = acquire() }
func byValue(old *resource) { p := old; defer p.Close(); p = acquire() }
func later(t *testing.T) { p := acquire(); t.Cleanup(func(){p.Close()}) }
func leak() { _ = acquire() }
func released() { p := acquire(); p.Close() }
`)
}
