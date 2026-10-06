package summaries

import (
	"testing"

	"github.com/kojah/gohawk/internal/analysis/passes/lifecyclefacts"
	proofs "github.com/kojah/gohawk/internal/engine/proof"
	"github.com/kojah/gohawk/internal/engine/ssaflow"
	"github.com/kojah/gohawk/internal/engine/ssaflow/ssaflowtest"
	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/ssa"
)

func TestReturnedViewAvailability(t *testing.T) {
	pkg := ssaflowtest.BuildPackage(t, "viewbroker", `package viewbroker
func view(*int) *int
func caller(p *int) *int { return view(p) }
`)
	fn := pkg.Func("caller")
	call := ssaflow.InstructionsOf[*ssa.Call](fn)[0]
	for _, test := range []struct {
		name      string
		requested bool
		facts     lifecyclefacts.Summaries
		limit     int
		state     proofs.EvidenceState
		reason    proofs.EvidenceReason
	}{
		{"unrequested", false, nil, proofs.SummaryBudget, proofs.EvidenceUnknown, proofs.EvidenceUnavailable},
		{"missing", true, nil, proofs.SummaryBudget, proofs.EvidenceUnknown, proofs.EvidenceUnavailable},
		{
			"empty", true,
			lifecyclefacts.Summaries{pkg.Func("view"): {}},
			proofs.SummaryBudget,
			proofs.EvidenceDisproven, proofs.EvidenceNotFound,
		},
		{
			"view", true,
			lifecyclefacts.Summaries{pkg.Func("view"): {Must: lifecyclefacts.MustClaims{ReturnedView: 1}}},
			proofs.SummaryBudget, proofs.EvidenceProven, proofs.EvidenceStructuralWalk,
		},
		{"cutoff", true, nil, 0, proofs.EvidenceUnknown, proofs.EvidenceBudgetExhausted},
	} {
		t.Run(test.name, func(t *testing.T) {
			pass := &analysis.Pass{ResultOf: map[*analysis.Analyzer]any{lifecyclefacts.Analyzer: test.facts}}
			provider := Select(Requirements{Lifecycle: test.requested}).Provider(pass)
			got := provider.ProveCallReturnsViewWithin(call, fn.Params[0], proofs.NewSearchBudget(test.limit))
			if got.State != test.state || got.Reason != test.reason {
				t.Fatalf("view binding = %+v, want %v/%v", got, test.state, test.reason)
			}
		})
	}
}
