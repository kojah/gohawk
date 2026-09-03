package ssaflow

import (
	"strings"

	"golang.org/x/tools/go/ssa"
)

// LocalEvidence memoizes related SSA proof requests for one analyzer scope.
// Its zero value is ready to use and is intentionally not safe for concurrent
// use; each analyzer function owns its evidence.
type LocalEvidence struct {
	identities  map[identityEvidenceKey]IdentityProof
	completions map[completionEvidenceKey]CompletionProof
	transfers   map[transferEvidenceKey]OwnershipTransferProof
}

type identityEvidenceKey struct {
	leftValue, leftRoot, rightValue, rightRoot ssa.Value
}

type completionEvidenceKey struct {
	instruction ssa.Instruction
	target      ssa.Value
	methods     string
	coverage    CompletionCoverage
}

type transferEvidenceKey struct {
	instruction ssa.Instruction
	value       ssa.Value
	modes       OwnershipTransferMode
}

func (evidence *LocalEvidence) Identity(left, right AccessPath) IdentityProof {
	key := identityEvidenceKey{left.Value, left.Root, right.Value, right.Root}
	if proof, ok := evidence.identities[key]; ok {
		return proof
	}
	proof := ProveIdentity(left, right)
	if evidence.identities == nil {
		evidence.identities = make(map[identityEvidenceKey]IdentityProof)
	}
	evidence.identities[key] = proof
	return proof
}

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
