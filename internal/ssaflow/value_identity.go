package ssaflow

import "golang.org/x/tools/go/ssa"

// StructurallyIdentical proves value identity from the value graph alone:
// one SSA value seen through wrappers, a phi whose alternatives all agree,
// or equal address selections. Distinct loads stay unknown here, even from
// the same address.
func StructurallyIdentical(left, right ssa.Value) bool {
	if left == nil || right == nil {
		return false
	}
	if left == right {
		return true
	}
	forms := TransparentChangeInterface | TransparentChangeType | TransparentConvert | TransparentMakeInterface
	return NewReachingWalk(forms).Every(left, func(_ ReachingWalk, left ssa.Value) bool {
		return NewReachingWalk(forms).Every(right, func(_ ReachingWalk, right ssa.Value) bool {
			if left == right {
				return true
			}
			switch left := left.(type) {
			case *ssa.FieldAddr:
				other, ok := right.(*ssa.FieldAddr)
				return ok && left.Field == other.Field && StructurallyIdentical(left.X, other.X)
			case *ssa.IndexAddr:
				other, ok := right.(*ssa.IndexAddr)
				if !ok || !StructurallyIdentical(left.X, other.X) {
					return false
				}
				a, aOK := ConstantIndex(left.Index)
				b, bOK := ConstantIndex(other.Index)
				return aOK && bOK && a == b || left.Index == other.Index
			}
			return false
		})
	})
}

// ProveIdentity reports whether two values denote corresponding access paths
// beneath roots that the caller has already established as equivalent.
func ProveIdentity(left, right AccessPath) IdentityProof {
	// Identity proves and memoizes whether two values denote corresponding access
	// paths beneath roots already established as equivalent by the caller.
	if left.Value == nil || right.Value == nil {
		return IdentityProof{Proof{State: EvidenceUnknown, Reason: EvidenceUnavailable}}
	}
	if StructurallyIdentical(left.Value, right.Value) {
		return IdentityProof{Proof{State: EvidenceProven, Reason: EvidenceSameValue, Provenance: EvidenceFromLocalSSA}}
	}
	leftPath, leftOK := AccessPathSteps(left.Value, left.Root, map[ssa.Value]bool{})
	rightPath, rightOK := AccessPathSteps(right.Value, right.Root, map[ssa.Value]bool{})
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
