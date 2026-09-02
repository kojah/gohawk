package ssaflow

import "golang.org/x/tools/go/ssa"

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
