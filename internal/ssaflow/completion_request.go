package ssaflow

import (
	"strings"

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
	Budget *SearchBudget
}

// ProveCompletion answers one completion request. Each call runs its own
// search, which memoizes the questions it asks itself; nothing is cached
// between requests. The proof is Unknown when no callee body was available or
// callback resolution was incomplete, so callers may consult imported
// summaries. A fully searched body that does not complete the target is
// Disproven.
func ProveCompletion(request CompletionRequest) CompletionProof {
	if request.Instruction == nil || request.Target == nil {
		return CompletionProof{Proof{State: EvidenceUnknown, Reason: EvidenceUnavailable}}
	}
	if request.InvokeTarget && len(request.Methods) != 0 || !request.InvokeTarget && len(request.Methods) == 0 {
		return CompletionProof{Proof{State: EvidenceUnknown, Reason: EvidenceUnavailable}}
	}
	searched := false
	incomplete := false
	methods := request.Methods
	if request.InvokeTarget {
		methods = []string{""}
	}
	for _, method := range methods {
		search := newCompletionSearch(method, request.Coverage, request.Budget)
		search.exactInvocation = request.InvokeTarget
		search.exactTarget = request.ExactTarget || request.InvokeTarget
		search.invokeTarget = request.InvokeTarget
		launch, proven, available := search.completes(request.Instruction, request.Target)
		if proven {
			return CompletionProof{Proof{State: EvidenceProven, Reason: launch.reason(), Method: method, Provenance: EvidenceFromLocalSSA}}
		}
		searched = searched || available
		incomplete = incomplete || *search.incomplete
	}
	if !searched {
		return request.giveUp(CompletionProof{Proof{State: EvidenceUnknown, Reason: EvidenceUnavailable}})
	}
	if request.Budget.Exhausted() {
		// The walk stopped early, so a missing completion is not evidence that
		// the callee fails to complete the target.
		return request.giveUp(CompletionProof{Proof{State: EvidenceUnknown, Reason: EvidenceBudgetExhausted, Provenance: EvidenceFromLocalSSA}})
	}
	if incomplete {
		return request.giveUp(CompletionProof{Proof{State: EvidenceUnknown, Reason: EvidenceUnavailable, Provenance: EvidenceFromLocalSSA}})
	}
	return request.giveUp(CompletionProof{Proof{State: EvidenceDisproven, Reason: EvidenceNotFound, Provenance: EvidenceFromLocalSSA}})
}

// giveUp reports a completion search that proved nothing to the budget's
// observer, naming the launch site, the target, and the methods sought, and
// returns the proof unchanged.
func (request CompletionRequest) giveUp(proof CompletionProof) CompletionProof {
	request.Budget.observe(proof.Reason, request.Instruction.Pos(), func() map[string]string {
		details := map[string]string{"instruction": request.Instruction.String(), "target": request.Target.Name()}
		if len(request.Methods) != 0 {
			details["methods"] = strings.Join(request.Methods, ",")
		}
		return details
	})
	return proof
}
