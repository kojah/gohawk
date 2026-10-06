package resourcelifetime

import (
	"github.com/kojah/gohawk/internal/lifecycle"
	proofs "github.com/kojah/gohawk/internal/proof"
	ssacall "github.com/kojah/gohawk/internal/ssaflow/calls"
	"golang.org/x/tools/go/ssa"
)

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
