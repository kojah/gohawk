package lifecyclefacts

import (
	"go/token"

	ssautil "github.com/kojah/gohawk/internal/analysisutil/ssa"
	analysisTrace "github.com/kojah/gohawk/internal/trace"

	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/ssa"
)

const (
	reasonLifecycleSummary      ssautil.EvidenceReason = "lifecycle-summary"
	reasonReceiverStoreTransfer ssautil.EvidenceReason = "receiver-store-transfer"
	reasonReceiverDoesNotEscape ssautil.EvidenceReason = "receiver-does-not-escape"
)

// EvidenceQuery combines memoized local SSA evidence with lifecycle summaries
// imported through the prerequisite analyzer. One query belongs to one source
// function and is not safe for concurrent use.
type EvidenceQuery struct {
	pass     *analysis.Pass
	analyzer string
	check    string
	local    ssautil.EvidenceQuery
}

// NewEvidenceQuery constructs a query whose accepted, rejected, and unknown
// results use the supplied analyzer identity for structured tracing.
func NewEvidenceQuery(pass *analysis.Pass, analyzer, check string) *EvidenceQuery {
	return &EvidenceQuery{pass: pass, analyzer: analyzer, check: check}
}

// Identity proves a local access-path relationship through the same memoized
// query and tracing channel used for lifecycle evidence.
func (query *EvidenceQuery) Identity(instruction ssa.Instruction, left, right ssautil.AccessPath) ssautil.IdentityProof {
	proof := query.local.Identity(left, right)
	query.emit(EvidenceRequest{Instruction: instruction}, proof.Proof)
	return proof
}

// EvidenceRequest describes local and imported relationships that can settle
// one lifecycle obligation. Completion and Transfer are independently
// optional; imported selectors are consulted only when local evidence does not
// prove the obligation.
type EvidenceRequest struct {
	Instruction   ssa.Instruction
	Target        ssa.Value
	Completion    *ssautil.CompletionRequest
	Transfer      *ssautil.OwnershipTransferRequest
	Local         *ssautil.Proof
	SelectMask    func(Fact) ParameterMask
	ReceiverStore bool
}

// Prove returns one lifecycle proof with explicit provenance. Missing imported
// summaries produce Unknown rather than being conflated with a disproved local
// relationship.
func (query *EvidenceQuery) Prove(request EvidenceRequest) ssautil.Proof {
	local := query.localProof(request)
	if local.Proven() {
		query.emit(request, local)
		return local
	}

	fact, summarized := factFor(query.pass, request.Instruction)
	if request.SelectMask != nil && summarized {
		mask := request.SelectMask(fact)
		if factOwnsArgument(request.Instruction, request.Target, mask) {
			proof := importedProof(reasonLifecycleSummary, requestedMethod(request))
			query.emit(request, proof)
			return proof
		}
	}
	if request.ReceiverStore && summarized && factOwnsArgument(request.Instruction, request.Target, fact.ReceiverStore) {
		receiver := ssautil.CallReceiver(ssautil.InstructionCall(request.Instruction))
		if receiver != nil && (ssautil.ExternallyOwnedValue(receiver) || ssautil.ValueEscapes(receiver)) {
			proof := importedProof(reasonReceiverStoreTransfer, requestedMethod(request))
			query.emit(request, proof)
			return proof
		}
		proof := ssautil.Proof{
			State: ssautil.EvidenceDisproven, Reason: reasonReceiverDoesNotEscape,
			Provenance: ssautil.EvidenceFromImportedFact,
		}
		query.emit(request, proof)
		return proof
	}

	importedRequested := request.SelectMask != nil || request.ReceiverStore
	if importedRequested && !summarized {
		proof := ssautil.Proof{State: ssautil.EvidenceUnknown, Reason: ssautil.EvidenceUnavailable}
		query.emit(request, proof)
		return proof
	}
	if importedRequested && summarized {
		local = ssautil.Proof{
			State: ssautil.EvidenceDisproven, Reason: ssautil.EvidenceNotFound,
			Provenance: ssautil.EvidenceFromImportedFact,
		}
	}
	query.emit(request, local)
	return local
}

func (query *EvidenceQuery) localProof(request EvidenceRequest) ssautil.Proof {
	proof := ssautil.Proof{State: ssautil.EvidenceUnknown, Reason: ssautil.EvidenceUnavailable}
	if request.Local != nil {
		proof = *request.Local
		if proof.Proven() {
			return proof
		}
	}
	if request.Completion != nil {
		completion := query.local.Completion(*request.Completion)
		proof = completion.Proof
		if proof.Proven() {
			return proof
		}
	}
	if request.Transfer != nil {
		transfer := query.local.OwnershipTransfer(*request.Transfer)
		if transfer.Proven() {
			return transfer.Proof
		}
		if !transfer.Known() {
			return transfer.Proof
		}
		if !proof.Known() {
			return transfer.Proof
		}
	}
	return proof
}

func importedProof(reason ssautil.EvidenceReason, method string) ssautil.Proof {
	return ssautil.Proof{
		State: ssautil.EvidenceProven, Reason: reason, Method: method,
		Provenance: ssautil.EvidenceFromImportedFact,
	}
}

func requestedMethod(request EvidenceRequest) string {
	if request.Completion == nil || len(request.Completion.Methods) != 1 {
		return ""
	}
	return request.Completion.Methods[0]
}

func (query *EvidenceQuery) emit(request EvidenceRequest, proof ssautil.Proof) {
	if !analysisTrace.Enabled(query.analyzer, query.check) {
		return
	}
	outcome := analysisTrace.OutcomeUnknown
	switch proof.State {
	case ssautil.EvidenceProven:
		outcome = analysisTrace.OutcomeAccepted
	case ssautil.EvidenceDisproven:
		outcome = analysisTrace.OutcomeRejected
	case ssautil.EvidenceUnknown:
	}
	details := evidenceDetails(request.Instruction, request.Target)
	if proof.Method != "" {
		details["method"] = proof.Method
	}
	if proof.Provenance != "" {
		details["provenance"] = string(proof.Provenance)
	}
	position := token.NoPos
	if request.Instruction != nil {
		position = request.Instruction.Pos()
	}
	analysisTrace.Emit(query.pass, analysisTrace.Event{
		Analyzer: query.analyzer,
		Check:    query.check,
		Phase:    "evidence",
		Reason:   string(proof.Reason),
		Outcome:  outcome,
		Pos:      position,
		Function: instructionFunction(request.Instruction),
		Details:  details,
	})
}

func evidenceDetails(instruction ssa.Instruction, target ssa.Value) map[string]string {
	details := map[string]string{}
	if target != nil && target.Type() != nil {
		details["target_type"] = target.Type().String()
	}
	if common := ssautil.InstructionCall(instruction); common != nil && common.StaticCallee() != nil {
		details["callee"] = common.StaticCallee().String()
	}
	return details
}

func instructionFunction(instruction ssa.Instruction) string {
	if instruction == nil || instruction.Parent() == nil {
		return ""
	}
	return instruction.Parent().String()
}
