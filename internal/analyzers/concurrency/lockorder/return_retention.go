package lockorder

import (
	"go/token"

	"github.com/kojah/gohawk/internal/lifecycle"
	"github.com/kojah/gohawk/internal/ssaflow"
	"golang.org/x/tools/go/ssa"
)

// Return retention records the same held-state witnesses used by final
// contracts. A returned release capability or containing owner makes ownership
// uncertain, not completed. Retention, merges and capability queries share the
// function allowance; interrupted witnesses remain private to the cut walk.

func (query lockReturnQueries) recordUnreleasedLocks(
	instruction ssa.Instruction,
	held, deferred []string,
	lockValues map[string][]ssa.Value,
	unreleased map[string][]token.Pos,
	heldAtReturn map[*ssa.Return]lockReturnState,
) {
	returned, ok := instruction.(*ssa.Return)
	if !ok {
		return
	}
	retained := make([]string, 0, len(held))
	for _, identity := range held {
		if !query.budget.Spend() {
			return
		}
		if !query.containsLock(deferred, identity) && !query.returnedUnlockOwner(returned, lockValues[identity]) {
			unreleased[identity] = query.appendReturnPosition(unreleased[identity], returned.Pos())
			retained = append(retained, identity)
		}
	}
	previous, seen := heldAtReturn[returned]
	merged := query.mergeReturnState(previous, retained, seen)
	if !query.budget.Exhausted() {
		heldAtReturn[returned] = merged
	}
}

func (query lockReturnQueries) returnedUnlockOwner(returned *ssa.Return, values []ssa.Value) bool {
	for _, result := range returned.Results {
		if !query.budget.Spend() {
			return false
		}
		for _, value := range values {
			if !query.budget.Spend() {
				return false
			}
			if lifecycle.ProveValueCallsMethodWithin(result, "Unlock", value, query.budget).Proven() ||
				lifecycle.ProveValueCallsMethodWithin(result, "RUnlock", value, query.budget).Proven() {
				return true
			}
			// Returning the object containing a held mutex exposes its release to
			// the caller. This is unknown ownership, not proof that any method
			// named Unlock releases it. Kube-vip returns such an owner on success:
			// https://github.com/kube-vip/kube-vip/blob/be536eaaf73c80fa5161e757ac18b472498f986e/pkg/iptables/lock.go#L54-L67
			if ssaflow.NewReachingWalk(mutexForms).Within(query.budget).Any(result, func(_ ssaflow.ReachingWalk, owner ssa.Value) bool {
				return ssaflow.ValueIsAccessPathFromWithin(value, owner, query.budget)
			}) {
				return true
			}
		}
	}
	return false
}

func (query lockReturnQueries) mergeReturnState(previous lockReturnState, held []string, seen bool) lockReturnState {
	if !seen {
		// The first observation owns both masks. Detach each from the incoming
		// flow state so later path mutation cannot rewrite a return witness.
		var retained []string
		for _, identity := range held {
			if !query.budget.Spend() {
				return lockReturnState{}
			}
			retained = append(retained, identity)
		}
		var definite []string
		for _, identity := range retained {
			if !query.budget.Spend() {
				return lockReturnState{}
			}
			definite = append(definite, identity)
		}
		return lockReturnState{possible: retained, definite: definite}
	}
	// Possible retention is the union across paths; only the intersection
	// may establish a caller-held contract. Never expose a shortened mask.
	for _, identity := range held {
		if !query.budget.Spend() {
			return lockReturnState{}
		}
		if !query.containsLock(previous.possible, identity) {
			previous.possible = append(previous.possible, identity)
		}
	}
	var definite []string
	for _, identity := range previous.definite {
		if !query.budget.Spend() {
			return lockReturnState{}
		}
		if query.containsLock(held, identity) {
			definite = append(definite, identity)
		}
	}
	if query.budget.Exhausted() {
		return lockReturnState{}
	}
	previous.definite = definite
	return previous
}

func (query lockReturnQueries) appendReturnPosition(positions []token.Pos, candidate token.Pos) []token.Pos {
	for _, position := range positions {
		if !query.budget.Spend() {
			return positions
		}
		if position == candidate {
			return positions
		}
	}
	return append(positions, candidate)
}
