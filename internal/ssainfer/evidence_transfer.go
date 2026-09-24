package ssainfer

import (
	"github.com/kojah/gohawk/internal/ssaflow"
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
func (evidence *LocalEvidence) OwnershipTransfer(request OwnershipTransferRequest) ssaflow.OwnershipTransferProof {
	key := transferEvidenceKey{instruction: request.Instruction, value: request.Value, modes: request.Modes}
	if proof, ok := evidence.transfers[key]; ok {
		return proof
	}
	proof := proveOwnershipTransfer(request)
	if evidence.transfers == nil {
		evidence.transfers = make(map[transferEvidenceKey]ssaflow.OwnershipTransferProof)
	}
	evidence.transfers[key] = proof
	return proof
}

func proveOwnershipTransfer(request OwnershipTransferRequest) ssaflow.OwnershipTransferProof {
	if request.Instruction == nil || request.Value == nil || request.Modes == 0 {
		return ssaflow.OwnershipTransferProof{Proof: ssaflow.Proof{State: ssaflow.EvidenceUnknown, Reason: ssaflow.EvidenceUnavailable}}
	}
	checks := []struct {
		mode   OwnershipTransferMode
		reason ssaflow.EvidenceReason
		proven func(ssa.Instruction, ssa.Value) bool
	}{
		{TransferStoredInField, ssaflow.EvidenceStoredInField, StoresValueInField},
		{TransferOwnerStoredInField, ssaflow.EvidenceOwnerStoredInField, StoresOwnerOfValueInField},
		{TransferStoredInGlobal, ssaflow.EvidenceStoredInGlobal, StoresValueInGlobal},
		{TransferStoredInEnclosingScope, ssaflow.EvidenceStoredInEnclosingScope, StoresValueInEnclosingScope},
		{TransferOwnerStoredInExternalField, ssaflow.EvidenceOwnerStoredInExternalField, StoresOwnerOfValueInExternalField},
		{TransferStoredInOwnedMap, ssaflow.EvidenceStoredInOwnedMap, StoresValueInOwnedMap},
		{TransferSentToReceiver, ssaflow.EvidenceSentToReceiver, SendsValue},
		{TransferCapturedByClosure, ssaflow.EvidenceCapturedByClosure, ClosureCapturesValue},
		{TransferCallResultStoredInField, ssaflow.EvidenceCallResultStoredInField, CallTransfersValueToField},
		{TransferToReturnedOwner, ssaflow.EvidenceTransferredToReturnedOwner, CallTransfersArgumentToReturnedOwner},
		{TransferToReceiver, ssaflow.EvidenceTransferredToReceiver, CallTransfersArgumentToReceiver},
		{TransferToLifecycleOwner, ssaflow.EvidenceTransferredToLifecycleOwner, CallTransfersArgumentToLifecycleOwner},
	}
	for _, check := range checks {
		if request.Modes&check.mode != 0 && check.proven(request.Instruction, request.Value) {
			return ssaflow.OwnershipTransferProof{Proof: ssaflow.Proof{
				State: ssaflow.EvidenceProven, Reason: check.reason, Provenance: ssaflow.EvidenceFromLocalSSA,
			}}
		}
	}
	if transferEvidenceUnavailable(request) {
		return ssaflow.OwnershipTransferProof{Proof: ssaflow.Proof{State: ssaflow.EvidenceUnknown, Reason: ssaflow.EvidenceUnavailable}}
	}
	return ssaflow.OwnershipTransferProof{Proof: ssaflow.Proof{
		State: ssaflow.EvidenceDisproven, Reason: ssaflow.EvidenceNotFound, Provenance: ssaflow.EvidenceFromLocalSSA,
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
