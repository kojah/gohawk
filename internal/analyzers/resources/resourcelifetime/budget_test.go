package resourcelifetime

import (
	"go/token"
	"strings"
	"testing"

	"github.com/kojah/gohawk/internal/lifecycle"
	"github.com/kojah/gohawk/internal/passes/lifecyclefacts"
	proofs "github.com/kojah/gohawk/internal/proof"
	"github.com/kojah/gohawk/internal/ssaflow"
	"github.com/kojah/gohawk/internal/ssaflow/ssaflowtest"
	"golang.org/x/tools/go/ssa"
)

func TestHelperCompletionUsesCandidatePool(t *testing.T) {
	analysis, call := helperBudgetAnalysis(t, "value.Close()")
	type observation struct {
		reason  string
		at      token.Pos
		details map[string]string
	}
	var observations []observation
	analysis.pool = proofs.NewSearchBudget(0).Observed(func(reason string, at token.Pos, details map[string]string) {
		observations = append(observations, observation{reason, at, details})
	})
	action, reason := analysis.classify(call)
	if action != actionUnknown || reason != resourceReasonBudgetExhausted {
		t.Fatalf("classification = %v/%v, want exhausted unknown; observations = %+v", action, reason, observations)
	}
	found := 0
	for _, observation := range observations {
		if observation.reason == "budget-exhausted" && observation.at == call.Pos() && observation.details["methods"] == "Close" {
			found++
		}
	}
	if found != 1 {
		t.Fatalf("want one candidate-observed completion exhaustion, got %d: %+v", found, observations)
	}
}

func TestHelperCompletionKeepsLargerQueryAllowance(t *testing.T) {
	body := "n := 0\n" + strings.Repeat("n = step(n)\n", proofs.QueryBudget+1) + "println(n)\nvalue.Close()"
	analysis, call := helperBudgetAnalysis(t, body)
	callee := call.Common().StaticCallee()
	if len(callee.Blocks[0].Instrs) <= proofs.QueryBudget {
		t.Fatal("fixture must exceed the storage query limit in actual SSA")
	}
	if action, reason := analysis.classify(call); action != actionSettled || reason != resourceReasonSettled {
		t.Fatalf("large helper classification = %v/%v, want exact cleanup", action, reason)
	}
}

func helperBudgetAnalysis(t *testing.T, body string) (*resourceAnalysis, *ssa.Call) {
	t.Helper()
	pkg := ssaflowtest.BuildPackage(t, "candidatebudget", `package candidatebudget
type resource struct{}
func (*resource) Close() {}
func step(n int) int { return n + 1 }
func closeResource(value *resource) { `+body+` }
func acquire() *resource { return new(resource) }
func caller() { value := acquire(); closeResource(value) }
`)
	function := pkg.Func("caller")
	calls := ssaflow.InstructionsOf[*ssa.Call](function)
	acquisition, call := calls[0], calls[1]
	provider := resourceSummaries.Provider(nil)
	evidence, _ := provider.LifecycleEvidence("resourcelifetime", "resourcelifetime/missing-release")
	return &resourceAnalysis{
		function: function, acquisition: acquisition, resource: acquisition, evidence: evidence, summaries: provider,
		contract: resourceContract{cleanup: []string{"Close"}},
	}, call
}

func TestExhaustedCleanupDoesNotProveSettlement(t *testing.T) {
	pkg := ssaflowtest.BuildPackage(t, "cleanupbudget", `package cleanupbudget
type resource struct{}
func (*resource) Close() {}
func closeResource(value *resource) { value.Close() }
func maybeClose(value *resource, clean bool) { if clean { value.Close() } }
func loopClose(value *resource, run bool) { for run { value.Close() } }
func caller(value *resource) { closeResource(value) }
func conditionalCaller(value *resource, clean bool) { maybeClose(value, clean) }
func loopCaller(value *resource, run bool) { loopClose(value, run) }
`)
	for _, test := range []struct {
		name     string
		function string
		budget   int
		state    proofs.EvidenceState
		reason   proofs.EvidenceReason
		action   resourceAction
		label    resourceLifetimeReason
	}{
		{"exhausted", "caller", 1, proofs.EvidenceUnknown, proofs.EvidenceBudgetExhausted, actionUnknown, resourceReasonBudgetExhausted},
		{"completed", "caller", proofs.QueryBudget, proofs.EvidenceProven, proofs.EvidenceNone, actionSettled, resourceReasonSettled},
		{
			"conditional", "conditionalCaller", proofs.QueryBudget, proofs.EvidenceDisproven,
			proofs.EvidenceNotFound, actionNone, resourceReasonNone,
		},
		{
			"loop", "loopCaller", proofs.QueryBudget, proofs.EvidenceUnknown,
			proofs.EvidenceCompletionInCycle, actionUnknown, resourceReasonHelperCleanupInLoop,
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			function := pkg.Func(test.function)
			call := ssaflow.InstructionsOf[*ssa.Call](function)[0]
			completion := lifecycle.ProveCompletion(lifecycle.CompletionRequest{
				Instruction: call, Target: function.Params[0], Methods: []string{"Close"},
				Budget: proofs.NewSearchBudget(test.budget),
			})
			if completion.State != test.state || test.reason != proofs.EvidenceNone && completion.Reason != test.reason {
				t.Fatalf("completion = %+v, want state %v and reason %v", completion, test.state, test.reason)
			}
			if action, reason := releaseLabel(lifecyclefacts.Proof{Proof: completion.Proof}); action != test.action || reason != test.label {
				t.Fatalf("resource label = %v/%v, want %v/%v for %+v", action, reason, test.action, test.label, completion)
			}
		})
	}
}

// Candidate budget adapters share a pool while retaining each query's limit.
// This checks the adapters; it does not establish that every caller uses them.
func TestCandidateQueriesShareOnePool(t *testing.T) {
	analysis := &resourceAnalysis{}
	first := analysis.budget(1)
	if !first.Spend() || first.Spend() {
		t.Fatal("a query keeps its own limit")
	}
	second := analysis.budget(resourcePoolBudget)
	for range resourcePoolBudget - 1 {
		if !second.Spend() {
			t.Fatal("the pool had allowance left")
		}
	}
	if second.Spend() || !second.PoolExhausted() {
		t.Fatal("the pool, not the query, should have run out")
	}
}
