package lockorder

import (
	"go/token"
	"go/types"
	"slices"

	"github.com/kojah/gohawk/internal/lifecycle"
	proofs "github.com/kojah/gohawk/internal/proof"
	"github.com/kojah/gohawk/internal/ssaflow"
	ssacall "github.com/kojah/gohawk/internal/ssaflow/calls"
	cfg "github.com/kojah/gohawk/internal/ssaflow/cfg"
	ssapath "github.com/kojah/gohawk/internal/ssaflow/path"
	"github.com/kojah/gohawk/internal/syntax"
	"golang.org/x/tools/go/ssa"
)

// Return contracts consume the completed function inventory and held-state
// witnesses. All contract queries share the walk allowance; a cutoff cannot
// establish either caller transfer or a missing release. Returned-result storage
// and call metadata share that allowance; mutex identity retains separate costs.
type lockReturnQueries struct {
	setup  *lockFunctionSetup
	budget *proofs.SearchBudget
}

// acquiresForCaller reports whether the function's contract is to return with
// the lock held: every successful return that an acquisition dominates, one
// that returns no error or a nil error, still holds it, and at least one such
// return exists. Retention on every normal path also establishes this contract,
// including error returns. A helper that begins a critical section for its caller, with
// a matching helper that ends it, has this shape; a function that forgets an
// unlock on one successful path, or acquires only conditionally, does not.
// crabbox pairs beginOperation with endOperation:
// https://github.com/openclaw/crabbox/blob/3ef3f98cbe27e6ddc814c11fde15b89c1639bcbe/internal/providers/incus/client.go#L161-L185
// https://github.com/fiorix/go-diameter/blob/c7794c55a5412a3d91b17165971be4c6bc6b3ced/examples/s6a_proxy/service/util.go#L51-L87
func (query lockReturnQueries) acquiresForCaller(
	function *ssa.Function, acquisitions []ssa.Instruction, heldAt map[*ssa.Return]lockReturnState, identity string,
) callerReleaseProof {
	successful := 0
	allHeld := true
	returns := 0
	for _, returned := range query.setup.returns {
		if !query.budget.Spend() {
			return callerReleaseProof{reason: lockReasonLockStateBudgetExhausted}
		}
		returns++
		definite := query.containsLock(heldAt[returned].definite, identity)
		allHeld = allHeld && definite
		if !query.successfulReturn(function, returned) {
			continue
		}
		dominated := slices.ContainsFunc(acquisitions, func(acquisition ssa.Instruction) bool {
			return query.budget.Spend() && cfg.InstructionDominatesWithin(acquisition, returned, query.budget)
		})
		if !dominated {
			continue
		}
		successful++
		if !definite {
			return callerReleaseProof{reason: lockReasonHeldForCallerUnknown}
		}
	}
	if query.budget.Exhausted() {
		return callerReleaseProof{reason: lockReasonLockStateBudgetExhausted}
	}
	if successful > 0 || returns > 0 && allHeld {
		return callerReleaseProof{proven: true, reason: lockReasonHeldForCallerProven}
	}
	return callerReleaseProof{reason: lockReasonHeldForCallerUnknown}
}

// successfulReturn reports whether the return signals success by Go's
// result conventions: a trailing error result must be nil, and a trailing
// Boolean result, the comma-ok shape, must not be the constant false. A
// claim helper that returns (claim, false) after releasing the lock and
// (claim, true) while holding it is acquiring for its caller. kandev claims
// prompt completions this way:
// https://github.com/kdlbs/kandev/blob/17da0aafe33df01828e21fc79cc9dd156dc088dc/apps/backend/internal/agent/runtime/lifecycle/manager_events.go#L238-L272
func (query lockReturnQueries) successfulReturn(function *ssa.Function, returned *ssa.Return) bool {
	results := function.Signature.Results()
	if results.Len() == 0 || len(returned.Results) == 0 {
		return true
	}
	last := results.At(results.Len() - 1).Type()
	// A function with a defer returns loads of its result cells; resolve the
	// value stored on this path, or a nil error behind a defer reads as
	// unknown and the acquire-for-caller contract is never recognized.
	// libocr's transaction constructor holds a serialization lock for its
	// caller while deferring another unlock:
	// https://github.com/smartcontractkit/libocr/blob/618b5bf7f342075a81ca1273a04abce15529a101/offchainreporting2plus/ocrintegrationtesthelpers/in_memory_key_value_database.go#L196-L215
	result := lifecycle.ReturnedResultWithin(returned, len(returned.Results)-1, query.budget)
	if query.budget.Exhausted() || query.budget.PoolExhausted() {
		return false
	}
	if types.Identical(last, types.Universe.Lookup("error").Type()) {
		return ssaflow.DefinitelyNilWithin(result, query.budget) || query.nilGuardDominatesReturn(result, returned)
	}
	if basic, ok := last.Underlying().(*types.Basic); ok && basic.Kind() == types.Bool {
		return !constantFalse(result)
	}
	return true
}

// A Boolean result can mean "already unlocked", not necessarily success.
// Infer no naming convention: require every normal return to expose one exact
// held-state polarity and every known caller to release the same global mutex
// on that polarity. Opaque/escaping callers leave this contract unknown.
// https://github.com/fortio/fortio/blob/5c19725ff61c9f7ad944b91ec32d96a399341d87/fnet/network.go#L328-L393
func (query lockReturnQueries) conditionalCallerRelease(
	function *ssa.Function, values []ssa.Value, heldAt map[*ssa.Return]lockReturnState, identity string, callers conditionalCallerSet,
) callerReleaseProof {
	unknown := callerReleaseProof{reason: lockReasonConditionalCallerReleaseUnknown}
	if len(values) != 1 || callers.Escaped || len(callers.Calls) == 0 {
		return unknown
	}
	global, ok := values[0].(*ssa.Global)
	if !ok || !syntax.NamedType(global.Type(), "sync", "Mutex") {
		return unknown
	}
	for index := range function.Signature.Results().Len() {
		if !query.budget.Spend() {
			return callerReleaseProof{reason: lockReasonLockStateBudgetExhausted}
		}
		heldWhen, known := query.heldResultPolarity(heldAt, identity, index)
		if known && !slices.ContainsFunc(callers.Calls, func(call *ssa.Call) bool {
			return !query.callerReleasesOnFlag(call, global, heldWhen)
		}) && !query.budget.Exhausted() {
			return callerReleaseProof{proven: true, reason: lockReasonConditionalCallerReleaseProven}
		}
	}
	return unknown
}

// heldResultPolarity returns the result condition the lock is held under:
// every return that holds it has one Boolean value in result index, and every
// return that does not has the other.
func (query lockReturnQueries) heldResultPolarity(heldAt map[*ssa.Return]lockReturnState, identity string, index int) (ssacall.CallCondition, bool) {
	var held, unheld, sawHeld, sawUnheld bool
	for _, returned := range query.setup.returns {
		if !query.budget.Spend() {
			return ssacall.CallCondition{}, false
		}
		truth, known := lockBooleanValue(lifecycle.ReturnedResultWithin(returned, index, query.budget), nil)
		if !known {
			return ssacall.CallCondition{}, false
		}
		state, observed := heldAt[returned]
		definite := query.containsLock(state.definite, identity)
		if !observed || definite != query.containsLock(state.possible, identity) || query.budget.Exhausted() {
			return ssacall.CallCondition{}, false
		}
		if definite {
			if sawHeld && held != truth {
				return ssacall.CallCondition{}, false
			}
			held, sawHeld = truth, true
		} else {
			if sawUnheld && unheld != truth {
				return ssacall.CallCondition{}, false
			}
			unheld, sawUnheld = truth, true
		}
	}
	outcome := ssacall.OutcomeFalse
	if held {
		outcome = ssacall.OutcomeTrue
	}
	return ssacall.CallCondition{Result: index, Outcome: outcome}, sawHeld && sawUnheld && held != unheld && !query.budget.Exhausted()
}

// callerReleasesOnFlag reports whether the caller branches on the result the
// lock is held under and releases the mutex on the held arm before every
// return, while the other arm owes nothing.
func (query lockReturnQueries) callerReleasesOnFlag(call *ssa.Call, mutex *ssa.Global, heldWhen ssacall.CallCondition) bool {
	if !query.budget.Spend() {
		return false
	}
	index := heldWhen.Result
	block := call.Block()
	if cfg.BlockInCycleWithin(block, query.budget) || len(block.Succs) != 2 {
		return false
	}
	branch, ok := block.Instrs[len(block.Instrs)-1].(*ssa.If)
	if call.Common().Signature().Results().Len() == 1 {
		index = -1
	}
	if !ok || branch.Cond != ssacall.CallResultWithin(call, index, query.budget) {
		return false
	}
	unheld := block.Succs[0]
	if heldWhen.Outcome == ssacall.OutcomeTrue {
		unheld = block.Succs[1]
	}
	if len(unheld.Preds) != 1 {
		return false
	}
	witness := false
	owns := func(instruction ssa.Instruction) bool {
		if instruction.Block() == unheld {
			return true
		}
		operation, _, receiver, direct := mutexActionWithin(instruction, query.budget)
		if direct && operation == mutexRelease && receiver == mutex && !readModeRelease(instruction) {
			witness = true
			return true
		}
		return false
	}
	outcome := ssapath.EvaluateObligation(ssapath.ObligationFlow{
		Start: call, Budget: query.budget, Instruction: ssapath.ExactOrNone(owns),
	})
	return witness && outcome == ssapath.ObligationHonored && !query.budget.Exhausted()
}

func (query lockReturnQueries) containsLock(held []string, identity string) bool {
	for _, candidate := range held {
		if !query.budget.Spend() {
			return false
		}
		if candidate == identity {
			return true
		}
	}
	return false
}

// A held-on-success helper can return the original checked error instead of a
// literal nil. Match the exact SSA result before using branch feasibility:
// an error derived from it, or another loop iteration's merge, is not enough.
// https://github.com/DrmagicE/gmqtt/blob/92ed7d60915519f60c3d3cdb6420b1e11eb824e2/server/server.go#L309-L340
func (query lockReturnQueries) nilGuardDominatesReturn(value ssa.Value, returned *ssa.Return) bool {
	for _, branch := range query.setup.branches {
		if !query.budget.Spend() {
			return false
		}
		comparison, ok := branch.Cond.(*ssa.BinOp)
		if !ok || comparison.X != value && comparison.Y != value {
			continue
		}
		for _, successor := range branch.Block().Succs {
			if !query.budget.Spend() {
				return false
			}
			success, known := ssapath.SuccessBranchWithin(branch.Block(), successor, value, query.budget)
			if known && success && len(successor.Preds) == 1 && successor.Dominates(returned.Block()) {
				return true
			}
		}
	}
	return false
}

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
