package ssaflow

import "golang.org/x/tools/go/ssa"

// ProveIdentity reports whether two values denote corresponding access paths
// beneath roots that the caller has already established as equivalent.
func ProveIdentity(left, right AccessPath) IdentityProof {
	var evidence LocalEvidence
	return evidence.Identity(left, right)
}

// Identity proves and memoizes whether two values denote corresponding access
// paths beneath roots already established as equivalent by the caller.
func (evidence *LocalEvidence) Identity(left, right AccessPath) IdentityProof {
	key := identityEvidenceKey{left.Value, left.Root, right.Value, right.Root}
	if proof, ok := evidence.identities[key]; ok {
		return proof
	}
	proof := proveIdentity(left, right)
	if evidence.identities == nil {
		evidence.identities = make(map[identityEvidenceKey]IdentityProof)
	}
	evidence.identities[key] = proof
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
