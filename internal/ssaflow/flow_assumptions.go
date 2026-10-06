package ssaflow

import (
	"go/token"
	"go/types"

	proofs "github.com/kojah/gohawk/internal/proof"
	cfg "github.com/kojah/gohawk/internal/ssaflow/cfg"
	"golang.org/x/tools/go/ssa"
)

// Assumed successor evidence narrows existing feasible edges only for the
// exact value or direct field covered by the caller's nonnil/type assumption.
// Identity and nilness share the caller allowance; interrupted queries never
// establish an assumption or a pruned path.

// assumedSuccessorsWithin narrows already-feasible successors by the assumption
// that value is non-nil at the branch and, when its concrete type is
// known, that a comma-ok assertion of a type that concrete type satisfies
// succeeds.
func assumedSuccessorsWithin(
	successors []*ssa.BasicBlock, block *ssa.BasicBlock, value ssa.Value, concrete types.Type, budget *proofs.SearchBudget,
) []*ssa.BasicBlock {
	if value == nil || len(block.Succs) != 2 || len(block.Instrs) == 0 {
		return successors
	}
	branch, ok := block.Instrs[len(block.Instrs)-1].(*ssa.If)
	if !ok {
		return successors
	}
	// kubernetes closes a writer through a helper that asserts io.Closer on
	// it; for a caller passing an *os.File the assertion holds, so the arm
	// without the close is not a path the file takes.
	// https://github.com/kubernetes/kubernetes/blob/e72c2715ade37738aa5c029e8de5285cbe1c9441/test/e2e/storage/podlogs/podlogs.go#L360-L364
	if concrete != nil {
		holds := assertionHoldsWithin(branch.Cond, value, concrete, budget)
		if budget.Exhausted() {
			return nil
		}
		if holds {
			return cfg.KeepSuccessorWithin(successors, block.Succs[0], budget)
		}
	}
	comparison, ok := branch.Cond.(*ssa.BinOp)
	if !ok || comparison.Op != token.EQL && comparison.Op != token.NEQ {
		return successors
	}
	// Use identity, not general data derivation: an error returned by a call
	// that received the context may derive from the constructor result without
	// being the cleanup function whose non-nilness is known. A nil-comparable
	// field loaded from the value, such as resp.Body, is assumed non-nil with
	// it: when the field is nil there is nothing to release, so the branch
	// that skips the release on that account proves no leak. autobrr's shared
	// drain helper guards on both:
	// https://github.com/autobrr/autobrr/blob/31a08a55a4539d846f1c68bfef43798659e05596/pkg/sharedhttp/http.go#L109-L114
	comparesNil := assumedNilPairWithin(comparison.X, comparison.Y, value, budget)
	if budget.Exhausted() {
		return nil
	}
	if !comparesNil {
		comparesNil = assumedNilPairWithin(comparison.Y, comparison.X, value, budget)
	}
	if budget.Exhausted() {
		return nil
	}
	if !comparesNil {
		return successors
	}
	nonNil := block.Succs[0]
	if comparison.Op == token.EQL {
		nonNil = block.Succs[1]
	}
	return cfg.KeepSuccessorWithin(successors, nonNil, budget)
}

func assumedNilPairWithin(operand, other, value ssa.Value, budget *proofs.SearchBudget) bool {
	if !assumedNonNilWithin(operand, value, budget) || budget.Exhausted() {
		return false
	}
	return DefinitelyNilWithin(other, budget)
}

// assertionHoldsWithin reports whether condition is the ok result of a comma-ok
// assertion of the assumed value to a type its concrete type satisfies.
func assertionHoldsWithin(condition, value ssa.Value, concrete types.Type, budget *proofs.SearchBudget) bool {
	okResult, ok := condition.(*ssa.Extract)
	if !ok || okResult.Index != 1 {
		return false
	}
	assertion, ok := okResult.Tuple.(*ssa.TypeAssert)
	if !ok || !assertion.CommaOk {
		return false
	}
	if !StructurallyIdenticalWithin(assertion.X, value, budget) || budget.Exhausted() {
		return false
	}
	return budget.Spend() && types.AssignableTo(concrete, assertion.AssertedType)
}

// assumedNonNilWithin reports whether operand is the assumed value itself or a
// field loaded directly from it.
func assumedNonNilWithin(operand, value ssa.Value, budget *proofs.SearchBudget) bool {
	if StructurallyIdenticalWithin(operand, value, budget) {
		return true
	}
	if budget.Exhausted() {
		return false
	}
	load, ok := operand.(*ssa.UnOp)
	if !ok || load.Op != token.MUL {
		return false
	}
	field, ok := load.X.(*ssa.FieldAddr)
	return ok && StructurallyIdenticalWithin(field.X, value, budget)
}
