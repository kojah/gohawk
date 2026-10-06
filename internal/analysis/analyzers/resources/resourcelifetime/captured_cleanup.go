package resourcelifetime

import (
	"go/types"
	"slices"

	"github.com/kojah/gohawk/internal/engine/heapmodel"
	"github.com/kojah/gohawk/internal/engine/lifecycle"
	proofs "github.com/kojah/gohawk/internal/engine/proof"
	"github.com/kojah/gohawk/internal/engine/ssaflow"
	ssacall "github.com/kojah/gohawk/internal/engine/ssaflow/calls"
	cfg "github.com/kojah/gohawk/internal/engine/ssaflow/cfg"
	"github.com/kojah/gohawk/internal/engine/syntax"
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
		proof := analysis.proveCapturedCellCleanupWithin(deferred, analysis.budget(proofs.SummaryBudget))
		if proof.State != proofs.EvidenceDisproven {
			return proof.Reason, true
		}
	}
	if proof := analysis.proveGuardedCapturedBodyWithin(instruction, closure, analysis.budget(1000)); proof.State == proofs.EvidenceUnknown {
		return proof.Reason, true
	}
	// A captured aggregate can be populated after closure creation. The
	// closure observes its fields when called, so an unresolved cleanup of
	// that owner is unknown rather than proof that the acquisition leaks.
	// https://github.com/Autumn-27/ARTEX/blob/bf7f414477832b77d2152539c0723dc691086522/traffic/traffic.go#L1129-L1176
	if owner := analysis.proveCapturedAggregateOwnerWithin(closure, analysis.budget(proofs.SummaryBudget)); owner.State != proofs.EvidenceDisproven {
		return owner.Reason, true
	}
	if !carried {
		capture := analysis.proveClosureCarryWithin(closure, analysis.budget(proofs.SummaryBudget))
		if capture.State == proofs.EvidenceUnknown {
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
func (analysis *resourceAnalysis) proveCapturedAggregateOwnerWithin(closure *ssa.MakeClosure, budget *proofs.SearchBudget) resourceProof {
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
func (analysis *resourceAnalysis) provePriorCleanupWithin(acquisition *ssa.Call, budget *proofs.SearchBudget) priorCleanupProof {
	for instruction := range ssaflow.InstructionsWithin(analysis.function, budget) {
		deferred, ok := instruction.(*ssa.Defer)
		if !ok || !cfg.InstructionDominates(deferred, acquisition) {
			continue
		}
		proof := analysis.provePriorDeferredWithin(acquisition, deferred, budget)
		if proof.State == proofs.EvidenceUnknown {
			return priorCleanupProof{resourceProof: proof}
		}
		if proof.Proven() {
			return priorCleanupProof{resourceProof: proof, Instruction: deferred}
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
		if !ok || !cfg.InstructionDominates(call, acquisition) ||
			!ssacall.HasLibraryContract(call.Common(), ssacall.ContractTestingCleanup) {
			continue
		}
		for _, argument := range call.Common().Args {
			proof := analysis.proveCarriedClosureWithin(argument, budget)
			if proof.State == proofs.EvidenceUnknown {
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

// Preserve captured-cell, SQL parent and paired-context reason precedence.
// Each witness supplies possible cleanup; none is an exact release guarantee.
func (analysis *resourceAnalysis) provePriorDeferredWithin(acquisition *ssa.Call, deferred *ssa.Defer, budget *proofs.SearchBudget) resourceProof {
	proof := analysis.proveCapturedCellCleanupWithin(deferred, budget)
	if proof.State == proofs.EvidenceUnknown {
		return proof
	}
	if proof.Proven() {
		return carriedValueProof(true, resourceReasonPriorDeferMayCleanCapturedCell, budget)
	}
	parent := proveSQLParentCleanupWithin(acquisition, deferred, budget)
	if parent.State != proofs.EvidenceDisproven {
		return parent
	}
	return carriedValueProof(cancelsTransactionContext(acquisition, deferred), resourceReasonTransactionContextCanceled, budget)
}

// A defer observes the captured cell at return, not at registration.
// When several acquisitions feed that cell, exact value completion may fail.
// A positive cleanup witness for the cell makes ownership unknown; it does
// not prove which stored value will be closed. Read-only captures and by-value
// deferred arguments do not qualify. Overwritten-cell leaks may be missed.
// https://github.com/wind-c/comqtt/blob/11282b91abb06d5169b857a2c38fad5d54502050/plugin/auth/http/http.go#L89-L127
func (analysis *resourceAnalysis) proveCapturedCellCleanupWithin(deferred *ssa.Defer, budget *proofs.SearchBudget) resourceProof {
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

// Deferred releases guarded by a named result. A deferred literal runs after
// the return statement has set the function's named results, so a literal
// that closes only while err is non-nil, the close-on-error idiom, releases
// on some returns and not on others. Crediting it at the defer, as the
// data-dependent policy does for other guards, hides the success path that
// keeps the resource and hands it to nobody. Instead the defer settles
// nothing where it runs, and each return it dominates asks the shared
// completion search whether the literal releases given the value that return
// stores: a nil literal skips the cleanup, a value never nil runs it, and a
// value of unknown nilness leaves the resource unknown on that path.
//
// A literal counts as result-guarded only when the answer really turns on
// the result: it releases under one outcome of a captured named result and
// not under the other. A guard on any other variable, such as the committed
// flag of the transaction idiom, keeps the data-dependent policy.
// https://github.com/grpc/grpc-go/commit/db35da8bc5e8dcfcb57b94e9be0fba306710cc77

// cleanupRequests is one completion question per cleanup method.
func (analysis *resourceAnalysis) cleanupRequests(budget *proofs.SearchBudget) []lifecycle.CompletionRequest {
	requests := make([]lifecycle.CompletionRequest, 0, len(analysis.contract.cleanup))
	for _, method := range analysis.contract.cleanup {
		requests = append(requests, lifecycle.CompletionRequest{
			Target: analysis.resource, Methods: []string{method}, Budget: budget,
		})
	}
	return requests
}

func (analysis *resourceAnalysis) discoverResultGuardedDefersWithin(budget *proofs.SearchBudget) resourceProof {
	var guards []lifecycle.ResultGuard
	seen := make(map[*ssa.Defer]bool)
	for _, request := range analysis.cleanupRequests(budget) {
		if !budget.Spend() {
			return carriedValueProof(false, resourceReasonUntouched, budget)
		}
		discovery := lifecycle.ProveResultGuards(analysis.function, request)
		if !discovery.Proven() || resourceFlowExhausted(budget) {
			return resourceProof{State: proofs.EvidenceUnknown, Reason: resourceReasonBudgetExhausted}
		}
		for _, guard := range discovery.Guards {
			if !budget.Spend() {
				return carriedValueProof(false, resourceReasonUntouched, budget)
			}
			if !seen[guard.Defer] {
				seen[guard.Defer] = true
				guards = append(guards, guard)
			}
		}
	}
	if resourceFlowExhausted(budget) {
		return resourceProof{State: proofs.EvidenceUnknown, Reason: resourceReasonBudgetExhausted}
	}
	analysis.guardedDefers = guards
	return carriedValueProof(true, resourceReasonNone, budget)
}

func (analysis *resourceAnalysis) resultGuarded(deferred *ssa.Defer) bool {
	for _, guard := range analysis.guardedDefers {
		if guard.Defer == deferred {
			return true
		}
	}
	return false
}

// resultGuardedLabel labels a result-guarded defer, which settles nothing
// where it runs, and a return it may reach.
func (analysis *resourceAnalysis) resultGuardedLabel(instruction ssa.Instruction) (resourceAction, resourceLifetimeReason, bool) {
	switch typed := instruction.(type) {
	case *ssa.Defer:
		if analysis.resultGuarded(typed) {
			return actionNone, resourceReasonResultGuardedDefer, true
		}
	case *ssa.Return:
		return analysis.resultGuardedReturn(typed)
	}
	return actionNone, resourceReasonNone, false
}

// resultGuardedReturn labels a return by the result-guarded defers that
// reach it: settled when one releases given the values this return stores,
// unknown when a defer may or may not be registered on the way, or when the
// answer is not known. It declines when every such defer provably skips the
// cleanup, so the return keeps its ordinary label.
func (analysis *resourceAnalysis) resultGuardedReturn(returned *ssa.Return) (resourceAction, resourceLifetimeReason, bool) {
	uncertain := false
	budget := analysis.budget(releaseSearchBudget)
	outcomeOf := func(value ssa.Value) (ssacall.Outcome, bool) {
		return analysis.summaries.OutcomeOf(value, budget)
	}
	for _, guard := range analysis.guardedDefers {
		reaching := guard.ProveReachesReturn(returned, budget)
		if !reaching.Proven() {
			uncertain = uncertain || reaching.State == proofs.EvidenceUnknown
			continue
		}
		for _, request := range analysis.cleanupRequests(budget) {
			switch guard.CompletesAtReturn(request, returned, outcomeOf) {
			case proofs.EvidenceProven:
				return actionSettled, resourceReasonResultGuardedRelease, true
			case proofs.EvidenceUnknown:
				uncertain = true
			case proofs.EvidenceDisproven:
			}
		}
	}
	if uncertain {
		return actionUnknown, resourceReasonResultGuardedUnknown, true
	}
	return actionNone, resourceReasonNone, false
}
