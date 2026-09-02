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
	// Coverage defaults to CoverageEveryReturn. Callers asking only whether a
	// callee may complete the target select CoverageAnywhere.
	Coverage CompletionCoverage
}

// Completion proves and memoizes a lifecycle-completion request.
func (evidence *LocalEvidence) Completion(request CompletionRequest) CompletionProof {
	key := completionEvidenceKey{
		instruction: request.Instruction,
		target:      request.Target,
		methods:     strings.Join(request.Methods, "\x00"),
		coverage:    request.Coverage,
	}
	if proof, ok := evidence.completions[key]; ok {
		return proof
	}
	proof := ProveCompletion(request)
	if evidence.completions == nil {
		evidence.completions = make(map[completionEvidenceKey]CompletionProof)
	}
	evidence.completions[key] = proof
	return proof
}

// ProveCompletion runs the completion search once without memoization. The
// proof is Unknown when no callee body was available to search, so callers
// may consult imported summaries, and Disproven when a searched body does not
// complete the target.
func ProveCompletion(request CompletionRequest) CompletionProof {
	if request.Instruction == nil || request.Target == nil || len(request.Methods) == 0 {
		return CompletionProof{Proof{State: EvidenceUnknown, Reason: EvidenceUnavailable}}
	}
	searched := false
	for _, method := range request.Methods {
		search := newCompletionSearch(method, request.Coverage)
		launch, proven, available := search.completes(request.Instruction, request.Target)
		if proven {
			return CompletionProof{Proof{State: EvidenceProven, Reason: launch.reason(), Method: method, Provenance: EvidenceFromLocalSSA}}
		}
		searched = searched || available
	}
	if !searched {
		return CompletionProof{Proof{State: EvidenceUnknown, Reason: EvidenceUnavailable}}
	}
	return CompletionProof{Proof{State: EvidenceDisproven, Reason: EvidenceNotFound, Provenance: EvidenceFromLocalSSA}}
}
