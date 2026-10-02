package lockorder

import (
	"go/token"

	"github.com/kojah/gohawk/internal/lifecycle"
	"github.com/kojah/gohawk/internal/ssaflow"
	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/ssa"
)

// The lock work list owns state expansion, branch evidence and the availability
// barrier before function reports/contracts are published. A state quota limits
// expansions; the shared allowance also charges keys, copies, instruction visits
// and nested branch queries. Prewalk setup and helper/effect/heap queries retain
// their separate costs until their request boundaries are migrated.
const lockStateWorkBudget = 100 * ssaflow.SummaryBudget

type lockStateWalk struct {
	budget            *ssaflow.SearchBudget
	summaries         map[ssa.Instruction][]mutexEffect
	flow              lockFlowContext
	unreleasedReturns map[string][]token.Pos
	heldAtReturn      map[*ssa.Return]lockReturnState
	possibleWriters   []*ssa.Defer
	terminates        ssaflow.Terminator
	remaining         int
}

func (walk *lockStateWalk) run(
	pass *analysis.Pass, function *ssa.Function, relations *lockOrders,
	calleeLocks *calleeLockSearch, evidence *lifecycle.LocalEvidence,
	callers map[*ssa.Function]conditionalCallerSet, exclusive *exclusiveCallers,
) bool {
	if len(function.Blocks) == 0 {
		return true
	}
	// The walk is a work list over (block, held locks, deferred releases,
	// guards); a state is revisited only when that tuple is new, which bounds
	// the walk on loops while still separating the path that acquired a lock
	// from the path that did not.
	released := map[string]bool{}
	acquiredAt := map[string]token.Pos{}
	lockValues := map[string][]ssa.Value{}
	unreleasedReturns := map[string][]token.Pos{}
	heldAtReturn := map[*ssa.Return]lockReturnState{}
	acquisitions := map[string][]ssa.Instruction{}
	uncertainGuards := map[string]bool{}
	possibleWriters := possibleDeferredWriters(function, walk.summaries)
	callerOwned := callerOwnedLocks(function, walk.summaries)
	functionDefers := ssaflow.InstructionsOf[*ssa.Defer](function)
	flow := lockFlowContext{
		pass:         pass,
		function:     function,
		exclusive:    exclusive,
		evidence:     evidence,
		relations:    relations,
		calleeLocks:  calleeLocks,
		lockValues:   lockValues,
		acquiredAt:   acquiredAt,
		released:     released,
		acquisitions: acquisitions, uncertainGuards: uncertainGuards,
		unprovenRelease: map[string]bool{},
		callerOwned:     callerOwned,
		defers:          functionDefers,
		releaseAttempts: newReleaseAttempts(),
	}
	// Each predecessor selects its own phi values before any instruction runs.
	// Clone the lock collections so one successor's release cannot discharge
	// another successor's obligation; the immutable branch facts travel with it.
	walk.flow = flow
	walk.remaining = 4096
	walk.unreleasedReturns, walk.heldAtReturn = unreleasedReturns, heldAtReturn
	walk.possibleWriters = possibleWriters
	walk.terminates = summaryKnowledge.Provider(pass).TerminatesWithin(walk.budget)
	ssaflow.WalkStatesWithin([]lockFlowState{{block: function.Blocks[0]}}, func(state lockFlowState) string {
		return lockStateKey(state, walk.budget)
	}, walk.expand, walk.budget)
	if walk.remaining < 0 || walk.budget.Exhausted() || walk.budget.PoolExhausted() {
		return false
	}
	flow.reportMissingReleases(function, unreleasedReturns, heldAtReturn, callers[function])
	return true
}

func (walk *lockStateWalk) expand(state lockFlowState) ([]lockFlowState, bool) {
	walk.remaining--
	if walk.remaining < 0 {
		return nil, false
	}
	state.constants = lockPhiConstants(state, walk.budget)
	state, copied := cloneLockStateWithin(state, walk.budget)
	if !copied {
		return nil, false
	}
	// Incoming conditions guard only acquisitions before a merge. The successor
	// query chooses its own condition; carried constants and stable guards persist.
	if len(state.block.Preds) > 1 {
		state.condition = ""
	}
	for _, instruction := range state.block.Instrs {
		if !walk.budget.Spend() {
			return nil, false
		}
		// A proven terminating call leaves no normal return with this lock held.
		if ssaflow.InstructionTerminatesWithin(instruction, walk.terminates, walk.budget) {
			return nil, true
		}
		if walk.budget.Exhausted() {
			return nil, false
		}
		state = walk.transfer(instruction, state)
		if walk.budget.Exhausted() {
			return nil, false
		}
	}
	return lockSuccessorStates(walk.flow.pass, state, walk.budget), true
}

func (walk *lockStateWalk) transfer(instruction ssa.Instruction, state lockFlowState) lockFlowState {
	flow := &walk.flow
	recordUnreleasedLocks(instruction, state.held, state.deferred, flow.lockValues, walk.unreleasedReturns, walk.heldAtReturn)
	// A complete sequence replaces fallback release evidence: a helper that
	// releases and reacquires must leave the lock held.
	if effects, complete := walk.summaries[instruction]; complete {
		before := state.held
		for _, effect := range effects {
			if !walk.budget.Spend() {
				return state
			}
			state = flow.applyMutexAction(instruction, effect, state)
		}
		flow.releaseAttempts.recordSummarized(instruction, effects, before, state.held)
		return state
	}
	state.held = transferCalledUnlocks(
		flow.evidence, instruction, state.held, state.guards, flow.lockValues, flow.released, flow.unprovenRelease, flow.releaseAttempts,
	)
	// An unconditional release before any branch transfers to the spawned
	// worker; a conditional release cannot hide an uncovered return.
	state.held = transferSpawnedUnlocks(flow.evidence, instruction, state.held, state.guards, flow.lockValues, flow.released)
	// Opaque owner handoffs make release uncertain rather than prove a defect.
	state.held = transferOpaqueUnlocks(instruction, state.held, state.guards, flow.lockValues, flow.released)
	// A deferred closure may handle only the earlier returns before an explicit
	// unlock; retain that existing return-path cleanup boundary.
	// https://github.com/containerd/containerd/blob/716cbaf51212adb5e80ca1c30b644bfeb9c9d779/integration/nri_test.go#L1287-L1300
	state.deferred = flow.recordDeferredUnlocks(instruction, state.held, state.deferred)
	effect, ok := directMutexEffect(instruction)
	if ok {
		return flow.applyMutexAction(instruction, effect, state)
	}
	flow.recordCalledOrder(instruction, state.held, state.origins)
	reportReadLockWrites(*flow, instruction, state.held, state.readHeld, flow.lockValues, walk.possibleWriters)
	return state
}
