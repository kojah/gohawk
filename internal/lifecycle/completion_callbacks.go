package lifecycle

import (
	"github.com/kojah/gohawk/internal/ssaflow"
	"golang.org/x/tools/go/ssa"
)

// Callback invocation uses the ordinary completion engine's exact identity,
// binding and coverage rules. Observing a spawned wrapper's body says what
// that wrapper promises; it never credits its launch as caller completion.

// CallInvokesArgumentOnEveryReturn reports whether a synchronous or deferred
// callee invokes the exact target before every normal return.
func CallInvokesArgumentOnEveryReturn(instruction ssa.Instruction, target ssa.Value) bool {
	return ProveCompletion(CompletionRequest{
		Instruction: instruction, Target: target, InvokeTarget: true,
		Budget: ssaflow.NewSearchBudget(ssaflow.QueryBudget),
	}).Proven()
}

// ProveSpawnedInvocation observes the launched wrapper's body and requires
// synchronous invocation of target before every normal return. The caller's
// budget bounds this query; a cutoff stays unknown and proves no invocation.
func ProveSpawnedInvocation(spawn *ssa.Go, target ssa.Value, budget *ssaflow.SearchBudget) ssaflow.CompletionProof {
	if spawn == nil || target == nil {
		return ssaflow.CompletionProof{Proof: ssaflow.Proof{State: ssaflow.EvidenceUnknown, Reason: ssaflow.EvidenceUnavailable}}
	}
	request := CompletionRequest{Instruction: spawn, Target: target, InvokeTarget: true, Budget: budget}
	function, closure := ssaflow.DirectCallee(spawn.Common())
	if function == nil || len(function.Blocks) == 0 {
		return request.unprovenCompletion(false, false, false)
	}
	search := newCompletionSearch("", CoverageEveryReturn, budget)
	search.exactTarget, search.exactInvocation, search.invokeTarget = true, true, true
	// Examine the body independently of its launch, as for an uninvoked
	// callback. Nested launches still pass through ordinary completion and
	// cannot establish synchronous invocation before the wrapper returns.
	answer := search.calleeCompletes(completionCallee{
		launch: launchCallback, common: spawn.Common(), closure: closure,
		function: function, invocation: spawn,
	}, target, spawn)
	if answer.proven {
		return answer.proof("")
	}
	return request.unprovenCompletion(answer.available, *search.incomplete, *search.inCycle)
}
