package resourcelifetime

import (
	"go/token"
	"slices"

	"github.com/kojah/gohawk/internal/analysis/passes/lifecyclefacts"
	"github.com/kojah/gohawk/internal/engine/heapmodel"
	"github.com/kojah/gohawk/internal/engine/lifecycle"
	proofs "github.com/kojah/gohawk/internal/engine/proof"
	"github.com/kojah/gohawk/internal/engine/ssaflow"
	ssacall "github.com/kojah/gohawk/internal/engine/ssaflow/calls"
	cfg "github.com/kojah/gohawk/internal/engine/ssaflow/cfg"
	"github.com/kojah/gohawk/internal/engine/syntax"
	"golang.org/x/tools/go/ssa"
)

// Ambiguous cleanup proves only that a possible acquisition origin reaches a
// cleanup receiver or a merged argument whose own cleanup is established.
// It cannot settle the caller's exact resource; cutoff supplies uncertainty
// rather than a completed absence of possible cleanup.
func (analysis *resourceAnalysis) proveAmbiguousCleanupWithin(
	instruction ssa.Instruction, common *ssa.CallCommon, budget *proofs.SearchBudget,
) resourceProof {
	if analysis.optional.Proven() || common == nil {
		return resourceProof{State: proofs.EvidenceDisproven, Reason: resourceReasonUntouched}
	}
	// A merged receiver or escaped owner projection may still select this
	// acquisition. Exact storage identity cannot establish that relationship,
	// but absence of a match is not proof that the resource stays open. A body
	// read helper can consume a response before its explicit Body.Close:
	// https://github.com/james-6-23/codex2api/blob/4f96afe95bb16132347f4ab74e63b0b1fa0f778b/auth/claude_api_key.go#L94-L99
	if slices.Contains(analysis.contract.cleanup, ssaflow.CallName(common)) {
		derived := heapmodel.ValueDerivesFromWithin(ssaflow.CallReceiver(common), analysis.resource, budget)
		proof := carriedValueProof(derived, resourceReasonAmbiguousCleanupValue, budget)
		if proof.State != proofs.EvidenceDisproven {
			return proof
		}
	}
	return analysis.proveAmbiguousHelperCleanupWithin(instruction, common, budget)
}

// A cleanup helper may receive a projection of a merged owner. Proving cleanup
// of its actual argument does not prove which acquisition was released, but
// that ambiguous identity cannot establish a leak either. Direct Close calls
// have the same unknown boundary above; read-only helpers do not qualify.
// https://github.com/mr-karan/doggo/blob/7f6b105f240e562a5c7659976b1c16b683c25385/pkg/resolvers/doh.go#L143-L168
func (analysis *resourceAnalysis) proveAmbiguousHelperCleanupWithin(
	instruction ssa.Instruction, common *ssa.CallCommon, budget *proofs.SearchBudget,
) resourceProof {
	for _, argument := range common.Args {
		if !budget.Spend() {
			return carriedValueProof(false, resourceReasonUntouched, budget)
		}
		if !mergedCleanupArgument(argument) {
			continue
		}
		derived := heapmodel.ValueDerivesFromWithin(argument, analysis.resource, budget)
		if resourceFlowExhausted(budget) {
			return carriedValueProof(false, resourceReasonUntouched, budget)
		}
		if !derived {
			continue
		}
		for _, method := range analysis.contract.cleanup {
			if !budget.Spend() {
				return carriedValueProof(false, resourceReasonUntouched, budget)
			}
			completion := lifecycle.CompletionRequest{
				Instruction: instruction, Target: argument, Methods: []string{method},
				Coverage: lifecycle.CoverageEveryReturn, Budget: budget,
			}
			proof := analysis.evidence.Prove(lifecyclefacts.EvidenceRequest{
				Instruction: instruction, Target: argument, Completion: &completion, SelectMask: releaseMask(instruction, argument, method),
			})
			if proof.Reason == proofs.EvidenceBudgetExhausted {
				return resourceProof{State: proofs.EvidenceUnknown, Reason: resourceReasonBudgetExhausted}
			}
			candidate := carriedValueProof(proof.Proven(), resourceReasonAmbiguousHelperCleanupValue, budget)
			if candidate.State != proofs.EvidenceDisproven {
				return candidate
			}
		}
	}
	return carriedValueProof(false, resourceReasonUntouched, budget)
}

// Only merged values are ambiguous here. A helper that closes an overwritten
// field, or conditionally closes its exact argument, must remain diagnostic.
func mergedCleanupArgument(argument ssa.Value) bool {
	if _, merged := argument.(*ssa.Phi); merged {
		return true
	}
	load, loaded := argument.(*ssa.UnOp)
	if !loaded || load.Op != token.MUL {
		return false
	}
	field, projected := load.X.(*ssa.FieldAddr)
	if !projected {
		return false
	}
	_, merged := field.X.(*ssa.Phi)
	return merged
}

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
	instruction ssa.Instruction, common *ssa.CallCommon, budget *proofs.SearchBudget,
) resourceProof {
	if common == nil {
		return resourceProof{State: proofs.EvidenceDisproven, Reason: resourceReasonUntouched}
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
		if proof.State == proofs.EvidenceUnknown {
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
	if completion.Reason == proofs.EvidenceBudgetExhausted {
		return resourceProof{State: proofs.EvidenceUnknown, Reason: resourceReasonBudgetExhausted}
	}
	return carriedValueProof(completion.Proven(), resourceReasonPairedErrorHelperCleanup, budget)
}

// Identity, not derivation: a wrapped error is a different value. Result slots
// zero and one preserve the existing paired-acquisition contract; this query
// does not reinterpret an arbitrary factory's last error as the paired slot.
func (analysis *resourceAnalysis) proveCorrelatedErrorWithin(
	call ssa.Instruction, argument ssa.Value, budget *proofs.SearchBudget,
) resourceProof {
	if !syntax.IsErrorType(argument.Type()) {
		return carriedValueProof(false, resourceReasonUntouched, budget)
	}
	resource := ssacall.CallResultWithin(analysis.acquisition, 0, budget)
	if resourceFlowExhausted(budget) {
		return carriedValueProof(false, resourceReasonUntouched, budget)
	}
	if resource == analysis.resource {
		paired := ssacall.CallResultWithin(analysis.acquisition, 1, budget)
		proof := carriedValueProof(argument == paired, resourceReasonPairedErrorHelperCleanup, budget)
		if proof.State != proofs.EvidenceDisproven {
			return proof
		}
	}
	following := cfg.InstructionsReachableAfterWithin(call, budget)
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
			if proof.State != proofs.EvidenceDisproven {
				return proof
			}
		}
	}
	return carriedValueProof(false, resourceReasonUntouched, budget)
}

// Prior deferred completion asks the ordinary evidence engine with anywhere
// coverage. A complete witness supplies only may-release uncertainty; this
// census never settles an acquisition or duplicates the resource flow.

// A defer registered on every path to acquisition may drain a captured closer
// slice populated later. The deferred body decides at return how many entries
// it closes, so requiring every-return cleanup would lose this uncertainty.
// https://github.com/bazel-contrib/rules_img/blob/af5e1452f0cb68b1ed64dc6095210f1eb4ae625f/img_tool/cmd/mtree/mtree.go#L110-L128
func (analysis *resourceAnalysis) proveDeferredBeforeAcquisitionWithin(call *ssa.Call, budget *proofs.SearchBudget) resourceProof {
	for instruction := range ssaflow.InstructionsWithin(call.Parent(), budget) {
		deferred, ok := instruction.(*ssa.Defer)
		if !ok || !cfg.InstructionDominates(deferred, call) {
			continue
		}
		completion := lifecycle.CompletionRequest{
			Instruction: deferred, Target: analysis.resource, Methods: analysis.contract.cleanup,
			Coverage: lifecycle.CoverageAnywhere, Budget: budget,
		}
		proof := analysis.evidence.Prove(lifecyclefacts.EvidenceRequest{Instruction: deferred, Target: analysis.resource, Completion: &completion})
		action, reason := releaseLabel(proof)
		// An interrupted nested search cannot complete a negative census even
		// when the caller pool still has room. Other completed unavailable forms
		// retain the prior model's policy and may be recognized by later classifiers.
		if reason == resourceReasonBudgetExhausted || resourceFlowExhausted(budget) {
			return resourceProof{State: proofs.EvidenceUnknown, Reason: resourceReasonBudgetExhausted}
		}
		if action == actionSettled {
			return carriedValueProof(true, resourceReasonPriorDeferMayRelease, budget)
		}
	}
	return carriedValueProof(false, resourceReasonUntouched, budget)
}
