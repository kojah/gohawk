package resourcelifetime

import (
	"testing"

	proofs "github.com/kojah/gohawk/internal/engine/proof"
	"github.com/kojah/gohawk/internal/engine/ssaflow"
	"github.com/kojah/gohawk/internal/engine/ssaflow/ssaflowtest"
	"golang.org/x/tools/go/ssa"
)

func TestAggregateOwnerEscapeAllowance(t *testing.T) {
	pkg := ssaflowtest.BuildPackage(t, "aggregateescape", `package aggregateescape
type resource struct{}
type holder struct { value *resource }
var kept *holder
func retain(h *holder) { kept = h }
func inspect(h *holder) { _ = h.value }
func opaque(*holder)
func use(any)
func held(p, other *resource) { retain(&holder{p}) }
func borrowed(p, other *resource) { inspect(&holder{p}) }
func unrelated(p, other *resource) { retain(&holder{other}) }
func unreadable(p, other *resource) { opaque(&holder{p}) }
func direct(p, other *resource) { use(p) }
func callback(p, other *resource) { use(func(){ println(p) }) }
`)
	for _, test := range []struct {
		name string
		want bool
	}{{"held", true}, {"borrowed", false}, {"unrelated", false}, {"unreadable", true}, {"direct", false}, {"callback", false}} {
		t.Run(test.name, func(t *testing.T) {
			fn := pkg.Func(test.name)
			call := ssaflow.InstructionsOf[*ssa.Call](fn)[0]
			completed := false
			for limit := 0; limit <= proofs.SummaryBudget; limit++ {
				query := aggregateEscapeAnalysis(fn)
				query.pool = proofs.NewSearchBudget(limit)
				budget := query.budget(proofs.SummaryBudget)
				got := query.proveAggregateOwnerEscapeWithin(call, call.Common(), budget)
				if resourceFlowExhausted(budget) {
					if got.State != proofs.EvidenceUnknown || got.Reason != resourceReasonBudgetExhausted {
						t.Fatalf("allowance %d retained interrupted escape: %+v", limit, got)
					}
					continue
				}
				if got.State == proofs.EvidenceUnknown || got.Proven() != test.want {
					t.Fatalf("complete escape = %+v, want %v", got, test.want)
				}
				completed = true
				break
			}
			if !completed {
				t.Fatal("escape never completed")
			}
		})
	}
}

func TestAggregateEscapeClassifierCutoff(t *testing.T) {
	pkg := ssaflowtest.BuildPackage(t, "escapelabel", `package escapelabel
type resource struct{}
type holder struct { value *resource }
var kept *holder
func retain(h *holder) { kept = h }
func caller(p *resource) { retain(&holder{p}) }
`)
	fn := pkg.Func("caller")
	call := ssaflow.InstructionsOf[*ssa.Call](fn)[0]
	query := aggregateEscapeAnalysis(fn)
	query.pool = proofs.NewSearchBudget(0)
	if reason, opaque := query.opaqueFunctionCall(call, call.Common(), true); !opaque || reason != resourceReasonBudgetExhausted {
		t.Fatalf("interrupted classifier = %v/%v", reason, opaque)
	}
	query.pool = proofs.NewSearchBudget(proofs.SummaryBudget)
	if reason, opaque := query.opaqueFunctionCall(call, call.Common(), true); !opaque || reason != resourceReasonAggregateOwnerMayEscape {
		t.Fatalf("fresh classifier = %v/%v", reason, opaque)
	}
}

func aggregateEscapeAnalysis(fn *ssa.Function) *resourceAnalysis {
	provider := resourceSummaries.Provider(nil)
	evidence, _ := provider.LifecycleEvidence("resourcelifetime", "resourcelifetime/missing-release")
	return &resourceAnalysis{function: fn, resource: fn.Params[0], evidence: evidence, summaries: provider}
}
