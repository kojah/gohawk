package resourcelifetime

import (
	"testing"

	"github.com/kojah/gohawk/internal/heapmodel"
	"github.com/kojah/gohawk/internal/passes/lifecyclefacts"
	"github.com/kojah/gohawk/internal/ssaflow"
	"github.com/kojah/gohawk/internal/ssaflow/ssaflowtest"
	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/ssa"
)

func TestAggregateRetainedPathAllowance(t *testing.T) {
	pkg := ssaflowtest.BuildPackage(t, "aggregatepath", `package aggregatepath
type resource struct { n int }
type holder struct { first, second *resource }
func retainFirst(*holder)
func caller(p, other *resource) { retainFirst(&holder{p,other}) }
func sibling(p, other *resource) { retainFirst(&holder{other,p}) }
`)
	fact := lifecyclefacts.Fact{Heap: &heapmodel.HeapSummary{
		Version: heapmodel.SummaryVersion,
		Effects: []heapmodel.HeapEffect{{
			Slot:   heapmodel.HeapSlot{Root: heapmodel.HeapRoot{Kind: heapmodel.HeapParameter}, Path: "field:0"},
			Escape: heapmodel.HeapEscapedGlobal,
		}},
	}}
	fact.DescribeFact(pkg.Func("retainFirst").Object())
	pass := &analysis.Pass{ResultOf: map[*analysis.Analyzer]any{
		lifecyclefacts.Analyzer: lifecyclefacts.Summaries{pkg.Func("retainFirst"): fact},
	}}
	provider := resourceSummaries.Provider(pass)
	for _, name := range []string{"caller", "sibling"} {
		t.Run(name, func(t *testing.T) {
			fn := pkg.Func(name)
			query := callbackAnalysis(fn, provider)
			call := ssaflow.InstructionsOf[*ssa.Call](fn)[0]
			checkResourceProofAllowance(t, func(budget *ssaflow.SearchBudget) resourceProof {
				return query.proveAggregateOwnerEscapeWithin(call, call.Common(), budget)
			}, name == "caller")
			checkResourceProofAllowance(t, func(budget *ssaflow.SearchBudget) resourceProof {
				return query.proveAggregateContentsEscapeWithin(call, 0, call.Common().Args[0], budget)
			}, name == "caller")
			pool := ssaflow.NewSearchBudget(ssaflow.SummaryBudget)
			proof := query.proveAggregateContentsEscapeWithin(call, 0, call.Common().Args[0], pool.Within(2))
			if proof.State != ssaflow.EvidenceUnknown || proof.Reason != resourceReasonBudgetExhausted || resourceFlowExhausted(pool) {
				t.Fatalf("contents path child cutoff = %+v, parent exhausted %v", proof, resourceFlowExhausted(pool))
			}
			query.pool = ssaflow.NewSearchBudget(0)
			if reason, opaque := query.opaqueFunctionCall(call, call.Common(), true); !opaque || reason != resourceReasonBudgetExhausted {
				t.Fatalf("interrupted path classifier = %v/%v", reason, opaque)
			}
			query.pool = ssaflow.NewSearchBudget(ssaflow.SummaryBudget)
			reason, opaque := query.opaqueFunctionCall(call, call.Common(), true)
			if opaque != (name == "caller") || opaque && reason != resourceReasonAggregateOwnerMayEscape {
				t.Fatalf("fresh path classifier = %v/%v", reason, opaque)
			}
		})
	}
}
