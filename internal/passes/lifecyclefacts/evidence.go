package lifecyclefacts

import (
	"go/token"

	"github.com/kojah/gohawk/internal/ssaflow"
	analysisTrace "github.com/kojah/gohawk/internal/trace"

	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/ssa"
)

const (
	reasonLifecycleSummary      ssaflow.EvidenceReason = "lifecycle-summary"
	reasonReceiverStoreTransfer ssaflow.EvidenceReason = "receiver-store-transfer"
	reasonReceiverDoesNotEscape ssaflow.EvidenceReason = "receiver-does-not-escape"
)

// LifecycleEvidence combines memoized local SSA evidence with lifecycle summaries
// imported through the prerequisite analyzer. One evidence context belongs to one source
// function and is not safe for concurrent use.
type LifecycleEvidence struct {
	pass     *analysis.Pass
	analyzer string
	check    string
	local    ssaflow.LocalEvidence
}

// NewLifecycleEvidence constructs evidence whose accepted, rejected, and unknown
// results use the supplied analyzer identity for structured tracing.
func NewLifecycleEvidence(pass *analysis.Pass, analyzer, check string) *LifecycleEvidence {
	return &LifecycleEvidence{pass: pass, analyzer: analyzer, check: check}
}

// Identity proves a local access-path relationship through the same memoized
// evidence and tracing channel used for lifecycle evidence.
func (evidence *LifecycleEvidence) Identity(instruction ssa.Instruction, left, right ssaflow.AccessPath) ssaflow.IdentityProof {
	proof := evidence.local.Identity(left, right)
	evidence.emit(EvidenceRequest{Instruction: instruction}, proof.Proof)
	return proof
}

// EvidenceRequest describes local and imported relationships that can settle
// one lifecycle obligation. Completion and Transfer are independently
// optional; imported selectors are consulted only when local evidence does not
// prove the obligation.
type EvidenceRequest struct {
	Instruction   ssa.Instruction
	Target        ssa.Value
	Completion    *ssaflow.CompletionRequest
	Transfer      *ssaflow.OwnershipTransferRequest
	Local         *ssaflow.Proof
	SelectMask    func(Fact) ParameterMask
	ReceiverStore bool
}

// Prove returns one lifecycle proof with explicit provenance. Missing imported
// summaries produce Unknown rather than being conflated with a disproved local
// relationship.
func (evidence *LifecycleEvidence) Prove(request EvidenceRequest) ssaflow.Proof {
	local := evidence.localProof(request)
	if local.Proven() {
		evidence.emit(request, local)
		return local
	}

	fact, summarized := factFor(evidence.pass, request.Instruction)
	if request.SelectMask != nil && summarized {
		mask := request.SelectMask(fact)
		if factOwnsArgument(request.Instruction, request.Target, mask) {
			proof := importedProof(reasonLifecycleSummary, requestedMethod(request))
			evidence.emit(request, proof)
			return proof
		}
	}
	if request.ReceiverStore && summarized && factOwnsArgument(request.Instruction, request.Target, fact.ReceiverStore) {
		receiver := ssaflow.CallReceiver(ssaflow.InstructionCall(request.Instruction))
		if receiver != nil && (ssaflow.ExternallyOwnedValue(receiver) || ssaflow.ValueEscapes(receiver)) {
			proof := importedProof(reasonReceiverStoreTransfer, requestedMethod(request))
			evidence.emit(request, proof)
			return proof
		}
		proof := ssaflow.Proof{
			State: ssaflow.EvidenceDisproven, Reason: reasonReceiverDoesNotEscape,
			Provenance: ssaflow.EvidenceFromImportedFact,
		}
		evidence.emit(request, proof)
		return proof
	}

	importedRequested := request.SelectMask != nil || request.ReceiverStore
	if importedRequested && !summarized {
		proof := ssaflow.Proof{State: ssaflow.EvidenceUnknown, Reason: ssaflow.EvidenceUnavailable}
		evidence.emit(request, proof)
		return proof
	}
	if importedRequested && summarized {
		local = ssaflow.Proof{
			State: ssaflow.EvidenceDisproven, Reason: ssaflow.EvidenceNotFound,
			Provenance: ssaflow.EvidenceFromImportedFact,
		}
	}
	evidence.emit(request, local)
	return local
}

func (evidence *LifecycleEvidence) localProof(request EvidenceRequest) ssaflow.Proof {
	proof := ssaflow.Proof{State: ssaflow.EvidenceUnknown, Reason: ssaflow.EvidenceUnavailable}
	if request.Local != nil {
		proof = *request.Local
		if proof.Proven() {
			return proof
		}
	}
	if request.Completion != nil {
		completion := evidence.local.Completion(*request.Completion)
		proof = completion.Proof
		if proof.Proven() {
			return proof
		}
	}
	if request.Transfer != nil {
		transfer := evidence.local.OwnershipTransfer(*request.Transfer)
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

func importedProof(reason ssaflow.EvidenceReason, method string) ssaflow.Proof {
	return ssaflow.Proof{
		State: ssaflow.EvidenceProven, Reason: reason, Method: method,
		Provenance: ssaflow.EvidenceFromImportedFact,
	}
}

func requestedMethod(request EvidenceRequest) string {
	if request.Completion == nil || len(request.Completion.Methods) != 1 {
		return ""
	}
	return request.Completion.Methods[0]
}

func (evidence *LifecycleEvidence) emit(request EvidenceRequest, proof ssaflow.Proof) {
	if !analysisTrace.Enabled(evidence.analyzer, evidence.check) {
		return
	}
	outcome := analysisTrace.OutcomeUnknown
	switch proof.State {
	case ssaflow.EvidenceProven:
		outcome = analysisTrace.OutcomeAccepted
	case ssaflow.EvidenceDisproven:
		outcome = analysisTrace.OutcomeRejected
	case ssaflow.EvidenceUnknown:
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
	analysisTrace.Emit(evidence.pass, analysisTrace.Event{
		Analyzer: evidence.analyzer,
		Check:    evidence.check,
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
	if common := ssaflow.InstructionCall(instruction); common != nil && common.StaticCallee() != nil {
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
