package resourcelifetime

import (
	"go/token"
	"go/types"
	"slices"

	"github.com/kojah/gohawk/internal/heapmodel"
	"github.com/kojah/gohawk/internal/lifecycle"
	"github.com/kojah/gohawk/internal/passes/lifecyclefacts"
	"github.com/kojah/gohawk/internal/ssaflow"
	"golang.org/x/tools/go/ssa"
)

// Guarded Body cleanup requires a stable exact captured response and excludes
// visible pointer exposure. The repeated Body loads supply may-cleanup only;
// capture, exposure and coverage searches share one request allowance.

// A called literal may own HTTP body cleanup while guarding a distinct load
// of the captured response's Body. Repeated loads do not establish equality,
// so this is uncertainty, not completion. Require the exact unchanged capture
// and a body-nil guard alone; Boolean conditions and visible pointer escapes
// or replacement cannot establish even this narrow boundary.
// https://github.com/openkruise/kruise-game/blob/16a0418780d8abd3ee871448116bbc5dc1e98d48/test/e2e/framework/framework.go#L549-L570
func (analysis *resourceAnalysis) proveGuardedCapturedBodyWithin(
	instruction ssa.Instruction, closure *ssa.MakeClosure, budget *ssaflow.SearchBudget,
) resourceProof {
	missing := resourceProof{State: ssaflow.EvidenceDisproven, Reason: resourceReasonEvidenceNotFound}
	if _, called := instruction.(*ssa.Call); !called || analysis.contract.family != "http" {
		return missing
	}
	function, _ := closure.Fn.(*ssa.Function)
	for _, binding := range ssaflow.ClosureBindingPairs(function, closure) {
		if !budget.Spend() {
			return capturedBodyResult(false, budget)
		}
		stored := heapmodel.NewStorage(budget).StableContent(binding.Binding, instruction)
		if stored.Reason == ssaflow.EvidenceBudgetExhausted || resourceFlowExhausted(budget) {
			return resourceProof{State: ssaflow.EvidenceUnknown, Reason: resourceReasonBudgetExhausted}
		}
		if !stored.Proven() || stored.Value != analysis.resource {
			continue
		}
		if !responseCaptureUnmodified(analysis.function, analysis.resource, binding.Binding, closure, budget) ||
			!responseCaptureUnmodified(function, binding.Free, binding.Free, nil, budget) {
			continue
		}
		coverage := proveGuardedBodyCoverageWithin(function, binding.Free, budget)
		if coverage.State == ssaflow.EvidenceUnknown {
			return coverage
		}
		if coverage.Proven() {
			return capturedBodyResult(true, budget)
		}
	}
	return capturedBodyResult(false, budget)
}

func capturedBodyResult(found bool, budget *ssaflow.SearchBudget) resourceProof {
	if resourceFlowExhausted(budget) {
		return resourceProof{State: ssaflow.EvidenceUnknown, Reason: resourceReasonBudgetExhausted}
	}
	if found {
		return resourceProof{State: ssaflow.EvidenceUnknown, Reason: resourceReasonCapturedBodyGuardedCleanup}
	}
	return resourceProof{State: ssaflow.EvidenceDisproven, Reason: resourceReasonEvidenceNotFound}
}

func proveGuardedBodyCoverageWithin(function *ssa.Function, captured ssa.Value, budget *ssaflow.SearchBudget) resourceProof {
	for candidate := range ssaflow.InstructionsWithin(function, budget) {
		load, ok := candidate.(*ssa.UnOp)
		if !ok || !capturedResponseBody(load, captured) {
			continue
		}
		covered := lifecycle.ProveMethodCallCoverageWithin(function, func(instruction ssa.Instruction) bool {
			common := ssaflow.InstructionCall(instruction)
			return budget.Spend() && common != nil && ssaflow.CallName(common) == "Close" &&
				capturedResponseBody(ssaflow.CallReceiver(common), captured)
		}, lifecycle.CoverageEveryReturn, load, budget)
		if covered.Reason == ssaflow.EvidenceBudgetExhausted {
			return resourceProof{State: ssaflow.EvidenceUnknown, Reason: resourceReasonBudgetExhausted}
		}
		if covered.Proven() {
			return carriedValueProof(true, resourceReasonCapturedBodyGuardedCleanup, budget)
		}
	}
	return carriedValueProof(false, resourceReasonEvidenceNotFound, budget)
}

func capturedResponseBody(value, captured ssa.Value) bool {
	field := lifecyclefacts.ResponseBodyField(value)
	if field == nil {
		return false
	}
	response, ok := field.X.(*ssa.UnOp)
	return ok && response.Op == token.MUL && response.X == captured
}

// Account for pointer uses before relying on a captured cell's original value.
// A local spill of that value is permitted only for the proved capture itself.
// Other closures, pointer arguments and stores can hide Body replacement; even
// a read-only-looking helper is outside this local uncertainty contract.
func responseCaptureUnmodified(
	function *ssa.Function, resource, cell ssa.Value, allowed *ssa.MakeClosure, budget *ssaflow.SearchBudget,
) bool {
	for _, block := range function.Blocks {
		for _, instruction := range block.Instrs {
			if !budget.Spend() || responseCaptureExposed(instruction, resource, cell, allowed, budget) {
				return false
			}
		}
	}
	return true
}

func responseCaptureExposed(instruction ssa.Instruction, resource, cell ssa.Value, allowed *ssa.MakeClosure, budget *ssaflow.SearchBudget) bool {
	switch typed := instruction.(type) {
	case *ssa.Store:
		if typed.Addr == cell && typed.Val == resource && cell != resource {
			return false
		}
		return responsePointerUse(typed.Addr, resource, cell, budget) || responsePointerUse(typed.Val, resource, cell, budget)
	case *ssa.MakeClosure:
		return typed != allowed && slices.ContainsFunc(typed.Bindings, func(value ssa.Value) bool {
			return responsePointerUse(value, resource, cell, budget)
		})
	case *ssa.MapUpdate, *ssa.Send, *ssa.Select:
		return slices.ContainsFunc(instruction.Operands(nil), func(value *ssa.Value) bool {
			return value != nil && responsePointerUse(*value, resource, cell, budget)
		})
	}
	common := ssaflow.InstructionCall(instruction)
	return common != nil && slices.ContainsFunc(common.Args, func(value ssa.Value) bool {
		return responsePointerUse(value, resource, cell, budget)
	})
}

func responsePointerUse(value, resource, cell ssa.Value, budget *ssaflow.SearchBudget) bool {
	return ssaflow.NewReachingWalk(
		ssaflow.TransparentChangeInterface|ssaflow.TransparentChangeType|ssaflow.TransparentConvert|ssaflow.TransparentMakeInterface,
	).Within(budget).Any(value, func(_ ssaflow.ReachingWalk, value ssa.Value) bool {
		_, pointer := value.Type().Underlying().(*types.Pointer)
		return pointer && (heapmodel.ValueDerivesFromWithin(value, resource, budget) ||
			heapmodel.ValueDerivesFromWithin(value, cell, budget))
	})
}
