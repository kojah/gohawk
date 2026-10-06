package lockorder

import (
	"slices"

	proofs "github.com/kojah/gohawk/internal/proof"
)

// A successor may share its predecessor's state until expansion. Mutable lock
// collections are detached under the traversal allowance before any transfer;
// cutoff exposes no partly copied state to another path.
func cloneLockStateWithin(state lockFlowState, budget *proofs.SearchBudget) (lockFlowState, bool) {
	if budget.Exhausted() {
		return lockFlowState{}, false
	}
	for _, values := range [][]string{state.held, state.readHeld, state.deferred} {
		for range values {
			if !budget.Spend() {
				return lockFlowState{}, false
			}
		}
	}
	state.held = slices.Clone(state.held)
	state.readHeld = slices.Clone(state.readHeld)
	state.deferred = slices.Clone(state.deferred)
	guards := map[string]lockGuard{}
	for identity, guard := range state.guards {
		if !budget.Spend() {
			return lockFlowState{}, false
		}
		guards[identity] = guard
	}
	origins := map[string]lockAcquisition{}
	for identity, origin := range state.origins {
		if !budget.Spend() {
			return lockFlowState{}, false
		}
		origins[identity] = origin
	}
	state.guards, state.origins = guards, origins
	return state, true
}
