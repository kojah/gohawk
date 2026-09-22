package goroutineownership

import (
	"github.com/kojah/gohawk/internal/passes/concurrencyfacts"
	"github.com/kojah/gohawk/internal/ssaflow"
	analysisTrace "github.com/kojah/gohawk/internal/trace"

	"golang.org/x/tools/go/ssa"
)

// Ordered effects are positive join evidence for obligations already found by
// this analyzer. They do not establish worker completion: an early Done or
// close can be a readiness signal instead. Missing effects remain opaque under
// the existing classifier, and its branch-aware local proofs remain available.
type summaryJoinProof struct {
	joined bool
	reason string
}

func (analysis *spawnAnalysis) summarizedJoin(instruction ssa.Instruction) bool {
	engine := analysis.pass.ResultOf[concurrencyfacts.Analyzer].(*concurrencyfacts.Engine)
	budget := ssaflow.NewSearchBudget(helperUseBudget)
	for _, tracked := range analysis.tracked {
		proof := proveSummaryJoin(engine, instruction, tracked.value, tracked.kind, budget)
		if proof.joined {
			probe := analysisTrace.For(analysis.pass, "goroutineownership", string(analysis.checkID), analysis.spawn.Pos())
			if probe.Enabled() {
				probe.Evidence(analysisTrace.Step{
					Reason: proof.reason, Outcome: analysisTrace.OutcomeAccepted, Pos: instruction.Pos(),
					Function: analysis.function.String(),
				})
			}
			return true
		}
	}
	return false
}

func proveSummaryJoin(
	engine *concurrencyfacts.Engine, instruction ssa.Instruction, target ssa.Value,
	kind trackedKind, budget *ssaflow.SearchBudget,
) summaryJoinProof {
	if engine == nil || kind == trackedOwner {
		return summaryJoinProof{reason: "concurrency-join-not-applicable"}
	}
	call, ok := instruction.(ssa.CallInstruction)
	if _, launched := instruction.(*ssa.Go); !ok || launched {
		return summaryJoinProof{reason: "concurrency-join-not-synchronous"}
	}
	summary := engine.AtCall(call, budget)
	if summary.Reason != "" {
		return summaryJoinProof{reason: summary.Reason}
	}
	want := concurrencyfacts.Receive
	if kind == trackedGroup {
		want = concurrencyfacts.GroupWait
	}
	storage := ssaflow.NewStorage(budget)
	for _, operation := range summary.Operations {
		if !budget.Spend() {
			return summaryJoinProof{reason: "concurrency-join-budget-exhausted"}
		}
		if operation.Kind == want && !operation.Resource.Indirect && storage.Same(operation.Resource.Value, target).Proven() {
			return summaryJoinProof{joined: true, reason: "concurrency-summary-join"}
		}
	}
	return summaryJoinProof{reason: "concurrency-summary-no-exact-join"}
}
