package goroutineownership

import (
	"github.com/kojah/gohawk/internal/heapmodel"
	proofs "github.com/kojah/gohawk/internal/proof"
	"github.com/kojah/gohawk/internal/ssaflow"
	"golang.org/x/tools/go/ssa"
)

// Helper-call evidence maps caller handles into a source-visible helper and
// distinguishes exact completion from possible ownership. Binding preparation,
// recursive effects and exact caller identity share one candidate allowance;
// incomplete selection is opaque rather than a completed absence of handoff.

type helperCallProof struct {
	action ownershipAction
	reason goroutineOwnershipReason
}

func (analysis *spawnAnalysis) helperCallResult(action ownershipAction, budget *proofs.SearchBudget) helperCallProof {
	if budget.Exhausted() {
		budget.Observe(proofs.EvidenceBudgetExhausted, analysis.spawn.Pos(), func() map[string]string {
			return map[string]string{"phase": "helper-call"}
		})
		return helperCallProof{action: actionUnknown, reason: reasonHelperCallBudgetExhausted}
	}
	return helperCallProof{action: action, reason: reasonLabelHelper}
}

// helperAction follows every tracked value that the call site supplies to a
// source-visible callee, whether as an argument or a captured variable.
func (analysis *spawnAnalysis) helperAction(
	common *ssa.CallCommon, callee *ssa.Function, closure *ssa.MakeClosure, values []trackedValue,
) helperCallProof {
	budget := analysis.budget()
	result := actionNone
	var search *helperSearch
	for pair := range ssaflow.CallBindingsWithin(common, callee, closure, budget) {
		for _, tracked := range values {
			if !budget.Spend() {
				return analysis.helperCallResult(actionUnknown, budget)
			}
			carried := bindingCarries(pair.Supplied, tracked.value)
			projected := ssaflow.ValueIsAccessPathFrom(tracked.value, pair.Supplied)
			if !carried && !projected {
				continue
			}
			// The memo key retains the formal/capture and tracked kind. Caller
			// identity is checked separately for each supplied binding.
			if search == nil {
				search = newHelperSearchWithin(budget)
				search.concurrency, _ = summaryKnowledge.Provider(analysis.pass).Concurrency()
			}
			action := boundHelperAction(pair.Supplied, tracked, search.use(callee, pair.Local, tracked.kind), budget)
			if budget.Exhausted() {
				return analysis.helperCallResult(actionUnknown, budget)
			}
			result = strongerAction(result, action)
			// One exact completion alternative covers the worker. Later opaque
			// handles cannot weaken it; stop before preparing unrelated bindings.
			if result == actionJoin {
				return analysis.helperCallResult(result, budget)
			}
		}
	}
	return analysis.helperCallResult(result, budget)
}

// Owner coverage establishes a lifecycle call, not observation of worker
// completion. A completion handle also needs exact binding: containment includes
// old stores and aggregate projections, which supply only possible shutdown.
// https://github.com/jech/galene/blob/6d9338e909fdecdd906150e4dda34e10d9869654/rtpconn/webclient.go#L878-L894
func boundHelperAction(supplied ssa.Value, tracked trackedValue, effect ownershipAction, budget *proofs.SearchBudget) ownershipAction {
	if effect != actionJoin {
		return effect
	}
	if tracked.kind == trackedOwner {
		return actionUnknown
	}
	if !heapmodel.NewStorage(budget).Same(supplied, tracked.value).Proven() {
		return actionUnknown
	}
	return actionJoin
}
