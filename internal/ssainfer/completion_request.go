package ssainfer

import (
	"strings"

	"github.com/kojah/gohawk/internal/ssaflow"
	"golang.org/x/tools/go/ssa"
)

// CompletionRequest asks whether the callee launched by Instruction calls one
// of Methods on Target. The instruction's launch form decides the coverage
// the call must have; callers that accept only some launch forms, such as
// deferred releases, select the instructions they submit.
type CompletionRequest struct {
	Instruction ssa.Instruction
	Target      ssa.Value
	Methods     []string
	// InvokeTarget asks whether the exact function value Target is invoked,
	// rather than a method on an object. Methods must be empty. Asynchronous
	// launches are not invocation completion; their ownership is caller policy.
	InvokeTarget bool
	// ExactTarget excludes containment and may-alias receiver mappings. It is
	// appropriate when completion will justify cleanup in another scope.
	ExactTarget bool
	// Coverage defaults to CoverageEveryReturn. Callers asking only whether a
	// callee may complete the target select CoverageAnywhere.
	Coverage CompletionCoverage
	// Budget, when set, bounds the instructions this question may examine. A
	// nil budget leaves the search unbounded. Exhaustion is reported as an
	// Unknown proof with EvidenceBudgetExhausted rather than a disproof,
	// because the search stopped before it could decide; what an undecided
	// answer permits is the caller's policy, not this package's.
	Budget *ssaflow.SearchBudget
	// Summarized supplies exact completion guarantees for unavailable bodies.
	// Its policy is fixed for this request and all nested summary queries.
	Summarized CompletionSummaryLookup
	// ReturnedSummaries supplies exact callback-to-parameter/result relations
	// for factories whose bodies are unavailable.
	ReturnedSummaries ReturnedCleanupLookup
	// condition is set only by the edge query after resolving an exact call
	// result. It never changes an ordinary completion request's contract.
	condition completionCondition
}

// ProveCompletion answers one completion request. Each call runs its own
// search, which memoizes the questions it asks itself; nothing is cached
// between requests. The proof is Unknown when no callee body was available or
// callback resolution was incomplete, so callers may consult imported
// summaries. A fully searched body that does not complete the target is
// Disproven.
func ProveCompletion(request CompletionRequest) ssaflow.CompletionProof {
	if request.Instruction == nil || request.Target == nil {
		return ssaflow.CompletionProof{Proof: ssaflow.Proof{State: ssaflow.EvidenceUnknown, Reason: ssaflow.EvidenceUnavailable}}
	}
	if request.InvokeTarget && len(request.Methods) != 0 || !request.InvokeTarget && len(request.Methods) == 0 {
		return ssaflow.CompletionProof{Proof: ssaflow.Proof{State: ssaflow.EvidenceUnknown, Reason: ssaflow.EvidenceUnavailable}}
	}
	searched := false
	incomplete := false
	inCycle := false
	methods := request.Methods
	if request.InvokeTarget {
		methods = []string{""}
	}
	for _, method := range methods {
		search := newCompletionSearch(method, request.Coverage, request.Budget)
		search.exactInvocation = request.InvokeTarget
		search.exactTarget = request.ExactTarget || request.InvokeTarget
		search.invokeTarget = request.InvokeTarget
		search.condition = request.condition
		search.summarized = request.Summarized
		search.returnedSummaries = request.ReturnedSummaries
		answer := search.completes(request.Instruction, request.Target)
		if answer.proven {
			return ssaflow.CompletionProof{
				Proof: ssaflow.Proof{
					State: ssaflow.EvidenceProven, Reason: answer.launch.reason(), Method: method, Provenance: ssaflow.EvidenceFromLocalSSA,
				},
				Path:      answer.paths.path,
				PathKnown: answer.paths.known(),
			}
		}
		searched = searched || answer.available
		incomplete = incomplete || *search.incomplete
		inCycle = inCycle || *search.inCycle
	}
	if !searched {
		return request.giveUp(ssaflow.CompletionProof{Proof: ssaflow.Proof{State: ssaflow.EvidenceUnknown, Reason: ssaflow.EvidenceUnavailable}})
	}
	if request.Budget.Exhausted() {
		// The walk stopped early, so a missing completion is not evidence that
		// the callee fails to complete the target.
		return request.giveUp(ssaflow.CompletionProof{Proof: ssaflow.Proof{
			State: ssaflow.EvidenceUnknown, Reason: ssaflow.EvidenceBudgetExhausted, Provenance: ssaflow.EvidenceFromLocalSSA,
		}})
	}
	if inCycle {
		// A helper that releases every element of what it was handed inside
		// a loop, as slackdump's Destroy closes each stored handle, is not
		// covered on every return: the loop's exit edge skips the body, and
		// which element an iteration settles is decided by iteration. That
		// is uncertainty about the element, not a missing completion.
		// https://github.com/rusq/slackdump/blob/f7319928b0993b23d7e9bd8af5e4c69b6f1d2af4/internal/chunk/filemgr.go#L66-L73
		return request.giveUp(ssaflow.CompletionProof{Proof: ssaflow.Proof{
			State: ssaflow.EvidenceUnknown, Reason: ssaflow.EvidenceCompletionInCycle, Provenance: ssaflow.EvidenceFromLocalSSA,
		}})
	}
	if incomplete {
		return request.giveUp(ssaflow.CompletionProof{Proof: ssaflow.Proof{
			State: ssaflow.EvidenceUnknown, Reason: ssaflow.EvidenceUnavailable, Provenance: ssaflow.EvidenceFromLocalSSA,
		}})
	}
	return request.giveUp(ssaflow.CompletionProof{Proof: ssaflow.Proof{
		State: ssaflow.EvidenceDisproven, Reason: ssaflow.EvidenceNotFound, Provenance: ssaflow.EvidenceFromLocalSSA,
	}})
}

// giveUp reports a completion search that proved nothing to the budget's
// observer, naming the launch site, the target, and the methods sought, and
// returns the proof unchanged.
func (request CompletionRequest) giveUp(proof ssaflow.CompletionProof) ssaflow.CompletionProof {
	request.Budget.Observe(proof.Reason, request.Instruction.Pos(), func() map[string]string {
		details := map[string]string{"instruction": request.Instruction.String(), "target": request.Target.Name()}
		if len(request.Methods) != 0 {
			details["methods"] = strings.Join(request.Methods, ",")
		}
		return details
	})
	return proof
}
