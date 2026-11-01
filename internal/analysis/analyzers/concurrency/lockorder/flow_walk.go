package lockorder

import (
	"go/token"
	"maps"
	"slices"

	"github.com/kojah/gohawk/internal/engine/lifecycle"
	proofs "github.com/kojah/gohawk/internal/engine/proof"
	cfg "github.com/kojah/gohawk/internal/engine/ssaflow/cfg"
	ssapath "github.com/kojah/gohawk/internal/engine/ssaflow/path"
	"github.com/kojah/gohawk/internal/reporting/check"
	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/ssa"
)

// The lock work list owns state expansion, branch evidence and the availability
// barrier before function reports/contracts are published. A state quota limits
// expansions; the shared allowance also charges keys, copies, instruction visits
// and nested branch/release queries. Setup shares this pool; identity/heap/type
// query internals retain their separately recorded costs.
const lockStateWorkBudget = 100 * proofs.SummaryBudget

type lockStateWalk struct {
	budget            *proofs.SearchBudget
	fieldEvidence     readLockFieldEvidence
	setup             *lockFunctionSetup
	flow              lockFlowContext
	unreleasedReturns map[string][]token.Pos
	heldAtReturn      map[*ssa.Return]lockReturnState
	possibleWriters   []*ssa.Defer
	terminates        ssapath.Terminator
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
	possibleWriters := walk.setup.possibleWriters
	callerOwned := walk.setup.callerOwned
	functionDefers := walk.setup.defers
	releases := newLockReleaseQueries(evidence, walk.budget)
	releases.edge, _ = summaryKnowledge.Provider(pass).LifecycleEvidence("lockorder", string(check.LockMissingRelease))
	flow := lockFlowContext{
		pass:          pass,
		fieldEvidence: walk.fieldEvidence,
		function:      function,
		setup:         walk.setup,
		budget:        walk.budget,
		exclusive:     exclusive,
		releases:      releases,
		relations:     relations,
		calleeLocks:   calleeLocks,
		lockValues:    lockValues,
		acquiredAt:    acquiredAt,
		released:      released,
		acquisitions:  acquisitions, uncertainGuards: uncertainGuards,
		unprovenRelease: map[string]bool{},
		callerOwned:     callerOwned,
		defers:          functionDefers,
		releaseAttempts: newReleaseAttempts(),
		readLockWrites:  make(map[readLockWriteWitness]bool),
	}
	// Each predecessor selects its own phi values before any instruction runs.
	// Clone the lock collections so one successor's release cannot discharge
	// another successor's obligation; the immutable branch facts travel with it.
	walk.flow = flow
	walk.remaining = 4096
	walk.unreleasedReturns, walk.heldAtReturn = unreleasedReturns, heldAtReturn
	walk.possibleWriters = possibleWriters
	walk.terminates = summaryKnowledge.Provider(pass).TerminatesWithin(walk.budget)
	cfg.WalkStatesWithin([]lockFlowState{{block: function.Blocks[0]}}, func(state lockFlowState) string {
		return lockStateKey(state, walk.budget)
	}, walk.expand, walk.budget)
	if walk.incomplete() {
		return false
	}
	// Final metadata may outlive state expansion. A cutoff here must discard
	// earlier read-lock findings and order edges as well as missing releases.
	flow.reportMissingReleases(function, unreleasedReturns, heldAtReturn, callers[function],
		lockReturnQueries{setup: walk.setup, budget: walk.budget})
	return !walk.incomplete()
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
		if ssapath.InstructionTerminatesWithin(instruction, walk.terminates, walk.budget) {
			return nil, true
		}
		if walk.budget.Exhausted() {
			return nil, false
		}
		state = walk.transfer(instruction, state)
		if walk.incomplete() {
			return nil, false
		}
	}
	return lockSuccessorStates(walk.flow, state, walk.budget), true
}

func (walk *lockStateWalk) transfer(instruction ssa.Instruction, state lockFlowState) lockFlowState {
	flow := &walk.flow
	query := lockReturnQueries{setup: walk.setup, budget: walk.budget}
	query.recordUnreleasedLocks(instruction, state.held, state.deferred, flow.lockValues, walk.unreleasedReturns, walk.heldAtReturn)
	if walk.incomplete() {
		return state
	}
	// A complete sequence replaces fallback release evidence: a helper that
	// releases and reacquires must leave the lock held.
	if effects, complete := walk.setup.summaries[instruction]; complete {
		before := state.held
		for _, effect := range effects {
			if !walk.budget.Spend() {
				return state
			}
			state = flow.applyMutexAction(instruction, effect, state)
			if walk.incomplete() {
				return state
			}
		}
		flow.releaseAttempts.recordSummarized(instruction, effects, before, state.held)
		return state
	}
	state.held = flow.transferCompletedUnlocks(instruction, state)
	if walk.incomplete() {
		return state
	}
	// Opaque owner handoffs make release uncertain rather than prove a defect.
	state.held = transferOpaqueUnlocks(instruction, state.held, state.guards, flow.lockValues, flow.released, walk.budget)
	if walk.incomplete() {
		return state
	}
	// A deferred closure may handle only the earlier returns before an explicit
	// unlock; retain that existing return-path cleanup boundary.
	// https://github.com/containerd/containerd/blob/716cbaf51212adb5e80ca1c30b644bfeb9c9d779/integration/nri_test.go#L1287-L1300
	state.deferred = flow.recordDeferredUnlocks(instruction, state.held, state.deferred)
	if walk.incomplete() {
		return state
	}
	effect, ok := walk.setup.direct[instruction]
	if ok {
		return flow.applyMutexAction(instruction, effect, state)
	}
	flow.recordCalledOrder(instruction, state.held, state.origins)
	reportReadLockWrites(*flow, instruction, state.held, state.readHeld, flow.lockValues, walk.possibleWriters)
	return state
}

// incomplete is the shared publication/expansion barrier. A nested query can
// hit its own cap before the function pool; that answer is still unavailable.
func (walk *lockStateWalk) incomplete() bool {
	return walk.remaining < 0 || walk.budget.Exhausted() || walk.budget.PoolExhausted() ||
		walk.flow.releases.cutoff.Reason == proofs.EvidenceBudgetExhausted
}

// Successor selection for the lock walk: which edges of a block a state
// follows, and which are infeasible under the literal, carried-constant,
// and stable-guard evidence the state holds. Infeasibility here is always
// a proof about the branch, never a guess about a mutable field.

func lockSuccessorStates(flow lockFlowContext, state lockFlowState, budget *proofs.SearchBudget) []lockFlowState {
	pass, block := flow.pass, state.block
	states := make([]lockFlowState, 0, len(block.Succs))
	// A local release flag can merge true and false after only one branch
	// unlocked. Preserve the incoming edge for the shared constant-phi query;
	// exploring both values invents a still-held return on the released path.
	// Carried constants and stable parameter constraints are applied separately.
	// https://github.com/enetx/surf/blob/7da0502899af06f8318f95e632797cb2ac0c6c20/pkg/connectproxy/connectproxy.go#L256-L294
	feasible := ssapath.SuccessorPolicy{Feasible: func(block, predecessor *ssa.BasicBlock) []*ssa.BasicBlock {
		return summaryKnowledge.Provider(pass).FeasibleSuccessors(block, predecessor, budget)
	}}.SuccessorsWithin(block, state.predecessor, budget)
	for index, successor := range block.Succs {
		if !budget.Spend() {
			return nil
		}
		if !slices.Contains(feasible, successor) {
			traceInfeasibleLockBranch(pass, block, lockReasonPredecessorConstantBranchInfeasible)
			continue
		}
		constraints, compatible := extendLockConstraints(state.constraints, block, index == 0, budget)
		if !compatible {
			traceInfeasibleLockBranch(pass, block, lockReasonStableParameterBranchInfeasible)
			continue
		}
		if branch, ok := block.Instrs[len(block.Instrs)-1].(*ssa.If); ok {
			if truth, known := lockBooleanValue(branch.Cond, state.constants); known && truth != (index == 0) {
				traceInfeasibleLockBranch(pass, block, lockReasonCarriedConstantBranchInfeasible)
				continue
			}
		}
		// Correlate only the exact condition identity accepted by the guard
		// policy. A cycle query cut short cannot make a computed value stable.
		nextCondition, nextValue := "", false
		if condition, ok := blockCondition(block, budget); ok && len(block.Succs) == 2 {
			nextCondition, nextValue = condition, index == 0
			if guardConflicts(state.held, state.guards, condition, nextValue, budget) {
				traceInfeasibleLockBranch(pass, block, lockReasonRepeatedConditionInfeasible)
				continue
			}
		}
		// A conditional helper releases on the arm its result selects. The
		// successor's held set loses that lock only here; the sibling edge
		// keeps it and its release obligation. The shared slices and guard
		// map must stay intact for that sibling, so release into clones.
		held, readHeld, guards := state.held, state.readHeld, state.guards
		if released := flow.edgeReleasedLocks(block, successor, held); len(released) != 0 {
			held, readHeld = slices.Clone(held), slices.Clone(readHeld)
			guards = maps.Clone(guards)
			for _, identity := range released {
				held, readHeld = releaseLock(held, identity), releaseLock(readHeld, identity)
				delete(guards, identity)
			}
		}
		states = append(states, lockFlowState{
			block: successor, predecessor: block, held: held, readHeld: readHeld,
			deferred: state.deferred, guards: guards, origins: state.origins,
			condition: nextCondition, conditionValue: nextValue,
			constants:   state.constants,
			constraints: constraints,
		})
	}
	if budget.Exhausted() {
		return nil
	}
	return states
}

// edgeReleasedLocks reports the held locks a conditional helper released on
// this exact branch edge. The completion proof is per edge: a helper that
// unlocks before returning one outcome but returns the other while still
// holding settles only the arm that reaches its releasing returns, so the
// sibling edge keeps the lock and its obligation. Only an If whose condition
// resolves to a call result can carry the condition; other edges answer no.
// https://github.com/kojah/gohawk/issues/57
func (flow lockFlowContext) edgeReleasedLocks(from, to *ssa.BasicBlock, held []string) []string {
	if len(held) == 0 || flow.releases == nil || len(from.Succs) != 2 || len(from.Instrs) == 0 {
		return nil
	}
	if _, branch := from.Instrs[len(from.Instrs)-1].(*ssa.If); !branch {
		return nil
	}
	var released []string
	for _, identity := range held {
		for _, value := range flow.lockValues[identity] {
			if !flow.releases.budget.Spend() {
				return released
			}
			if flow.releases.queryEdge(from, to, value).Proven() {
				flow.released[identity] = true
				released = append(released, identity)
				break
			}
			if flow.releases.cutoff.Reason == proofs.EvidenceBudgetExhausted {
				return released
			}
		}
	}
	return released
}
