package lifecycle

import (
	"slices"

	"github.com/kojah/gohawk/internal/ssaflow"
	"golang.org/x/tools/go/ssa"
)

// Callback capabilities describe values that may carry cleanup, not a promise
// that their owners invoke it. Origin and stored-value traversal use the same
// completion engine for callback bodies; request cutoff supplies no capability.

// ValueCallsMethod reports whether value is, or carries, a callback that
// calls method on target when invoked: a function literal whose body
// completes the target, a bound method value, or such a callback held in a
// local, passed through a call result, or merged by a phi.
func ValueCallsMethod(value ssa.Value, method string, target ssa.Value) bool {
	return ProveValueCallsMethodWithin(value, method, target, nil).Proven()
}

// ProveValueCallsMethodWithin asks whether value may carry a callback whose
// body completes method on target. It retains ValueCallsMethod's any-origin
// and callback-preserving wrapper policies; it proves neither invocation nor
// that every possible callback completes. Value, referrer and callee queries
// share budget. A cutoff is unknown; nil retains the default unbounded search.
func ProveValueCallsMethodWithin(value ssa.Value, method string, target ssa.Value, budget *ssaflow.SearchBudget) ssaflow.Proof {
	matched := newCompletionSearch(method, CoverageEveryReturn, budget).valueCallsMethod(value, target)
	if budget.Exhausted() {
		return ssaflow.Proof{State: ssaflow.EvidenceUnknown, Reason: ssaflow.EvidenceBudgetExhausted}
	}
	if matched {
		return ssaflow.Proof{State: ssaflow.EvidenceProven, Reason: ssaflow.EvidenceCallbackCompletion, Method: method}
	}
	return ssaflow.Proof{State: ssaflow.EvidenceUnknown, Reason: ssaflow.EvidenceUnavailable}
}

func (search *completionSearch) valueCallsMethod(value, target ssa.Value) bool {
	return search.callbackCapability(search.callbackValues, value, target)
}

// Any keeps the historical may-carry policy across phi alternatives. Its fold
// owns wrapper/origin recursion; callee recursion stays in the completion memo.
// A revisited origin supplies no capability and never proves invocation.
func (search *completionSearch) callbackCapability(walk ssaflow.ReachingWalk, value, target ssa.Value) bool {
	return walk.Any(value, func(next ssaflow.ReachingWalk, leaf ssa.Value) bool {
		switch typed := leaf.(type) {
		case *ssa.MakeClosure:
			callees, ok := closureCallees(typed, launchCallback, search.budget)
			return ok && search.calleeCompletes(callees[0], target, nil).proven
		case *ssa.Alloc:
			return search.storedValueCallsMethod(next, typed, target)
		case *ssa.UnOp:
			return search.storedValueCallsMethod(next, typed.X, target)
		case *ssa.Call:
			// This is a possible retained capability, not an exact factory relation:
			// a wrapper receiving a callback may carry it in its returned value.
			return slices.ContainsFunc(typed.Common().Args, func(argument ssa.Value) bool {
				return search.callbackCapability(next, argument, target)
			})
		}
		return false
	})
}

func (search *completionSearch) storedValueCallsMethod(walk ssaflow.ReachingWalk, address, target ssa.Value) bool {
	if address == nil || address.Referrers() == nil {
		return false
	}
	for _, reference := range *address.Referrers() {
		if !search.budget.Spend() {
			return false
		}
		store, ok := reference.(*ssa.Store)
		if ok && store.Addr == address && search.callbackCapability(walk, store.Val, target) {
			return true
		}
	}
	return false
}
