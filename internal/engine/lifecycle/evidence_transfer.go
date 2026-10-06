package lifecycle

import (
	proofs "github.com/kojah/gohawk/internal/engine/proof"
	"github.com/kojah/gohawk/internal/engine/ssaflow"
	"golang.org/x/tools/go/ssa"
)

// OwnershipTransferMode selects concrete escape relationships that an analyzer
// accepts as transfer of its lifecycle obligation.
type OwnershipTransferMode uint16

const (
	TransferStoredInField OwnershipTransferMode = 1 << iota
	TransferOwnerStoredInField
	TransferStoredInGlobal
	TransferStoredInEnclosingScope
	TransferOwnerStoredInExternalField
	TransferStoredInOwnedMap
	TransferSentToReceiver
	TransferCapturedByClosure
	TransferCallResultStoredInField
	TransferToReturnedOwner
	TransferToReceiver
	TransferToLifecycleOwner
)

// OwnershipTransferRequest describes the value flow relationships that may
// transfer one analyzer's ownership obligation.
type OwnershipTransferRequest struct {
	Instruction ssa.Instruction
	Value       ssa.Value
	Modes       OwnershipTransferMode
}

// OwnershipTransfer proves and memoizes an ownership-transfer request.
func (evidence *LocalEvidence) OwnershipTransfer(request OwnershipTransferRequest) proofs.OwnershipTransferProof {
	key := transferEvidenceKey{instruction: request.Instruction, value: request.Value, modes: request.Modes}
	if proof, ok := evidence.transfers[key]; ok {
		return proof
	}
	proof := proveOwnershipTransfer(request)
	if evidence.transfers == nil {
		evidence.transfers = make(map[transferEvidenceKey]proofs.OwnershipTransferProof)
	}
	evidence.transfers[key] = proof
	return proof
}

func proveOwnershipTransfer(request OwnershipTransferRequest) proofs.OwnershipTransferProof {
	if request.Instruction == nil || request.Value == nil || request.Modes == 0 {
		return proofs.OwnershipTransferProof{Proof: proofs.Proof{State: proofs.EvidenceUnknown, Reason: proofs.EvidenceUnavailable}}
	}
	checks := []struct {
		mode   OwnershipTransferMode
		reason proofs.EvidenceReason
		proven func(ssa.Instruction, ssa.Value) bool
	}{
		{TransferStoredInField, proofs.EvidenceStoredInField, StoresValueInField},
		{TransferOwnerStoredInField, proofs.EvidenceOwnerStoredInField, StoresOwnerOfValueInField},
		{TransferStoredInGlobal, proofs.EvidenceStoredInGlobal, StoresValueInGlobal},
		{TransferStoredInEnclosingScope, proofs.EvidenceStoredInEnclosingScope, StoresValueInEnclosingScope},
		{TransferOwnerStoredInExternalField, proofs.EvidenceOwnerStoredInExternalField, StoresOwnerOfValueInExternalField},
		{TransferStoredInOwnedMap, proofs.EvidenceStoredInOwnedMap, StoresValueInOwnedMap},
		{TransferSentToReceiver, proofs.EvidenceSentToReceiver, SendsValue},
		{TransferCapturedByClosure, proofs.EvidenceCapturedByClosure, ClosureCapturesValue},
		{TransferCallResultStoredInField, proofs.EvidenceCallResultStoredInField, CallTransfersValueToField},
		{TransferToReturnedOwner, proofs.EvidenceTransferredToReturnedOwner, CallTransfersArgumentToReturnedOwner},
		{TransferToReceiver, proofs.EvidenceTransferredToReceiver, CallTransfersArgumentToReceiver},
		{TransferToLifecycleOwner, proofs.EvidenceTransferredToLifecycleOwner, CallTransfersArgumentToLifecycleOwner},
	}
	for _, check := range checks {
		if request.Modes&check.mode != 0 && check.proven(request.Instruction, request.Value) {
			return proofs.OwnershipTransferProof{Proof: proofs.Proof{
				State: proofs.EvidenceProven, Reason: check.reason, Provenance: proofs.EvidenceFromLocalSSA,
			}}
		}
	}
	if transferEvidenceUnavailable(request) {
		return proofs.OwnershipTransferProof{Proof: proofs.Proof{State: proofs.EvidenceUnknown, Reason: proofs.EvidenceUnavailable}}
	}
	return proofs.OwnershipTransferProof{Proof: proofs.Proof{
		State: proofs.EvidenceDisproven, Reason: proofs.EvidenceNotFound, Provenance: proofs.EvidenceFromLocalSSA,
	}}
}

func transferEvidenceUnavailable(request OwnershipTransferRequest) bool {
	interprocedural := request.Modes&(TransferToReturnedOwner|TransferToReceiver) != 0
	if !interprocedural {
		return false
	}
	common := ssaflow.InstructionCall(request.Instruction)
	return common == nil || common.StaticCallee() == nil || len(common.StaticCallee().Blocks) == 0
}
