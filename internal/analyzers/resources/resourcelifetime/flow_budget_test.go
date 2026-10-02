package resourcelifetime

import (
	"testing"

	"github.com/kojah/gohawk/internal/ssaflow"
	"github.com/kojah/gohawk/internal/ssaflow/ssaflowtest"
	"golang.org/x/tools/go/ssa"
)

func TestResourceFlowCandidateAllowance(t *testing.T) {
	for _, test := range []struct {
		body string
		want ssaflow.EvidenceState
	}{{"value.Close()", ssaflow.EvidenceDisproven}, {"println(value)", ssaflow.EvidenceProven}} {
		t.Run(test.body, func(t *testing.T) {
			analysis, _ := helperBudgetAnalysis(t, test.body)
			analysis.actions = make(map[ssa.Instruction]resourceAction)
			for _, block := range analysis.function.Blocks {
				for _, instruction := range block.Instrs {
					t.Log(instruction.String())
				}
			}
			got := analysis.proveResourceFlow(nil)
			if got.state != test.want || analysis.pool.Exhausted() || (got.leak != nil) != (test.want == ssaflow.EvidenceProven) {
				t.Fatalf("fresh flow = %+v, want state %v", got, test.want)
			}
			checkResourceFlowCutoffs(t, test.body, test.want)
		})
	}
}

func TestResourceFlowGuardedAllowance(t *testing.T) {
	pkg := ssaflowtest.BuildPackage(t, "guardbudget", `package guardbudget
type resource struct{}
func (*resource) Close() {}
func acquire() *resource { return new(resource) }
func leaky(flag bool) { if !flag { return }; value := acquire(); println(value) }
func closed(flag bool) { if !flag { return }; value := acquire(); value.Close() }
func presence(flag bool) { value := acquire(); if value != nil { value.Close() } }
`)
	for _, test := range []struct {
		name string
		want ssaflow.EvidenceState
	}{{"leaky", ssaflow.EvidenceProven}, {"closed", ssaflow.EvidenceDisproven}, {"presence", ssaflow.EvidenceDisproven}} {
		function := pkg.Func(test.name)
		acquisition := ssaflow.InstructionsOf[*ssa.Call](function)[0]
		if test.name != "presence" && len(ssaflow.GuardsDominatingWithin(acquisition, nil)) == 0 {
			t.Fatal("fixture must establish a dominating guard in actual SSA")
		}
		completed := false
		for limit := 0; limit <= ssaflow.SummaryBudget; limit++ {
			provider := resourceSummaries.Provider(nil)
			evidence, _ := provider.LifecycleEvidence("resourcelifetime", "resourcelifetime/missing-release")
			analysis := &resourceAnalysis{
				function: function, acquisition: acquisition, resource: acquisition,
				evidence: evidence, summaries: provider, contract: resourceContract{cleanup: []string{"Close"}},
				actions: make(map[ssa.Instruction]resourceAction), pool: ssaflow.NewSearchBudget(limit),
			}
			got := analysis.proveResourceFlow(nil)
			if !analysis.pool.Exhausted() {
				if got.state != test.want {
					t.Fatalf("guarded complete proof = %+v", got)
				}
				completed = true
				break
			}
			if got.state != ssaflow.EvidenceUnknown || got.reason != resourceReasonBudgetExhausted || got.leak != nil {
				t.Fatalf("guarded cutoff admitted proof: %+v", got)
			}
		}
		if !completed {
			t.Fatal("guarded proof never completed")
		}
	}
}

func checkResourceFlowCutoffs(t *testing.T, body string, want ssaflow.EvidenceState) {
	t.Helper()
	completed := false
	for limit := 0; limit <= ssaflow.SummaryBudget; limit++ {
		analysis, _ := helperBudgetAnalysis(t, body)
		analysis.actions = make(map[ssa.Instruction]resourceAction)
		analysis.pool = ssaflow.NewSearchBudget(limit)
		got := analysis.proveResourceFlow(nil)
		if !analysis.pool.Exhausted() {
			if got.state != want {
				t.Fatalf("completed flow = %+v, want %v", got, want)
			}
			completed = true
			break
		}
		if got.state != ssaflow.EvidenceUnknown || got.reason != resourceReasonBudgetExhausted || got.leak != nil || analysis.leak != nil {
			t.Fatalf("allowance %d admitted incomplete flow: %+v", limit, got)
		}
	}
	if !completed {
		t.Fatal("flow never completed")
	}
}
