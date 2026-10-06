package resourcelifetime

import (
	"strings"
	"testing"

	"github.com/kojah/gohawk/internal/analysis/passes/lifecyclefacts"
	"github.com/kojah/gohawk/internal/engine/heapmodel"
	proofs "github.com/kojah/gohawk/internal/engine/proof"
	"github.com/kojah/gohawk/internal/engine/ssaflow"
	"github.com/kojah/gohawk/internal/engine/ssaflow/ssaflowtest"
	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/ssa"
)

func TestAsynchronousExposureAllowance(t *testing.T) {
	pkg := ssaflowtest.BuildPackage(t, "asyncallowance", `package asyncallowance
type resource struct { n int }
func read(p *resource) { _ = p.n }
func async(p *resource) { go read(p) }
func imported(*resource)
func opaque(*resource)
func observed(p, other *resource) { read(p) }
func launched(p, other *resource) { async(p) }
func exported(p, other *resource) { imported(p) }
func unrelated(p, other *resource) { imported(other) }
func unreadable(p, other *resource) { opaque(p) }
`)
	fact := lifecyclefacts.Fact{Heap: &heapmodel.HeapSummary{
		Version: heapmodel.SummaryVersion,
		Effects: []heapmodel.HeapEffect{{
			Slot: heapmodel.HeapSlot{Root: heapmodel.HeapRoot{Kind: heapmodel.HeapParameter}}, Escape: heapmodel.HeapEscapedAsync,
		}},
	}}
	fact.DescribeFact(pkg.Func("imported").Object())
	pass := &analysis.Pass{ResultOf: map[*analysis.Analyzer]any{
		lifecyclefacts.Analyzer: lifecyclefacts.Summaries{pkg.Func("imported"): fact},
	}}
	provider := resourceSummaries.Provider(pass)
	for _, test := range []struct {
		name string
		want bool
	}{{"observed", false}, {"launched", true}, {"exported", true}, {"unrelated", false}, {"unreadable", false}} {
		t.Run(test.name, func(t *testing.T) {
			fn := pkg.Func(test.name)
			query := callbackAnalysis(fn, provider)
			call := ssaflow.InstructionsOf[*ssa.Call](fn)[0]
			checkResourceProofAllowance(t, func(budget *proofs.SearchBudget) resourceProof {
				return query.proveAsynchronousExposureWithin(call, call.Common(), budget)
			}, test.want)
		})
	}
}

func TestAsynchronousEffectChildCutoff(t *testing.T) {
	pkg := asyncExposureCapFixture(t)
	fn := pkg.Func("caller")
	call := ssaflow.InstructionsOf[*ssa.Call](fn)[0]
	query := aggregateEscapeAnalysis(fn)
	budget := proofs.NewSearchBudget(10 * proofs.SummaryBudget)
	proof := query.proveAsynchronousExposureWithin(call, call.Common(), budget)
	if proof.State != proofs.EvidenceUnknown || proof.Reason != resourceReasonBudgetExhausted || resourceFlowExhausted(budget) {
		t.Fatalf("effect child cutoff = %+v, caller exhausted %v", proof, resourceFlowExhausted(budget))
	}
}

func TestAsynchronousExposureClassifierCutoff(t *testing.T) {
	pkg := asyncExposureCapFixture(t)
	fn := pkg.Func("caller")
	call := ssaflow.InstructionsOf[*ssa.Call](fn)[0]
	query := aggregateEscapeAnalysis(fn)
	query.pool = proofs.NewSearchBudget(10 * proofs.SummaryBudget)
	if reason, opaque := query.opaqueCall(call, call.Common()); !opaque || reason != resourceReasonBudgetExhausted {
		t.Fatalf("effect child-cutoff classifier = %v/%v", reason, opaque)
	}
	if resourceFlowExhausted(query.pool) {
		t.Fatal("effect child cutoff exhausted candidate pool")
	}
	short := pkg.Func("short")
	borrow := ssaflow.InstructionsOf[*ssa.Call](short)[0]
	if reason, opaque := aggregateEscapeAnalysis(short).opaqueCall(borrow, borrow.Common()); opaque {
		t.Fatalf("fresh borrowing classifier = %v/%v", reason, opaque)
	}
}

func asyncExposureCapFixture(t *testing.T) *ssa.Package {
	t.Helper()
	source := `package asynccap
type resource struct { n int }
func read(p *resource) { _ = p.n }
func many(p *resource) {
` + strings.Repeat("read(p)\n", proofs.QueryBudget+100) +
		"}\nfunc caller(p *resource) { many(p) }\nfunc short(p *resource) { read(p) }"
	return ssaflowtest.BuildPackage(t, "asynccap", source)
}
