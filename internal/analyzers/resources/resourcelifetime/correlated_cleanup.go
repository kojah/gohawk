package resourcelifetime

import (
	"go/token"

	"github.com/kojah/gohawk/internal/lifecycle"
	"github.com/kojah/gohawk/internal/ssaflow"
	"github.com/kojah/gohawk/internal/syntax"
	"golang.org/x/tools/go/ssa"
)

// Correlated-error cleanup supplies only possible release, never settlement.
// Argument identity, paired-result lookup, later nil tests and the cleanup
// witness share one allowance; an interrupted request remains unknown.

// A helper can condition cleanup on an error it receives beside the
// resource. Unconditional completion cannot represent that relation, so a
// witnessed cleanup plus a correlated error is uncertainty, not proof of
// either release or a leak. The error is correlated when it is the one
// paired with this acquisition, or when the caller itself branches on it
// being nil after the call: then the caller's own paths split on the same
// value the helper's cleanup does, as in closeOnError(f, err) followed by
// if err != nil { return nil, err }; return f, nil.
// https://github.com/h44z/wg-portal/blob/eb44c8c4ff120f34c26b2415c47560f4fba0603c/internal/lowlevel/mikrotik.go#L267-L280
// Helpers that merely inspect the pair, receive an error the caller never
// tests again, or condition cleanup on a flag stay visible: a flag the
// caller does not branch on leaves the unreleased path feasible.
func (analysis *resourceAnalysis) provePairedErrorCleanupWithin(
	instruction ssa.Instruction, common *ssa.CallCommon, budget *ssaflow.SearchBudget,
) resourceProof {
	if common == nil {
		return resourceProof{State: ssaflow.EvidenceDisproven, Reason: resourceReasonUntouched}
	}
	// Correlation belongs to this exact acquisition. A wrapper, projection or
	// possible origin cannot use its error to discharge a different resource.
	exact := false
	for _, argument := range common.Args {
		if !budget.Spend() {
			return carriedValueProof(false, resourceReasonUntouched, budget)
		}
		if argument == analysis.resource {
			exact = true
			break
		}
	}
	if !exact {
		return carriedValueProof(false, resourceReasonUntouched, budget)
	}
	correlated := false
	for _, argument := range common.Args {
		if !budget.Spend() {
			return carriedValueProof(false, resourceReasonUntouched, budget)
		}
		proof := analysis.proveCorrelatedErrorWithin(instruction, argument, budget)
		if proof.State == ssaflow.EvidenceUnknown {
			return proof
		}
		if proof.Proven() {
			correlated = true
			break
		}
	}
	if !correlated {
		return carriedValueProof(false, resourceReasonUntouched, budget)
	}
	// Anywhere coverage asks only whether cleanup can occur in the helper.
	// Even with a correlated error it cannot establish every-return release;
	// the caller uses this witness as uncertainty, never a settled obligation.
	completion := lifecycle.ProveCompletion(lifecycle.CompletionRequest{
		Instruction: instruction, Target: analysis.resource, Methods: analysis.contract.cleanup,
		Coverage: lifecycle.CoverageAnywhere, Budget: budget,
	})
	if completion.Reason == ssaflow.EvidenceBudgetExhausted {
		return resourceProof{State: ssaflow.EvidenceUnknown, Reason: resourceReasonBudgetExhausted}
	}
	return carriedValueProof(completion.Proven(), resourceReasonPairedErrorHelperCleanup, budget)
}

// Identity, not derivation: a wrapped error is a different value. Result slots
// zero and one preserve the existing paired-acquisition contract; this query
// does not reinterpret an arbitrary factory's last error as the paired slot.
func (analysis *resourceAnalysis) proveCorrelatedErrorWithin(
	call ssa.Instruction, argument ssa.Value, budget *ssaflow.SearchBudget,
) resourceProof {
	if !syntax.IsErrorType(argument.Type()) {
		return carriedValueProof(false, resourceReasonUntouched, budget)
	}
	resource := ssaflow.CallResultWithin(analysis.acquisition, 0, budget)
	if resourceFlowExhausted(budget) {
		return carriedValueProof(false, resourceReasonUntouched, budget)
	}
	if resource == analysis.resource {
		paired := ssaflow.CallResultWithin(analysis.acquisition, 1, budget)
		proof := carriedValueProof(argument == paired, resourceReasonPairedErrorHelperCleanup, budget)
		if proof.State != ssaflow.EvidenceDisproven {
			return proof
		}
	}
	following := ssaflow.InstructionsReachableAfterWithin(call, budget)
	// The shared census may return a prefix at cutoff. It cannot complete the
	// question or publish a correlation from an interrupted request.
	if resourceFlowExhausted(budget) {
		return carriedValueProof(false, resourceReasonUntouched, budget)
	}
	for _, instruction := range following {
		if !budget.Spend() {
			return carriedValueProof(false, resourceReasonUntouched, budget)
		}
		branch, ok := instruction.(*ssa.If)
		if !ok {
			continue
		}
		comparison, ok := branch.Cond.(*ssa.BinOp)
		if !ok || comparison.Op != token.EQL && comparison.Op != token.NEQ {
			continue
		}
		var other ssa.Value
		if comparison.X == argument {
			other = comparison.Y
		} else if comparison.Y == argument {
			other = comparison.X
		}
		if other != nil {
			nilValue := ssaflow.DefinitelyNilWithin(other, budget)
			proof := carriedValueProof(nilValue, resourceReasonPairedErrorHelperCleanup, budget)
			if proof.State != ssaflow.EvidenceDisproven {
				return proof
			}
		}
	}
	return carriedValueProof(false, resourceReasonUntouched, budget)
}
