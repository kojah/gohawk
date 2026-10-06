package resourcelifetime

import (
	"testing"

	proofs "github.com/kojah/gohawk/internal/engine/proof"
	"github.com/kojah/gohawk/internal/engine/resourcemodel"
	"github.com/kojah/gohawk/internal/engine/ssaflow"
	cfg "github.com/kojah/gohawk/internal/engine/ssaflow/cfg"
	ssapath "github.com/kojah/gohawk/internal/engine/ssaflow/path"
	"github.com/kojah/gohawk/internal/engine/ssaflow/ssaflowtest"
	"golang.org/x/tools/go/ssa"
)

func TestResourceKeyKeepsObligationAtSameLocation(t *testing.T) {
	analysis, _ := helperBudgetAnalysis(t, "println(value)")
	block := analysis.acquisition.Block()
	obligation := resourcemodel.Acquired()
	states := []resourceFlowState{
		{block: block, obligation: obligation},
		{block: block, obligation: obligation.Discharged()},
		{block: block, obligation: obligation.Uncertain()},
		{block: block, obligation: obligation.Absent()},
	}
	expanded := 0
	cfg.WalkStates(states, func(state resourceFlowState) resourceFlowKey { return resourceStateKey(state, nil) },
		func(resourceFlowState) ([]resourceFlowState, bool) { expanded++; return nil, true })
	if expanded != len(states) {
		t.Fatalf("obligation collapsed at one guarded position: expanded=%d", expanded)
	}
}

func TestResourceFlowCandidateAllowance(t *testing.T) {
	for _, test := range []struct {
		body string
		want proofs.EvidenceState
	}{{"value.Close()", proofs.EvidenceDisproven}, {"println(value)", proofs.EvidenceProven}} {
		t.Run(test.body, func(t *testing.T) {
			analysis, _ := helperBudgetAnalysis(t, test.body)
			analysis.actions = make(map[ssa.Instruction]resourceAction)
			for _, block := range analysis.function.Blocks {
				for _, instruction := range block.Instrs {
					t.Log(instruction.String())
				}
			}
			got := analysis.proveResourceFlow(nil)
			if got.state != test.want || analysis.pool.Exhausted() || (got.leak != nil) != (test.want == proofs.EvidenceProven) {
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
		want proofs.EvidenceState
	}{{"leaky", proofs.EvidenceProven}, {"closed", proofs.EvidenceDisproven}, {"presence", proofs.EvidenceDisproven}} {
		function := pkg.Func(test.name)
		acquisition := ssaflow.InstructionsOf[*ssa.Call](function)[0]
		if test.name != "presence" && len(ssapath.GuardsDominatingWithin(acquisition, nil)) == 0 {
			t.Fatal("fixture must establish a dominating guard in actual SSA")
		}
		completed := false
		for limit := 0; limit <= proofs.SummaryBudget; limit++ {
			provider := resourceSummaries.Provider(nil)
			evidence, _ := provider.LifecycleEvidence("resourcelifetime", "resourcelifetime/missing-release")
			analysis := &resourceAnalysis{
				function: function, acquisition: acquisition, resource: acquisition,
				evidence: evidence, summaries: provider, contract: resourceContract{cleanup: []string{"Close"}},
				actions: make(map[ssa.Instruction]resourceAction), pool: proofs.NewSearchBudget(limit),
			}
			got := analysis.proveResourceFlow(nil)
			if !analysis.pool.Exhausted() {
				if got.state != test.want {
					t.Fatalf("guarded complete proof = %+v", got)
				}
				completed = true
				break
			}
			if got.state != proofs.EvidenceUnknown || got.reason != resourceReasonBudgetExhausted || got.leak != nil {
				t.Fatalf("guarded cutoff admitted proof: %+v", got)
			}
		}
		if !completed {
			t.Fatal("guarded proof never completed")
		}
	}
}

func checkResourceFlowCutoffs(t *testing.T, body string, want proofs.EvidenceState) {
	t.Helper()
	completed := false
	for limit := 0; limit <= proofs.SummaryBudget; limit++ {
		analysis, _ := helperBudgetAnalysis(t, body)
		analysis.actions = make(map[ssa.Instruction]resourceAction)
		analysis.pool = proofs.NewSearchBudget(limit)
		got := analysis.proveResourceFlow(nil)
		if !analysis.pool.Exhausted() {
			if got.state != want {
				t.Fatalf("completed flow = %+v, want %v", got, want)
			}
			completed = true
			break
		}
		if got.state != proofs.EvidenceUnknown || got.reason != resourceReasonBudgetExhausted || got.leak != nil || analysis.leak != nil {
			t.Fatalf("allowance %d admitted incomplete flow: %+v", limit, got)
		}
	}
	if !completed {
		t.Fatal("flow never completed")
	}
}
