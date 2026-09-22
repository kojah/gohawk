package ssaflow

import (
	"strings"

	"golang.org/x/tools/go/ssa"
)

// LocalEvidence memoizes related SSA proof requests for one analyzer scope.
// Its zero value is ready to use and is intentionally not safe for concurrent
// use; each analyzer function owns its evidence.
type LocalEvidence struct {
	completions map[completionEvidenceKey]CompletionProof
	transfers   map[transferEvidenceKey]OwnershipTransferProof
}

type completionEvidenceKey struct {
	instruction  ssa.Instruction
	target       ssa.Value
	methods      string
	coverage     CompletionCoverage
	invokeTarget bool
	exactTarget  bool
	condition    completionCondition
}

type transferEvidenceKey struct {
	instruction ssa.Instruction
	value       ssa.Value
	modes       OwnershipTransferMode
}

func (evidence *LocalEvidence) Completion(request CompletionRequest) CompletionProof {
	// Lookup policies may differ between requests. Their identities are not
	// comparable; retain only the per-query summary cache in this case.
	if request.Summarized != nil || request.ReturnedSummaries != nil {
		return ProveCompletion(request)
	}
	key := completionEvidenceKey{
		instruction:  request.Instruction,
		target:       request.Target,
		methods:      strings.Join(request.Methods, "\x00"),
		coverage:     request.Coverage,
		invokeTarget: request.InvokeTarget,
		exactTarget:  request.ExactTarget,
		condition:    request.condition,
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
