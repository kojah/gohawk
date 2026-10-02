package ssaflow

import "golang.org/x/tools/go/ssa"

// StructurallyIdentical proves value identity from the value graph alone:
// one SSA value seen through wrappers, a phi whose alternatives all agree,
// or equal address selections. Distinct loads stay unknown here, even from
// the same address.
func StructurallyIdentical(left, right ssa.Value) bool {
	return StructurallyIdenticalWithin(left, right, nil)
}

// StructurallyIdenticalWithin charges structural comparisons and reaching-value
// visits to budget. A cutoff supplies no identity evidence; false remains
// unproved, never inequality. A nil budget retains the default structural policy.
func StructurallyIdenticalWithin(left, right ssa.Value, budget *SearchBudget) bool {
	if !budget.Spend() {
		return false
	}
	if left == nil || right == nil {
		return false
	}
	if left == right {
		return true
	}
	forms := TransparentChangeInterface | TransparentChangeType | TransparentConvert | TransparentMakeInterface
	return NewReachingWalk(forms).Within(budget).Every(left, func(_ ReachingWalk, left ssa.Value) bool {
		return NewReachingWalk(forms).Within(budget).Every(right, func(_ ReachingWalk, right ssa.Value) bool {
			if left == right {
				return true
			}
			switch left := left.(type) {
			case *ssa.FieldAddr:
				other, ok := right.(*ssa.FieldAddr)
				return ok && left.Field == other.Field && StructurallyIdenticalWithin(left.X, other.X, budget)
			case *ssa.IndexAddr:
				other, ok := right.(*ssa.IndexAddr)
				if !ok || !StructurallyIdenticalWithin(left.X, other.X, budget) {
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
	return ProveIdentityWithin(left, right, nil)
}

// ProveIdentityWithin shares budget across structural identity, both path
// searches and step comparison. Exhaustion is an unknown proof with the budget
// reason, never differing paths. Roots must already be established as equivalent.
func ProveIdentityWithin(left, right AccessPath, budget *SearchBudget) IdentityProof {
	unknown := func() IdentityProof {
		reason := EvidenceUnavailable
		if budget.Exhausted() {
			reason = EvidenceBudgetExhausted
		}
		return IdentityProof{Proof{State: EvidenceUnknown, Reason: reason}}
	}
	if left.Value == nil || right.Value == nil {
		return unknown()
	}
	if StructurallyIdenticalWithin(left.Value, right.Value, budget) {
		return IdentityProof{Proof{State: EvidenceProven, Reason: EvidenceSameValue, Provenance: EvidenceFromLocalSSA}}
	}
	if budget.Exhausted() {
		return unknown()
	}
	leftPath, leftOK := AccessPathStepsWithin(left.Value, left.Root, budget)
	if !leftOK || budget.Exhausted() {
		return unknown()
	}
	rightPath, rightOK := AccessPathStepsWithin(right.Value, right.Root, budget)
	if !rightOK || budget.Exhausted() {
		return unknown()
	}
	if sameAccessPathSteps(leftPath, rightPath, budget) {
		return IdentityProof{Proof{State: EvidenceProven, Reason: EvidenceSameAccessPath, Provenance: EvidenceFromLocalSSA}}
	}
	if budget.Exhausted() {
		return unknown()
	}
	return IdentityProof{Proof{State: EvidenceDisproven, Reason: EvidenceNotFound, Provenance: EvidenceFromLocalSSA}}
}
