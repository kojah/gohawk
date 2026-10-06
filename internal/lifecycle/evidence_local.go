package lifecycle

import (
	"strings"

	proofs "github.com/kojah/gohawk/internal/proof"
	ssacall "github.com/kojah/gohawk/internal/ssaflow/calls"
	"golang.org/x/tools/go/ssa"
)

// LocalEvidence memoizes related SSA proof requests for one analyzer scope.
// Its zero value is ready to use and is intentionally not safe for concurrent
// use; each analyzer function owns its evidence.
type LocalEvidence struct {
	completions map[completionEvidenceKey]proofs.CompletionProof
	transfers   map[transferEvidenceKey]proofs.OwnershipTransferProof
	returned    ReturnedCleanupLookup
}

// NewLocalEvidenceWithReturnedCleanup fixes one immutable returned-summary
// policy for this evidence scope. Fixing the policy permits cache reuse;
// per-request lookup overrides remain uncached. The lookup's answers must not
// change during the scope's lifetime.
func NewLocalEvidenceWithReturnedCleanup(lookup ReturnedCleanupLookup) LocalEvidence {
	return LocalEvidence{returned: lookup}
}

type completionEvidenceKey struct {
	instruction  ssa.Instruction
	target       ssa.Value
	methods      string
	coverage     CompletionCoverage
	invokeTarget bool
	exactTarget  bool
	condition    ssacall.CallCondition
}

type transferEvidenceKey struct {
	instruction ssa.Instruction
	value       ssa.Value
	modes       OwnershipTransferMode
}

func (evidence *LocalEvidence) Completion(request CompletionRequest) proofs.CompletionProof {
	// Lookup policies may differ between requests. Their identities are not
	// comparable; retain only the per-query summary cache in this case.
	if request.Summarized != nil || request.CallContract != nil || request.ReturnedSummaries != nil || len(request.Constants) != 0 {
		if request.ReturnedSummaries == nil {
			request.ReturnedSummaries = evidence.returned
		}
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
	request.ReturnedSummaries = evidence.returned
	proof := ProveCompletion(request)
	// A cutoff describes this request's allowance, not the callee's behavior.
	// Retaining it would prevent a later query with fresh allowance from deciding.
	if proof.Reason == proofs.EvidenceBudgetExhausted || request.Budget.Exhausted() || request.Budget.PoolExhausted() {
		return proof
	}
	if evidence.completions == nil {
		evidence.completions = make(map[completionEvidenceKey]proofs.CompletionProof)
	}
	evidence.completions[key] = proof
	return proof
}
