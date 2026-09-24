package ssaflow

// EvidenceReason identifies the concrete SSA relationship that established a
// proof. String supplies stable trace codes; numeric values are not wire identifiers.
type EvidenceReason uint8

const (
	EvidenceNone EvidenceReason = iota
	EvidenceNotFound
	EvidenceUnavailable

	EvidenceSameValue
	// EvidenceSharedSlot and the other alias reasons name the rule behind
	// a may-alias answer; see AliasProof.
	EvidenceSharedSlot
	EvidenceUnknownPointee
	EvidenceDisjointPaths
	EvidenceDisjointObjects
	EvidenceUnescapedLocal
	EvidenceStructuralWalk
	EvidenceSameAccessPath

	// EvidenceDeferredCompletion and the other completion reasons name the
	// launch form of the callee that ran the lifecycle method; nested launches
	// report the outermost form.
	EvidenceDeferredCompletion
	EvidenceCalledCompletion
	EvidenceStartedCompletion
	EvidenceCallbackCompletion
	// EvidenceBudgetExhausted marks a question abandoned before it could be
	// decided, so a caller can tell "not proven" from "not searched".
	EvidenceBudgetExhausted
	// EvidenceCompletionInCycle: the only completion found lies inside a
	// cycle, so it is not on every return, but which element or iteration
	// it settles is decided by iteration; the search declines to call that
	// a missing completion.
	EvidenceCompletionInCycle
	EvidenceHelperInvocation
	EvidenceReturnedDeferredCleanup

	// EvidenceStorageNotLocal and the other storage give-up reasons say where
	// a point-in-time query of local storage stopped. None of them means the
	// location was empty, unequal, or released; they let a reader see which
	// write, use, or merge defeated the proof instead of a bare "unavailable".
	EvidenceStorageNotLocal
	EvidenceStorageOutsideFunction
	EvidenceStorageAddressEscapes
	EvidenceStorageWriteThroughAlias
	EvidenceStoragePartialWrite
	EvidenceStorageConflictingWrites
	EvidenceStorageNoReachingWrite
	EvidenceStorageWriteInCycle
	EvidenceStorageWriteAfterObservation
	EvidenceStorageProjectionNotLoad
	EvidenceStorageProjectionModified
	EvidenceStoredValuesDiffer

	// EvidenceSummaryBodyUnavailable and EvidenceSummaryRecursive say why a
	// callee could not be summarized: an opaque body or dispatch, or a callee
	// already on the active call path.
	EvidenceSummaryBodyUnavailable
	EvidenceSummaryRecursive

	EvidenceStoredInField
	EvidenceOwnerStoredInField
	EvidenceStoredInGlobal
	EvidenceStoredInEnclosingScope
	EvidenceOwnerStoredInExternalField
	EvidenceStoredInOwnedMap
	EvidenceSentToReceiver
	EvidenceCapturedByClosure
	EvidenceCallResultStoredInField
	EvidenceTransferredToReturnedOwner
	EvidenceTransferredToReceiver
	EvidenceTransferredToLifecycleOwner
	EvidenceCallEffectsKnown
	evidenceReasonCount
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

// AliasProof records whether two values may refer to one object, and the
// reason. Aliases true is possibility, never identity; it includes an object
// carried around a loop's back edge, which only a must-answer filters. A
// false answer is a claim of disjointness, and its reason says which rule
// made it: two paths of one object, an unescaped local against something it
// was never stored into, or two objects the function's flow never connects.
type AliasProof struct {
	Aliases    bool
	Reason     EvidenceReason
	Provenance EvidenceProvenance
}

// CompletionProof records evidence that a lifecycle method runs under the
// path guarantees selected by an analyzer.
type CompletionProof struct {
	Proof
	// Path is the joined access path beneath the target on which the
	// completing calls were made, empty for the target itself, and is
	// meaningful only when PathKnown holds: every completing call the
	// coverage relied on was on a mapped local at one static path. It is
	// unknown when a receiver was derived from the target without a static
	// path, when different calls settled different paths, or when the
	// completion came through a summary or an invoked callback. A caller
	// exporting the completion as a claim about the target's contents must
	// require it; a caller settling the target itself may ignore it.
	Path      string
	PathKnown bool
}

// OwnershipTransferProof records evidence that an obligation moved to an
// owner accepted by an analyzer.
type OwnershipTransferProof struct{ Proof }
