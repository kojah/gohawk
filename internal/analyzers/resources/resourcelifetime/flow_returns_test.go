package resourcelifetime

import (
	"testing"

	"github.com/kojah/gohawk/internal/ssaflow"
	"github.com/kojah/gohawk/internal/ssaflow/ssaflowtest"
	"golang.org/x/tools/go/ssa"
)

func TestResourceReturnAllowance(t *testing.T) {
	pkg := ssaflowtest.BuildPackage(t, "returnbudget", `package returnbudget
type resource struct{}
func (*resource) Close() {}
type owner struct { value *resource }
func direct(value, other *resource) *resource { return value }
func nested(value, other *resource) *owner { return &owner{value} }
func unrelated(value, other *resource) *resource { return other }
func opaque(*resource) *resource
func derived(value, other *resource) *resource { return opaque(value) }
func scalar(*resource) int
func scalarResult(value, other *resource) int { return scalar(value) }
`)
	for _, test := range []struct {
		name  string
		owner bool
		want  ssaflow.EvidenceState
	}{
		{"direct", false, ssaflow.EvidenceDisproven},
		{"nested", false, ssaflow.EvidenceDisproven},
		{"unrelated", false, ssaflow.EvidenceProven},
		{"unrelated", true, ssaflow.EvidenceDisproven},
		{"derived", false, ssaflow.EvidenceDisproven},
		{"scalarResult", false, ssaflow.EvidenceProven},
	} {
		t.Run(test.name, func(t *testing.T) {
			fn := pkg.Func(test.name)
			returned := ssaflow.InstructionsOf[*ssa.Return](fn)[0]
			provider := resourceSummaries.Provider(nil)
			evidence, _ := provider.LifecycleEvidence("resourcelifetime", "resourcelifetime/missing-release")
			analysis := &resourceAnalysis{
				function: fn, resource: fn.Params[0], summaries: provider, evidence: evidence,
				contract: resourceContract{cleanup: []string{"Close"}},
			}
			if test.owner {
				analysis.owners = []ssa.Value{fn.Params[1]}
			}
			completed := false
			for limit := 0; limit <= ssaflow.SummaryBudget; limit++ {
				analysis.pool = ssaflow.NewSearchBudget(limit)
				budget := analysis.budget(ssaflow.SummaryBudget)
				got := analysis.proveResourceReturn(returned, budget)
				if resourceFlowExhausted(budget) {
					if got.state != ssaflow.EvidenceUnknown || got.reason != resourceReasonBudgetExhausted || got.leak != nil {
						t.Fatalf("allowance %d admitted interrupted return: %+v", limit, got)
					}
					continue
				}
				if limit == 0 || got.state != test.want || (got.leak != nil) != (test.want == ssaflow.EvidenceProven) {
					t.Fatalf("complete return proof = %+v at allowance %d", got, limit)
				}
				completed = true
				break
			}
			if !completed {
				t.Fatal("return proof never completed")
			}
		})
	}
}
