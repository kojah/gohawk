package ssaflow

import "golang.org/x/tools/go/ssa"

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
func (evidence *LocalEvidence) OwnershipTransfer(request OwnershipTransferRequest) OwnershipTransferProof {
	key := transferEvidenceKey{instruction: request.Instruction, value: request.Value, modes: request.Modes}
	if proof, ok := evidence.transfers[key]; ok {
		return proof
	}
	proof := proveOwnershipTransfer(request)
	if evidence.transfers == nil {
		evidence.transfers = make(map[transferEvidenceKey]OwnershipTransferProof)
	}
	evidence.transfers[key] = proof
	return proof
}

func proveOwnershipTransfer(request OwnershipTransferRequest) OwnershipTransferProof {
	if request.Instruction == nil || request.Value == nil || request.Modes == 0 {
		return OwnershipTransferProof{Proof{State: EvidenceUnknown, Reason: EvidenceUnavailable}}
	}
	checks := []struct {
		mode   OwnershipTransferMode
		reason EvidenceReason
		proven func(ssa.Instruction, ssa.Value) bool
	}{
		{TransferStoredInField, EvidenceStoredInField, StoresValueInField},
		{TransferOwnerStoredInField, EvidenceOwnerStoredInField, StoresOwnerOfValueInField},
		{TransferStoredInGlobal, EvidenceStoredInGlobal, StoresValueInGlobal},
		{TransferStoredInEnclosingScope, EvidenceStoredInEnclosingScope, StoresValueInEnclosingScope},
		{TransferOwnerStoredInExternalField, EvidenceOwnerStoredInExternalField, StoresOwnerOfValueInExternalField},
		{TransferStoredInOwnedMap, EvidenceStoredInOwnedMap, StoresValueInOwnedMap},
		{TransferSentToReceiver, EvidenceSentToReceiver, SendsValue},
		{TransferCapturedByClosure, EvidenceCapturedByClosure, ClosureCapturesValue},
		{TransferCallResultStoredInField, EvidenceCallResultStoredInField, CallTransfersValueToField},
		{TransferToReturnedOwner, EvidenceTransferredToReturnedOwner, CallTransfersArgumentToReturnedOwner},
		{TransferToReceiver, EvidenceTransferredToReceiver, CallTransfersArgumentToReceiver},
		{TransferToLifecycleOwner, EvidenceTransferredToLifecycleOwner, CallTransfersArgumentToLifecycleOwner},
	}
	for _, check := range checks {
		if request.Modes&check.mode != 0 && check.proven(request.Instruction, request.Value) {
			return OwnershipTransferProof{Proof{
				State: EvidenceProven, Reason: check.reason, Provenance: EvidenceFromLocalSSA,
			}}
		}
	}
	if transferEvidenceUnavailable(request) {
		return OwnershipTransferProof{Proof{State: EvidenceUnknown, Reason: EvidenceUnavailable}}
	}
	return OwnershipTransferProof{Proof{State: EvidenceDisproven, Reason: EvidenceNotFound, Provenance: EvidenceFromLocalSSA}}
}

func transferEvidenceUnavailable(request OwnershipTransferRequest) bool {
	interprocedural := request.Modes&(TransferToReturnedOwner|TransferToReceiver) != 0
	if !interprocedural {
		return false
	}
	common := InstructionCall(request.Instruction)
	return common == nil || common.StaticCallee() == nil || len(common.StaticCallee().Blocks) == 0
}
