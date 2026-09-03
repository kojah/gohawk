package ssaflow

import "golang.org/x/tools/go/ssa"

// CompletionRequest asks whether the callee launched by Instruction calls one
// of Methods on Target. The instruction's launch form decides the coverage
// the call must have; callers that accept only some launch forms, such as
// deferred releases, select the instructions they submit.
type CompletionRequest struct {
	Instruction ssa.Instruction
	Target      ssa.Value
	Methods     []string
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
// between requests. The proof is Unknown when no callee body was available to
// search, so callers may consult imported summaries, and Disproven when a
// searched body does not complete the target.
func ProveCompletion(request CompletionRequest) CompletionProof {
	if request.Instruction == nil || request.Target == nil || len(request.Methods) == 0 {
		return CompletionProof{Proof{State: EvidenceUnknown, Reason: EvidenceUnavailable}}
	}
	searched := false
	for _, method := range request.Methods {
		search := newCompletionSearch(method, request.Coverage, request.Budget)
		launch, proven, available := search.completes(request.Instruction, request.Target)
		if proven {
			return CompletionProof{Proof{State: EvidenceProven, Reason: launch.reason(), Method: method, Provenance: EvidenceFromLocalSSA}}
		}
		searched = searched || available
	}
	if !searched {
		return CompletionProof{Proof{State: EvidenceUnknown, Reason: EvidenceUnavailable}}
	}
	if request.Budget.Exhausted() {
		// The walk stopped early, so a missing completion is not evidence that
		// the callee fails to complete the target.
		return CompletionProof{Proof{State: EvidenceUnknown, Reason: EvidenceBudgetExhausted, Provenance: EvidenceFromLocalSSA}}
	}
	return CompletionProof{Proof{State: EvidenceDisproven, Reason: EvidenceNotFound, Provenance: EvidenceFromLocalSSA}}
}
