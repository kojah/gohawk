package resourcelifetime

import (
	"go/token"
	"go/types"
	"slices"

	"github.com/kojah/gohawk/internal/heapmodel"
	"github.com/kojah/gohawk/internal/lifecycle"
	"github.com/kojah/gohawk/internal/passes/lifecyclefacts"
	"github.com/kojah/gohawk/internal/ssaflow"
	"github.com/kojah/gohawk/internal/syntax"
	"golang.org/x/tools/go/ssa"
)

// Captured cleanup classifies lifetime evidence where a closure observes an
// addressable cell. Deferred closures observe cells at return; called closures
// need an exact current capture. Neither uncertain path proves release.
// These are classifier inputs to the ordinary resource flow, not another flow.

func (analysis *resourceAnalysis) opaqueClosureCall(instruction ssa.Instruction, closure *ssa.MakeClosure, carried bool) (resourceLifetimeReason, bool) {
	// A deferred literal observes the cell at return, which can differ from
	// its registration-time value. Reuse the prior-registration uncertainty
	// boundary rather than treating an unresolved release as a transparent call.
	// https://github.com/anton48/vk-turn-proxy-ios/blob/001caf2ae24ecd07b021d7ca7b14a98a006bff65/third_party/speedtest-go/speedtest/server.go#L262-L285
	if deferred, ok := instruction.(*ssa.Defer); ok {
		proof := analysis.proveCapturedCellCleanupWithin(deferred, analysis.budget(ssaflow.SummaryBudget))
		if proof.State != ssaflow.EvidenceDisproven {
			return proof.Reason, true
		}
	}
	if proof := analysis.guardedCapturedBodyCleanup(instruction, closure); proof.State == ssaflow.EvidenceUnknown {
		return proof.Reason, true
	}
	// A captured aggregate can be populated after closure creation. The
	// closure observes its fields when called, so an unresolved cleanup of
	// that owner is unknown rather than proof that the acquisition leaks.
	// https://github.com/Autumn-27/ARTEX/blob/bf7f414477832b77d2152539c0723dc691086522/traffic/traffic.go#L1129-L1176
	if owner := analysis.proveCapturedAggregateOwnerWithin(closure, analysis.budget(ssaflow.SummaryBudget)); owner.State != ssaflow.EvidenceDisproven {
		return owner.Reason, true
	}
	if !carried {
		capture := analysis.proveClosureCarryWithin(closure, analysis.budget(ssaflow.SummaryBudget))
		if capture.State == ssaflow.EvidenceUnknown {
			return capture.Reason, true
		}
		if !capture.Proven() {
			return resourceReasonNone, false
		}
	}
	// A started literal runs on another goroutine, so a release inside it
	// cannot be ordered against this function's returns and the resource
	// is beyond what this flow can judge.
	if _, started := instruction.(*ssa.Go); started {
		return resourceReasonCapturedByStartedLiteral, true
	}
	// A called or deferred literal runs in this frame, so its body is as
	// readable as a named callee's. A release inside it was already proved
	// before this point, so what is left to ask is whether it keeps the
	// resource: if it does the obligation moved, and if it does not the
	// literal is transparent and this function still owns the resource.
	//
	// That holds only for a body this pass can judge. A capture is a cell
	// the body loads first, so the argument at a call inside the literal is
	// the load rather than the acquired value, and a summary matched on
	// exact identity does not recognize it. block/spirit closes rows
	// through a helper in another package from inside a deferred literal,
	// and the release went uncredited while the literal was called
	// transparent:
	// https://github.com/block/spirit/blob/c554eae/pkg/checksum/single.go#L493-L503
	if analysis.evidence.ClosureHandsValueToUnreadableCallee(closure, analysis.resource) {
		return resourceReasonCapturedByLiteralCallingUnreadableCallee, true
	}
	return resourceReasonCapturedByRetainingLiteral, analysis.evidence.ClosureRetainsValue(closure, analysis.resource)
}

// A capture can refer to a discovered aggregate owner even when the resource
// is assigned after closure creation. Possible matching supplies uncertainty,
// never cleanup. An interrupted owner or binding census cannot reject it.
func (analysis *resourceAnalysis) proveCapturedAggregateOwnerWithin(closure *ssa.MakeClosure, budget *ssaflow.SearchBudget) resourceProof {
	if !budget.Spend() {
		return carriedValueProof(false, resourceReasonUntouched, budget)
	}
	for _, owner := range analysis.owners {
		if !budget.Spend() {
			return carriedValueProof(false, resourceReasonUntouched, budget)
		}
		pointer, ok := owner.Type().Underlying().(*types.Pointer)
		if !ok {
			continue
		}
		if !budget.Spend() {
			return carriedValueProof(false, resourceReasonUntouched, budget)
		}
		if heapmodel.MayAlias(owner, analysis.resource) || syntax.PointerStruct(pointer) == nil {
			continue
		}
		for _, binding := range closure.Bindings {
			if heapmodel.CapturedBindingMatchesWithin(binding, owner, budget) {
				return carriedValueProof(true, resourceReasonCapturedAggregateOwner, budget)
			}
			if resourceFlowExhausted(budget) {
				return carriedValueProof(false, resourceReasonUntouched, budget)
			}
		}
	}
	return carriedValueProof(false, resourceReasonUntouched, budget)
}

type priorCleanupProof struct {
	resourceProof
	Instruction ssa.Instruction
}

// provePriorCleanupWithin handles a retained callback registered before a
// captured variable is reassigned to this acquisition. The callback observes
// the cell at cleanup time, not its registration-time value. Mutable guards
// prevent proving release, so this is unknown rather than a settled resource.
// https://github.com/james-6-23/codex2api/blob/4f96afe95bb16132347f4ab74e63b0b1fa0f778b/admin/handler_test.go#L1398-L1447
func (analysis *resourceAnalysis) provePriorCleanupWithin(acquisition *ssa.Call, budget *ssaflow.SearchBudget) priorCleanupProof {
	for instruction := range ssaflow.InstructionsWithin(analysis.function, budget) {
		deferred, ok := instruction.(*ssa.Defer)
		if !ok || !ssaflow.InstructionDominates(deferred, acquisition) {
			continue
		}
		proof := analysis.proveCapturedCellCleanupWithin(deferred, budget)
		if proof.State == ssaflow.EvidenceUnknown {
			return priorCleanupProof{resourceProof: proof}
		}
		reason := resourceReasonNone
		switch {
		case proof.Proven():
			reason = resourceReasonPriorDeferMayCleanCapturedCell
		case closesStatementDatabase(acquisition, deferred):
			reason = resourceReasonStatementParentClosed
		case finishesRowsTransaction(acquisition, deferred):
			reason = resourceReasonRowsTransactionFinished
		case cancelsTransactionContext(acquisition, deferred):
			reason = resourceReasonTransactionContextCanceled
		}
		if reason != resourceReasonNone {
			return priorCleanupProof{resourceProof: carriedValueProof(true, reason, budget), Instruction: deferred}
		}
	}
	if resourceFlowExhausted(budget) {
		return priorCleanupProof{resourceProof: carriedValueProof(false, resourceReasonUntouched, budget)}
	}
	// Keep deferred cleanup ahead of registration witnesses, preserving the
	// existing reason precedence. A known testing callback may retain this
	// resource without providing any synchronous cleanup guarantee.
	for instruction := range ssaflow.InstructionsWithin(analysis.function, budget) {
		call, ok := instruction.(*ssa.Call)
		if !ok || !ssaflow.InstructionDominates(call, acquisition) ||
			!ssaflow.HasLibraryContract(call.Common(), ssaflow.ContractTestingCleanup) {
			continue
		}
		for _, argument := range call.Common().Args {
			proof := analysis.proveCarriedClosureWithin(argument, budget)
			if proof.State == ssaflow.EvidenceUnknown {
				return priorCleanupProof{resourceProof: proof}
			}
			if proof.Proven() {
				return priorCleanupProof{
					resourceProof: carriedValueProof(true, resourceReasonCapturedByPriorCleanup, budget), Instruction: call,
				}
			}
		}
	}
	return priorCleanupProof{resourceProof: carriedValueProof(false, resourceReasonUntouched, budget)}
}

// A defer observes the captured cell at return, not at registration.
// When several acquisitions feed that cell, exact value completion may fail.
// A positive cleanup witness for the cell makes ownership unknown; it does
// not prove which stored value will be closed. Read-only captures and by-value
// deferred arguments do not qualify. Overwritten-cell leaks may be missed.
// https://github.com/wind-c/comqtt/blob/11282b91abb06d5169b857a2c38fad5d54502050/plugin/auth/http/http.go#L89-L127
func (analysis *resourceAnalysis) proveCapturedCellCleanupWithin(deferred *ssa.Defer, budget *ssaflow.SearchBudget) resourceProof {
	if !budget.Spend() {
		return carriedValueProof(false, resourceReasonUntouched, budget)
	}
	closure, ok := deferred.Common().Value.(*ssa.MakeClosure)
	if !ok {
		return carriedValueProof(false, resourceReasonUntouched, budget)
	}
	function, _ := closure.Fn.(*ssa.Function)
	for _, pair := range ssaflow.ClosureBindingPairs(function, closure) {
		if !budget.Spend() || !heapmodel.CapturedBindingMatchesWithin(pair.Binding, analysis.resource, budget) {
			if resourceFlowExhausted(budget) {
				return carriedValueProof(false, resourceReasonUntouched, budget)
			}
			continue
		}
		// Anywhere coverage preserves the former may-cleanup boundary. This
		// witness never establishes release before every normal return.
		for instruction := range ssaflow.InstructionsWithin(function, budget) {
			common := ssaflow.InstructionCall(instruction)
			if common != nil && slices.Contains(analysis.contract.cleanup, ssaflow.CallName(common)) &&
				ssaflow.ValueIsAccessPathFromWithin(ssaflow.CallReceiver(common), pair.Free, budget) {
				return carriedValueProof(true, resourceReasonCapturedCellMayCleanup, budget)
			}
			if resourceFlowExhausted(budget) {
				return carriedValueProof(false, resourceReasonUntouched, budget)
			}
		}
	}

	return carriedValueProof(false, resourceReasonUntouched, budget)
}

// A called literal may own HTTP body cleanup while guarding a distinct load
// of the captured response's Body. Repeated loads do not establish equality,
// so this is uncertainty, not completion. Require the exact unchanged capture
// and a body-nil guard alone; Boolean conditions and visible pointer escapes
// or replacement cannot establish even this narrow boundary.
// https://github.com/openkruise/kruise-game/blob/16a0418780d8abd3ee871448116bbc5dc1e98d48/test/e2e/framework/framework.go#L549-L570
func (analysis *resourceAnalysis) guardedCapturedBodyCleanup(instruction ssa.Instruction, closure *ssa.MakeClosure) resourceProof {
	missing := resourceProof{State: ssaflow.EvidenceDisproven, Reason: resourceReasonEvidenceNotFound}
	if _, called := instruction.(*ssa.Call); !called || analysis.contract.family != "http" {
		return missing
	}
	function, _ := closure.Fn.(*ssa.Function)
	budget := analysis.budget(1000)
	for _, binding := range ssaflow.ClosureBindingPairs(function, closure) {
		if !budget.Spend() {
			return missing
		}
		stored := heapmodel.NewStorage(budget).StableContent(binding.Binding, instruction)
		if !stored.Proven() || stored.Value != analysis.resource {
			continue
		}
		if !responseCaptureUnmodified(analysis.function, analysis.resource, binding.Binding, closure, budget) ||
			!responseCaptureUnmodified(function, binding.Free, binding.Free, nil, budget) {
			continue
		}
		if guardedBodyCoverage(function, binding.Free, budget) {
			return resourceProof{State: ssaflow.EvidenceUnknown, Reason: resourceReasonCapturedBodyGuardedCleanup}
		}
	}
	return missing
}

func guardedBodyCoverage(function *ssa.Function, captured ssa.Value, budget *ssaflow.SearchBudget) bool {
	for _, load := range ssaflow.InstructionsOf[*ssa.UnOp](function) {
		if !budget.Spend() || !capturedResponseBody(load, captured) {
			continue
		}
		covered := lifecycle.MethodCallCoverage(function, func(instruction ssa.Instruction) bool {
			common := ssaflow.InstructionCall(instruction)
			return budget.Spend() && common != nil && ssaflow.CallName(common) == "Close" &&
				capturedResponseBody(ssaflow.CallReceiver(common), captured)
		}, lifecycle.CoverageEveryReturn, load)
		if covered && !budget.Exhausted() {
			return true
		}
	}
	return false
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
			if !budget.Spend() || responseCaptureExposed(instruction, resource, cell, allowed) {
				return false
			}
		}
	}
	return true
}

func responseCaptureExposed(instruction ssa.Instruction, resource, cell ssa.Value, allowed *ssa.MakeClosure) bool {
	switch typed := instruction.(type) {
	case *ssa.Store:
		if typed.Addr == cell && typed.Val == resource && cell != resource {
			return false
		}
		return responsePointerUse(typed.Addr, resource, cell) || responsePointerUse(typed.Val, resource, cell)
	case *ssa.MakeClosure:
		return typed != allowed && slices.ContainsFunc(typed.Bindings, func(value ssa.Value) bool {
			return responsePointerUse(value, resource, cell)
		})
	case *ssa.MapUpdate, *ssa.Send, *ssa.Select:
		return slices.ContainsFunc(instruction.Operands(nil), func(value *ssa.Value) bool {
			return value != nil && responsePointerUse(*value, resource, cell)
		})
	}
	common := ssaflow.InstructionCall(instruction)
	return common != nil && slices.ContainsFunc(common.Args, func(value ssa.Value) bool {
		return responsePointerUse(value, resource, cell)
	})
}

func responsePointerUse(value, resource, cell ssa.Value) bool {
	return ssaflow.NewReachingWalk(
		ssaflow.TransparentChangeInterface|ssaflow.TransparentChangeType|ssaflow.TransparentConvert|ssaflow.TransparentMakeInterface,
	).Any(value, func(_ ssaflow.ReachingWalk, value ssa.Value) bool {
		_, pointer := value.Type().Underlying().(*types.Pointer)
		return pointer && (heapmodel.ValueDerivesFrom(value, resource) ||
			heapmodel.ValueDerivesFrom(value, cell))
	})
}
