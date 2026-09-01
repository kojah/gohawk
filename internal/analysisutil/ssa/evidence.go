package ssautil

import "golang.org/x/tools/go/ssa"

// EvidenceReason identifies the concrete SSA relationship that established a
// proof. Reason values are stable diagnostic vocabulary suitable for tracing.
type EvidenceReason string

const (
	EvidenceNone EvidenceReason = ""

	EvidenceSameValue      EvidenceReason = "same-value"
	EvidenceSameAccessPath EvidenceReason = "same-access-path"

	EvidenceDeferredCompletion      EvidenceReason = "deferred-completion"
	EvidenceClosureCompletion       EvidenceReason = "closure-completion"
	EvidenceCompletionBeforeBranch  EvidenceReason = "completion-before-branch"
	EvidenceHelperCompletion        EvidenceReason = "helper-completion"
	EvidenceStartedCompletion       EvidenceReason = "started-completion"
	EvidenceStartedHelperCompletion EvidenceReason = "started-helper-completion"

	EvidenceStoredInField               EvidenceReason = "stored-in-field"
	EvidenceOwnerStoredInField          EvidenceReason = "owner-stored-in-field"
	EvidenceStoredInGlobal              EvidenceReason = "stored-in-global"
	EvidenceStoredInEnclosingScope      EvidenceReason = "stored-in-enclosing-scope"
	EvidenceOwnerStoredInExternalField  EvidenceReason = "owner-stored-in-external-field"
	EvidenceStoredInOwnedMap            EvidenceReason = "stored-in-owned-map"
	EvidenceSentToReceiver              EvidenceReason = "sent-to-receiver"
	EvidenceCapturedByClosure           EvidenceReason = "captured-by-closure"
	EvidenceCallResultStoredInField     EvidenceReason = "call-result-stored-in-field"
	EvidenceTransferredToReturnedOwner  EvidenceReason = "transferred-to-returned-owner"
	EvidenceTransferredToReceiver       EvidenceReason = "transferred-to-receiver"
	EvidenceTransferredToLifecycleOwner EvidenceReason = "transferred-to-lifecycle-owner"
)

// Proof records whether an SSA policy was established and why. Its zero value
// represents an unproven relationship.
type Proof struct {
	Reason EvidenceReason
	Method string
}

// Proven reports whether the requested relationship was established.
func (proof Proof) Proven() bool {
	return proof.Reason != EvidenceNone
}

// IdentityProof records evidence that two SSA values denote the same value or
// corresponding access path.
type IdentityProof struct{ Proof }

// CompletionProof records evidence that a lifecycle method runs under the
// path guarantees selected by an analyzer.
type CompletionProof struct{ Proof }

// OwnershipTransferProof records evidence that an obligation moved to an
// owner accepted by an analyzer.
type OwnershipTransferProof struct{ Proof }

// ProveIdentity reports whether two values denote corresponding access paths
// beneath roots that the caller has already established as equivalent.
func ProveIdentity(left, right AccessPath) IdentityProof {
	if SameValue(left.Value, right.Value) {
		return IdentityProof{Proof{Reason: EvidenceSameValue}}
	}
	if SameAccessPath(left, right) {
		return IdentityProof{Proof{Reason: EvidenceSameAccessPath}}
	}
	return IdentityProof{}
}

// CompletionMode selects lifecycle-completion relationships accepted by an
// analyzer. Modes are policy: callers should enable only the relationships
// that settle their particular ownership obligation.
type CompletionMode uint8

const (
	CompletionDeferred CompletionMode = 1 << iota
	CompletionInClosure
	CompletionBeforeBranch
	CompletionByHelper
	CompletionInStartedClosure
	CompletionByStartedHelper
)

// CompletionRequest describes lifecycle completion to prove for one or more
// equivalent cleanup methods.
type CompletionRequest struct {
	Instruction ssa.Instruction
	Target      ssa.Value
	Methods     []string
	Modes       CompletionMode
}

// ProveCompletion returns the first concrete relationship, in precision-first
// order, that proves the requested lifecycle completion.
func ProveCompletion(request CompletionRequest) CompletionProof {
	for _, method := range request.Methods {
		if request.Modes&CompletionDeferred != 0 && DeferredClosureCalls(request.Instruction, method, request.Target) {
			return CompletionProof{Proof{Reason: EvidenceDeferredCompletion, Method: method}}
		}
		if request.Modes&CompletionBeforeBranch != 0 && ClosureCallsMethodBeforeBranch(request.Instruction, method, request.Target) {
			return CompletionProof{Proof{Reason: EvidenceCompletionBeforeBranch, Method: method}}
		}
		if request.Modes&CompletionByHelper != 0 && CallCallsMethodOnArgumentOnEveryReturn(request.Instruction, method, request.Target) {
			return CompletionProof{Proof{Reason: EvidenceHelperCompletion, Method: method}}
		}
		if request.Modes&CompletionInStartedClosure != 0 && StartedClosureCallsMethodOnEveryReturn(request.Instruction, method, request.Target) {
			return CompletionProof{Proof{Reason: EvidenceStartedCompletion, Method: method}}
		}
		if request.Modes&CompletionByStartedHelper != 0 && StartedClosureCallsMethodViaHelper(request.Instruction, method, request.Target) {
			return CompletionProof{Proof{Reason: EvidenceStartedHelperCompletion, Method: method}}
		}
		if request.Modes&CompletionInClosure != 0 && ClosureCallsMethod(request.Instruction, method, request.Target) {
			return CompletionProof{Proof{Reason: EvidenceClosureCompletion, Method: method}}
		}
	}
	return CompletionProof{}
}

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

// ProveOwnershipTransfer returns the first concrete relationship that proves
// ownership escaped to an accepted owner.
func ProveOwnershipTransfer(request OwnershipTransferRequest) OwnershipTransferProof {
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
			return OwnershipTransferProof{Proof{Reason: check.reason}}
		}
	}
	return OwnershipTransferProof{}
}
