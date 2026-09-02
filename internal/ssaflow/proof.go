package ssaflow

// EvidenceReason identifies the concrete SSA relationship that established a
// proof. Reason values are stable diagnostic vocabulary suitable for tracing.
type EvidenceReason string

const (
	EvidenceNone        EvidenceReason = ""
	EvidenceNotFound    EvidenceReason = "evidence-not-found"
	EvidenceUnavailable EvidenceReason = "evidence-unavailable"

	EvidenceSameValue      EvidenceReason = "same-value"
	EvidenceSameAccessPath EvidenceReason = "same-access-path"

	EvidenceDeferredCompletion            EvidenceReason = "deferred-completion"
	EvidenceDeferredCallback              EvidenceReason = "deferred-callback-completion"
	EvidenceDeferredArgumentCompletion    EvidenceReason = "deferred-argument-completion"
	EvidenceDeferredHelperCallback        EvidenceReason = "deferred-helper-callback-completion"
	EvidenceClosureCompletion             EvidenceReason = "closure-completion"
	EvidenceCompletionBeforeBranch        EvidenceReason = "completion-before-branch"
	EvidenceCalledCompletionBeforeBranch  EvidenceReason = "called-closure-completion-before-branch"
	EvidenceCalledCompletionOnEveryReturn EvidenceReason = "called-closure-completion-on-every-return"
	EvidenceHelperCompletion              EvidenceReason = "helper-completion"
	EvidenceStartedCompletion             EvidenceReason = "started-completion"
	EvidenceStartedHelperCompletion       EvidenceReason = "started-helper-completion"
	EvidenceHelperInvocation              EvidenceReason = "helper-invocation"
	EvidenceReturnedDeferredCleanup       EvidenceReason = "returned-deferred-cleanup"

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
