package summaries

import (
	"testing"

	"github.com/kojah/gohawk/internal/passes/lifecyclefacts"
	"github.com/kojah/gohawk/internal/ssaflow"
	"github.com/kojah/gohawk/internal/ssaflow/ssaflowtest"
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
		state     ssaflow.EvidenceState
		reason    ssaflow.EvidenceReason
	}{
		{"unrequested", false, nil, ssaflow.SummaryBudget, ssaflow.EvidenceUnknown, ssaflow.EvidenceUnavailable},
		{"missing", true, nil, ssaflow.SummaryBudget, ssaflow.EvidenceUnknown, ssaflow.EvidenceUnavailable},
		{
			"empty", true,
			lifecyclefacts.Summaries{pkg.Func("view"): {}},
			ssaflow.SummaryBudget,
			ssaflow.EvidenceDisproven, ssaflow.EvidenceNotFound,
		},
		{
			"view", true,
			lifecyclefacts.Summaries{pkg.Func("view"): {Must: lifecyclefacts.MustClaims{ReturnedView: 1}}},
			ssaflow.SummaryBudget, ssaflow.EvidenceProven, ssaflow.EvidenceStructuralWalk,
		},
		{"cutoff", true, nil, 0, ssaflow.EvidenceUnknown, ssaflow.EvidenceBudgetExhausted},
	} {
		t.Run(test.name, func(t *testing.T) {
			pass := &analysis.Pass{ResultOf: map[*analysis.Analyzer]any{lifecyclefacts.Analyzer: test.facts}}
			provider := Select(Requirements{Lifecycle: test.requested}).Provider(pass)
			got := provider.ProveCallReturnsViewWithin(call, fn.Params[0], ssaflow.NewSearchBudget(test.limit))
			if got.State != test.state || got.Reason != test.reason {
				t.Fatalf("view binding = %+v, want %v/%v", got, test.state, test.reason)
			}
		})
	}
}
