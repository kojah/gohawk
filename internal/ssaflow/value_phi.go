package ssaflow

import (
	"iter"

	"golang.org/x/tools/go/ssa"
)

// Phi mechanics that analyzers need beyond the reaching-value folds: pairing
// each incoming edge with the predecessor block it arrives from, and asking
// whether a merge carries one exact value. Analyzers must not range over
// phi edges themselves; the architecture tests enforce that so the fan-out
// and its bounds guard live in one place.

// PhiIncoming yields each edge of phi with the predecessor block it comes
// from. An edge without a matching predecessor, which malformed SSA could
// produce, is skipped.
func PhiIncoming(phi *ssa.Phi) iter.Seq2[*ssa.BasicBlock, ssa.Value] {
	return func(yield func(*ssa.BasicBlock, ssa.Value) bool) {
		predecessors := phi.Block().Preds
		for index, edge := range phi.Edges {
			if index >= len(predecessors) {
				return
			}
			if !yield(predecessors[index], edge) {
				return
			}
		}
	}
}

// PhiMergesValue reports whether some edge of phi is value under SameValue.
func PhiMergesValue(phi *ssa.Phi, value ssa.Value) bool {
	for _, edge := range phi.Edges {
		if SameValue(edge, value) {
			return true
		}
	}
	return false
}

// PhiEdgeCount returns how many edges phi merges.
func PhiEdgeCount(phi *ssa.Phi) int {
	return len(phi.Edges)
}

// IdentitySource returns the operand a value is an alias of for identity
// resolution: every transparent wrapper, and a load, because `*p` names the
// same lock or context as the cell p holds. This is sound only for identity
// questions. Ownership and lifecycle proofs must not peel loads, since the
// loaded value is what was stored at p, not p itself, so this helper is
// deliberately not a ReachingWalk form.
func IdentitySource(value ssa.Value) (ssa.Value, bool) {
	if inner, ok := UnwrapTransparentValue(
		value,
		TransparentChangeInterface|TransparentChangeType|TransparentConvert|TransparentMakeInterface,
	); ok {
		return inner, true
	}
	if loaded, ok := value.(*ssa.UnOp); ok {
		return loaded.X, true
	}
	return nil, false
}
