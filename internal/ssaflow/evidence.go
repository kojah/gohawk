package ssaflow

import (
	"strings"

	"golang.org/x/tools/go/ssa"
)

// EvidenceReason identifies the concrete SSA relationship that established a
// proof. Reason values are stable diagnostic vocabulary suitable for tracing.
type EvidenceReason string

const (
	EvidenceNone        EvidenceReason = ""
	EvidenceNotFound    EvidenceReason = "evidence-not-found"
	EvidenceUnavailable EvidenceReason = "evidence-unavailable"

	EvidenceSameValue      EvidenceReason = "same-value"
	EvidenceSameAccessPath EvidenceReason = "same-access-path"

	EvidenceDeferredCompletion      EvidenceReason = "deferred-completion"
	EvidenceClosureCompletion       EvidenceReason = "closure-completion"
	EvidenceCompletionBeforeBranch  EvidenceReason = "completion-before-branch"
	EvidenceHelperCompletion        EvidenceReason = "helper-completion"
	EvidenceStartedCompletion       EvidenceReason = "started-completion"
	EvidenceStartedHelperCompletion EvidenceReason = "started-helper-completion"
	EvidenceHelperInvocation        EvidenceReason = "helper-invocation"
	EvidenceReturnedDeferredCleanup EvidenceReason = "returned-deferred-cleanup"

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

// EvidenceState distinguishes a disproved relationship from one that could
// not be decided with the available SSA. Unknown is the useful zero value.
type EvidenceState uint8

const (
	EvidenceUnknown EvidenceState = iota
	EvidenceDisproven
	EvidenceProven
)

// EvidenceProvenance identifies the analysis boundary that supplied a proof.
type EvidenceProvenance string

const (
	EvidenceFromLocalSSA     EvidenceProvenance = "local-ssa"
	EvidenceFromImportedFact EvidenceProvenance = "imported-fact"
)

// Proof records whether an SSA policy was established and why. Its zero value
// represents an unproven relationship.
type Proof struct {
	State      EvidenceState
	Reason     EvidenceReason
	Method     string
	Provenance EvidenceProvenance
}

// Proven reports whether the requested relationship was established.
func (proof Proof) Proven() bool {
	return proof.State == EvidenceProven
}

// Known reports whether available evidence proved or disproved the requested
// relationship.
func (proof Proof) Known() bool {
	return proof.State != EvidenceUnknown
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
	var query EvidenceQuery
	return query.Identity(left, right)
}

// EvidenceQuery memoizes related SSA proof requests for one analyzer scope.
// Its zero value is ready to use and is intentionally not safe for concurrent
// use; each analyzer function owns its query.
type EvidenceQuery struct {
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
	modes       CompletionMode
}

type transferEvidenceKey struct {
	instruction ssa.Instruction
	value       ssa.Value
	modes       OwnershipTransferMode
}

// Identity proves and memoizes whether two values denote corresponding access
// paths beneath roots already established as equivalent by the caller.
func (query *EvidenceQuery) Identity(left, right AccessPath) IdentityProof {
	key := identityEvidenceKey{left.Value, left.Root, right.Value, right.Root}
	if proof, ok := query.identities[key]; ok {
		return proof
	}
	proof := proveIdentity(left, right)
	if query.identities == nil {
		query.identities = make(map[identityEvidenceKey]IdentityProof)
	}
	query.identities[key] = proof
	return proof
}

func proveIdentity(left, right AccessPath) IdentityProof {
	if left.Value == nil || right.Value == nil {
		return IdentityProof{Proof{State: EvidenceUnknown, Reason: EvidenceUnavailable}}
	}
	if SameValue(left.Value, right.Value) {
		return IdentityProof{Proof{State: EvidenceProven, Reason: EvidenceSameValue, Provenance: EvidenceFromLocalSSA}}
	}
	leftPath, leftOK := accessPath(left.Value, left.Root, map[ssa.Value]bool{})
	rightPath, rightOK := accessPath(right.Value, right.Root, map[ssa.Value]bool{})
	if !leftOK || !rightOK {
		return IdentityProof{Proof{State: EvidenceUnknown, Reason: EvidenceUnavailable}}
	}
	if len(leftPath) == len(rightPath) && slicesEqual(leftPath, rightPath) {
		return IdentityProof{Proof{State: EvidenceProven, Reason: EvidenceSameAccessPath, Provenance: EvidenceFromLocalSSA}}
	}
	return IdentityProof{Proof{State: EvidenceDisproven, Reason: EvidenceNotFound, Provenance: EvidenceFromLocalSSA}}
}

func slicesEqual(left, right []string) bool {
	for index := range left {
		if left[index] != right[index] {
			return false
		}
	}
	return true
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
	var query EvidenceQuery
	return query.Completion(request)
}

// Completion proves and memoizes a lifecycle-completion request.
func (query *EvidenceQuery) Completion(request CompletionRequest) CompletionProof {
	key := completionEvidenceKey{
		instruction: request.Instruction,
		target:      request.Target,
		methods:     completionMethodsKey(request.Methods),
		modes:       request.Modes,
	}
	if proof, ok := query.completions[key]; ok {
		return proof
	}
	proof := proveCompletion(request)
	if query.completions == nil {
		query.completions = make(map[completionEvidenceKey]CompletionProof)
	}
	query.completions[key] = proof
	return proof
}

func completionMethodsKey(methods []string) string {
	if len(methods) == 1 {
		return methods[0]
	}
	return strings.Join(methods, "\x00")
}

func proveCompletion(request CompletionRequest) CompletionProof {
	if request.Instruction == nil || request.Target == nil || len(request.Methods) == 0 || request.Modes == 0 {
		return CompletionProof{Proof{State: EvidenceUnknown, Reason: EvidenceUnavailable}}
	}
	for _, method := range request.Methods {
		if request.Modes&CompletionDeferred != 0 && DeferredClosureCalls(request.Instruction, method, request.Target) {
			return completionProof(EvidenceDeferredCompletion, method)
		}
		if request.Modes&CompletionBeforeBranch != 0 && ClosureCallsMethodBeforeBranch(request.Instruction, method, request.Target) {
			return completionProof(EvidenceCompletionBeforeBranch, method)
		}
		if request.Modes&CompletionByHelper != 0 && CallCallsMethodOnArgumentOnEveryReturn(request.Instruction, method, request.Target) {
			return completionProof(EvidenceHelperCompletion, method)
		}
		if request.Modes&CompletionInStartedClosure != 0 && StartedClosureCallsMethodOnEveryReturn(request.Instruction, method, request.Target) {
			return completionProof(EvidenceStartedCompletion, method)
		}
		if request.Modes&CompletionByStartedHelper != 0 && StartedClosureCallsMethodViaHelper(request.Instruction, method, request.Target) {
			return completionProof(EvidenceStartedHelperCompletion, method)
		}
		if request.Modes&CompletionInClosure != 0 && ClosureCallsMethod(request.Instruction, method, request.Target) {
			return completionProof(EvidenceClosureCompletion, method)
		}
	}
	if completionEvidenceUnavailable(request) {
		return CompletionProof{Proof{State: EvidenceUnknown, Reason: EvidenceUnavailable}}
	}
	return CompletionProof{Proof{State: EvidenceDisproven, Reason: EvidenceNotFound, Provenance: EvidenceFromLocalSSA}}
}

func completionProof(reason EvidenceReason, method string) CompletionProof {
	return CompletionProof{Proof{State: EvidenceProven, Reason: reason, Method: method, Provenance: EvidenceFromLocalSSA}}
}

func completionEvidenceUnavailable(request CompletionRequest) bool {
	common, _, function := calledFunction(request.Instruction)
	if request.Modes&(CompletionDeferred|CompletionInClosure|CompletionBeforeBranch|CompletionInStartedClosure|CompletionByStartedHelper) != 0 &&
		(function == nil || len(function.Blocks) == 0) {
		return true
	}
	return request.Modes&CompletionByHelper != 0 && (common == nil || common.StaticCallee() == nil || len(common.StaticCallee().Blocks) == 0)
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
	var query EvidenceQuery
	return query.OwnershipTransfer(request)
}

// OwnershipTransfer proves and memoizes an ownership-transfer request.
func (query *EvidenceQuery) OwnershipTransfer(request OwnershipTransferRequest) OwnershipTransferProof {
	key := transferEvidenceKey{instruction: request.Instruction, value: request.Value, modes: request.Modes}
	if proof, ok := query.transfers[key]; ok {
		return proof
	}
	proof := proveOwnershipTransfer(request)
	if query.transfers == nil {
		query.transfers = make(map[transferEvidenceKey]OwnershipTransferProof)
	}
	query.transfers[key] = proof
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
