package goroutineownership

import (
	"github.com/kojah/gohawk/internal/passes/concurrencyfacts"
	"github.com/kojah/gohawk/internal/passes/lifecyclefacts"
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
	engine, _ := summaryKnowledge.Provider(analysis.pass).Concurrency()
	budget := ssaflow.NewSearchBudget(helperUseBudget)
	for _, tracked := range analysis.tracked {
		if tracked.kind == trackedGroup && analysis.returnedGroupJoin(instruction, tracked.value, budget) {
			return true
		}
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

// A returned waiter can honor an already-established group obligation. A
// shutdown callback is not a join: only Wait on this exact settling group
// counts, and launching the waiter asynchronously never joins the parent.
func (analysis *spawnAnalysis) returnedGroupJoin(instruction ssa.Instruction, target ssa.Value, budget *ssaflow.SearchBudget) bool {
	common := ssaflow.InstructionCall(instruction)
	if common == nil || common.StaticCallee() != nil || common.IsInvoke() {
		return false
	}
	request := ssaflow.CompletionRequest{
		Instruction: instruction, Target: target, Methods: []string{"Wait"}, ExactTarget: true, Budget: budget,
	}
	evidence, _ := summaryKnowledge.Provider(analysis.pass).LifecycleEvidence("goroutineownership", string(analysis.checkID))
	evidence.ForCandidate(analysis.spawn.Pos())
	return evidence.Prove(lifecyclefacts.EvidenceRequest{
		Instruction: instruction, Target: target, Completion: &request,
	}).Proven()
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
	if !summary.Complete() {
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
