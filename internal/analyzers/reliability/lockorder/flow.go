package lockorder

import (
	"go/constant"
	"go/token"
	"go/types"
	"slices"

	"github.com/kojah/gohawk/internal/check"
	"github.com/kojah/gohawk/internal/ssaflow"
	analysisTrace "github.com/kojah/gohawk/internal/trace"

	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/ssa"
)

// lockCompletionBudget bounds one "does this callee release my lock?" question
// by the instructions it may examine. Mutually recursive helpers make the
// number of routes through a call graph explode, and an answer the cycle guard
// cuts short cannot be memoized, so an unbounded search re-walks the graph once
// per route. A package of eighteen mutually recursive methods with four calls
// each took over seven seconds before this bound and is instant with it.
const lockCompletionBudget = 250_000

// releaseSettled reports whether the analyzer may treat the lock as released
// at this instruction: the search proved the release with the launch form the
// rule accepts, or it was abandoned before it could decide. missing-release
// reports a lock the analysis proves is still held, so an undecided release
// has to suppress. Leaving it held would let a walk the analyzer gave up on
// produce a defect-tier diagnostic.
func releaseSettled(proof ssaflow.CompletionProof, reason ssaflow.EvidenceReason) bool {
	return proof.Reason == ssaflow.EvidenceBudgetExhausted || proof.Proven() && proof.Reason == reason
}

type lockFlowContext struct {
	pass        *analysis.Pass
	evidence    *ssaflow.LocalEvidence
	relations   map[lockRelation]token.Pos
	keys        map[string]string
	calleeLocks *calleeLockSearch
	lockValues  map[string][]ssa.Value
	acquiredAt  map[string]token.Pos
	released    map[string]bool
	callerOwned map[string]bool
	defers      []*ssa.Defer
}

func walkLockOrder(
	pass *analysis.Pass,
	function *ssa.Function,
	relations map[lockRelation]token.Pos,
	keys map[string]string,
	calleeLocks *calleeLockSearch,
	evidence *ssaflow.LocalEvidence,
) {
	if len(function.Blocks) == 0 {
		return
	}
	// The walk is a work list over (block, held locks, deferred releases,
	// guards); a state is revisited only when that tuple is new, which bounds
	// the walk on loops while still separating the path that acquired a lock
	// from the path that did not.
	released := map[string]bool{}
	acquiredAt := map[string]token.Pos{}
	lockValues := map[string][]ssa.Value{}
	unreleasedReturns := map[string][]token.Pos{}
	heldAtReturn := map[string]map[*ssa.Return]bool{}
	acquisitions := map[string][]ssa.Instruction{}
	callerOwned := callerOwnedLocks(function)
	functionDefers := ssaflow.InstructionsOf[*ssa.Defer](function)
	flow := lockFlowContext{
		pass:        pass,
		evidence:    evidence,
		relations:   relations,
		keys:        keys,
		calleeLocks: calleeLocks,
		lockValues:  lockValues,
		acquiredAt:  acquiredAt,
		released:    released,
		callerOwned: callerOwned,
		defers:      functionDefers,
	}
	ssaflow.WalkStates([]lockFlowState{{block: function.Blocks[0]}}, lockStateKey, func(state lockFlowState) ([]lockFlowState, bool) {
		held := slices.Clone(state.held)
		readHeld := slices.Clone(state.readHeld)
		deferred := slices.Clone(state.deferred)
		guards := cloneLockGuards(state.guards)
		condition := state.condition
		if len(state.block.Preds) > 1 {
			condition = ""
		}
		for _, instruction := range state.block.Instrs {
			recordUnreleasedLocks(instruction, held, deferred, lockValues, unreleasedReturns, heldAtReturn)
			held = transferCalledUnlocks(evidence, instruction, held, guards, lockValues, released)
			// An unconditional unlock at the start of a spawned closure transfers
			// the held lock to that goroutine. Requiring it before any branch keeps
			// conditional handoffs from hiding a genuinely unreleased return path.
			held = transferSpawnedUnlocks(evidence, instruction, held, guards, lockValues, released)
			// A lock whose owner is handed to something the analysis cannot see
			// through may be released there, so a later return proves nothing.
			held = transferOpaqueUnlocks(instruction, held, guards, lockValues, released)
			// Treat an Unlock inside a deferred closure as return-path cleanup even
			// when guarded by state. This supports early-unlock patterns where the
			// defer handles only earlier returns:
			// https://github.com/containerd/containerd/blob/716cbaf51212adb5e80ca1c30b644bfeb9c9d779/integration/nri_test.go#L1287-L1300
			deferred = recordDeferredUnlocks(evidence, instruction, held, deferred, lockValues, released)
			operation, identity, receiver, ok := mutexAction(instruction)
			if !ok {
				flow.recordCalledOrder(instruction, held)
				reportReadLockWrites(pass, instruction, held, readHeld, lockValues)
				continue
			}
			// Acquisitions are remembered so the acquire-for-caller contract can
			// ask which returns an acquisition dominates.
			if operation == mutexAcquire {
				acquisitions[identity] = appendUniqueInstruction(acquisitions[identity], instruction)
			}
			actionState := lockFlowState{
				held: held, readHeld: readHeld, deferred: deferred, guards: guards,
				condition: condition, conditionValue: state.conditionValue,
			}
			actionState = flow.applyMutexAction(instruction, operation, identity, receiver, actionState)
			held, readHeld, guards = actionState.held, actionState.readHeld, actionState.guards
			deferred = actionState.deferred
		}
		return lockSuccessorStates(pass, state.block, held, readHeld, deferred, guards), true
	})
	// A lock is reported only when some path releases it and another returns
	// with it held: a lock never released anywhere is either transferred to a
	// caller or held for the function's whole life, and both are contracts the
	// flow cannot distinguish from a leak. A function whose successful returns
	// all hold the lock is acquiring it for its caller, so its held returns are
	// the contract rather than the defect.
	for identity, returns := range unreleasedReturns {
		if !released[identity] || acquiresForCaller(function, acquisitions[identity], heldAtReturn[identity]) {
			continue
		}
		for _, position := range returns {
			if position == token.NoPos {
				position = acquiredAt[identity]
			}
			check.Reportf(pass, check.LockMissingRelease, position, "lock %s is not released on this return path", identity)
		}
	}
}

func recordUnreleasedLocks(
	instruction ssa.Instruction,
	held, deferred []string,
	lockValues map[string][]ssa.Value,
	unreleased map[string][]token.Pos,
	heldAtReturn map[string]map[*ssa.Return]bool,
) {
	returned, ok := instruction.(*ssa.Return)
	if !ok {
		return
	}
	for _, identity := range held {
		if !slices.Contains(deferred, identity) && !returnedUnlockOwner(returned, lockValues[identity]) {
			unreleased[identity] = appendUniquePosition(unreleased[identity], returned.Pos())
			if heldAtReturn[identity] == nil {
				heldAtReturn[identity] = map[*ssa.Return]bool{}
			}
			heldAtReturn[identity][returned] = true
		}
	}
}

// acquiresForCaller reports whether the function's contract is to return with
// the lock held: every successful return that an acquisition dominates, one
// that returns no error or a nil error, still holds it, and at least one such
// return exists. A helper that begins a critical section for its caller, with
// a matching helper that ends it, has this shape; a function that forgets an
// unlock on one successful path, or acquires only conditionally, does not.
// crabbox pairs beginOperation with endOperation:
// https://github.com/openclaw/crabbox/blob/3ef3f98cbe27e6ddc814c11fde15b89c1639bcbe/internal/providers/incus/client.go#L161-L185
func acquiresForCaller(function *ssa.Function, acquisitions []ssa.Instruction, heldAt map[*ssa.Return]bool) bool {
	successful := 0
	for _, block := range function.Blocks {
		for _, instruction := range block.Instrs {
			returned, ok := instruction.(*ssa.Return)
			if !ok || !successfulReturn(function, returned) {
				continue
			}
			dominated := slices.ContainsFunc(acquisitions, func(acquisition ssa.Instruction) bool {
				return ssaflow.InstructionDominates(acquisition, returned)
			})
			if !dominated {
				continue
			}
			successful++
			if !heldAt[returned] {
				return false
			}
		}
	}
	return successful > 0
}

func appendUniqueInstruction(instructions []ssa.Instruction, instruction ssa.Instruction) []ssa.Instruction {
	if slices.Contains(instructions, instruction) {
		return instructions
	}
	return append(instructions, instruction)
}

// successfulReturn reports whether the return signals success by Go's
// result conventions: a trailing error result must be nil, and a trailing
// Boolean result, the comma-ok shape, must not be the constant false. A
// claim helper that returns (claim, false) after releasing the lock and
// (claim, true) while holding it is acquiring for its caller. kandev claims
// prompt completions this way:
// https://github.com/kdlbs/kandev/blob/17da0aafe33df01828e21fc79cc9dd156dc088dc/apps/backend/internal/agent/runtime/lifecycle/manager_events.go#L238-L272
func successfulReturn(function *ssa.Function, returned *ssa.Return) bool {
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
	result := ssaflow.ReturnedResult(returned, len(returned.Results)-1)
	if types.Identical(last, types.Universe.Lookup("error").Type()) {
		return ssaflow.DefinitelyNil(result)
	}
	if basic, ok := last.Underlying().(*types.Basic); ok && basic.Kind() == types.Bool {
		return !constantFalse(result)
	}
	return true
}

func constantFalse(value ssa.Value) bool {
	literal, ok := value.(*ssa.Const)
	return ok && literal.Value != nil && literal.Value.Kind() == constant.Bool && !constant.BoolVal(literal.Value)
}

func possiblyDeferredUnlock(
	evidence *ssaflow.LocalEvidence,
	acquisition ssa.Instruction,
	functionDefers []*ssa.Defer,
	values []ssa.Value,
) bool {
	for _, deferred := range functionDefers {
		if !ssaflow.InstructionDominates(deferred, acquisition) {
			continue
		}
		for _, value := range values {
			proof := evidence.Completion(ssaflow.CompletionRequest{
				Instruction: deferred,
				Target:      value,
				Methods:     []string{"Unlock", "RUnlock"},
				Coverage:    ssaflow.CoverageAnywhere,
				Budget:      ssaflow.NewSearchBudget(lockCompletionBudget),
			})
			if releaseSettled(proof, ssaflow.EvidenceDeferredCompletion) {
				// A defer registered before acquisition can conditionally release the
				// exact lock using state established after Lock. Without proving the
				// deferred guard false, a missing-release defect is uncertain. Telekom's
				// artifact store uses this rollback shape:
				// https://github.com/telekom/k8s-breakglass/blob/9b078a5e78c5663cfdf8b7711ff24fc2a6aaee59/pkg/artifacts/storage/local/local.go#L265-L329
				return true
			}
		}
	}
	return false
}

func transferCalledUnlocks(
	evidence *ssaflow.LocalEvidence,
	instruction ssa.Instruction,
	held []string,
	guards map[string]lockGuard,
	lockValues map[string][]ssa.Value,
	released map[string]bool,
) []string {
	for _, identity := range slices.Clone(held) {
		for _, value := range lockValues[identity] {
			proof := evidence.Completion(ssaflow.CompletionRequest{
				Instruction: instruction,
				Target:      value,
				Methods:     []string{"Unlock", "RUnlock"},
				Budget:      ssaflow.NewSearchBudget(lockCompletionBudget),
			})
			if !releaseSettled(proof, ssaflow.EvidenceCalledCompletion) {
				continue
			}
			// A synchronous helper or immediately invoked closure that releases the
			// exact lock unconditionally consumes the caller's held-lock obligation.
			// gRPC funnels an exit-idle failure through updateResolverStateAndUnlock:
			// https://github.com/grpc/grpc-go/blob/9f8027448a64b6446d0c7256a1efe907b1cb6b1b/clientconn.go#L416-L419
			// NATS funnels publish failures through a local closure that may notify
			// an error callback first but still unlocks on every normal return:
			// https://github.com/nats-io/nats.go/blob/850f889cf3d63bfd1a549ab9af59f0145146fb41/js.go#L906-L976
			released[identity] = true
			held = releaseLock(held, identity)
			delete(guards, identity)
			break
		}
	}
	return held
}

func transferSpawnedUnlocks(
	evidence *ssaflow.LocalEvidence,
	instruction ssa.Instruction,
	held []string,
	guards map[string]lockGuard,
	lockValues map[string][]ssa.Value,
	released map[string]bool,
) []string {
	if _, ok := instruction.(*ssa.Go); !ok {
		return held
	}
	for _, identity := range slices.Clone(held) {
		for _, value := range lockValues[identity] {
			proof := evidence.Completion(ssaflow.CompletionRequest{
				Instruction: instruction,
				Target:      value,
				Methods:     []string{"Unlock", "RUnlock"},
				Budget:      ssaflow.NewSearchBudget(lockCompletionBudget),
			})
			if releaseSettled(proof, ssaflow.EvidenceStartedCompletion) {
				// A spawned helper may branch before releasing the caller's lock as
				// long as every normal return performs the release. gRPC transfers
				// addrConn.mu to its reconnect worker this way:
				// https://github.com/grpc/grpc-go/blob/9f8027448a64b6446d0c7256a1efe907b1cb6b1b/clientconn.go#L1071
				released[identity] = true
				held = releaseLock(held, identity)
				delete(guards, identity)
				break
			}
		}
	}
	return held
}

func recordDeferredUnlocks(
	evidence *ssaflow.LocalEvidence,
	instruction ssa.Instruction,
	held, deferred []string,
	lockValues map[string][]ssa.Value,
	released map[string]bool,
) []string {
	if _, ok := instruction.(*ssa.Defer); !ok {
		return deferred
	}
	for _, identity := range slices.Clone(held) {
		for _, value := range lockValues[identity] {
			// A deferred literal that releases on some path makes the release
			// data-dependent, typically through an "already unlocked" flag.
			// Missing-release diagnostics need the release to be impossible, so
			// this asks only whether the defer may unlock.
			proof := evidence.Completion(ssaflow.CompletionRequest{
				Instruction: instruction,
				Target:      value,
				Methods:     []string{"Unlock", "RUnlock"},
				Coverage:    ssaflow.CoverageAnywhere,
				Budget:      ssaflow.NewSearchBudget(lockCompletionBudget),
			})
			if releaseSettled(proof, ssaflow.EvidenceDeferredCompletion) {
				released[identity] = true
				deferred = appendUniqueString(deferred, identity)
				break
			}
		}
	}
	return deferred
}

// recordCalledOrder orders the locks held at a call before every lock the
// callee takes. The held set is the one the transfer rules above already
// adjusted, so a lock handed to a goroutine or to code the analysis cannot see
// through is no longer held here and orders nothing.
func (flow lockFlowContext) recordCalledOrder(instruction ssa.Instruction, held []string) {
	if len(held) == 0 {
		return
	}
	call, ok := instruction.(*ssa.Call)
	if !ok {
		return
	}
	locks := flow.calleeLocks.locks(call.Common().StaticCallee())
	for _, owner := range held {
		for _, class := range locks.acquires {
			recordOrder(flow.pass, instruction.Pos(), flow.relations, flow.keys[owner], class)
		}
	}
}

func (flow lockFlowContext) applyMutexAction(
	instruction ssa.Instruction,
	operation mutexOperation,
	identity string,
	receiver ssa.Value,
	state lockFlowState,
) lockFlowState {
	if operation == mutexRelease {
		flow.released[identity] = true
		if _, deferredRelease := instruction.(*ssa.Defer); deferredRelease {
			// A deferred unlock remains effective on every later trip around a
			// control-flow loop. Unique identities let the dataflow reach a fixed
			// point instead of growing without bound.
			state.deferred = appendUniqueString(state.deferred, identity)
			return state
		}
		delete(state.guards, identity)
		state.held = releaseLock(state.held, identity)
		state.readHeld = releaseLock(state.readHeld, identity)
		return state
	}
	flow.lockValues[identity] = appendLockValue(flow.lockValues[identity], receiver)
	// A mutex selected from a map, slice, or loop-carried value may represent a
	// different runtime lock on every iteration. Collapsing those values into one
	// SSA identity creates recursive-acquire and missing-release false positives:
	// https://github.com/caidaoli/ccLoad/blob/9ed11fe1b1dd2bfed12a32c9290354ff3cdc9b77/internal/cursorauth/sdk_runner.go#L410-L470
	// https://github.com/kubernetes/kubernetes/blob/e72c2715ade37738aa5c029e8de5285cbe1c9441/pkg/kubelet/images/pullmanager/locks.go#L56-L65
	if dynamicIndexedMutex(receiver) {
		return state
	}
	// A release before the first acquisition means this helper borrowed a
	// caller-held lock. Reacquiring restores the caller's state; it does not make
	// the helper responsible for a subsequent unlock:
	// https://github.com/kubernetes/kubernetes/blob/e72c2715ade37738aa5c029e8de5285cbe1c9441/pkg/kubelet/cm/devicemanager/manager.go#L1065-L1075
	if flow.callerOwned[identity] {
		return state
	}
	if state.condition != "" {
		state.guards[identity] = lockGuard{condition: state.condition, value: state.conditionValue}
	}
	if flow.acquiredAt[identity] == token.NoPos {
		flow.acquiredAt[identity] = instruction.Pos()
	}
	if possiblyDeferredUnlock(flow.evidence, instruction, flow.defers, flow.lockValues[identity]) {
		flow.released[identity] = true
		state.deferred = appendUniqueString(state.deferred, identity)
	}
	flow.keys[identity] = lockComparisonKey(identity, receiver)
	if readModeAcquisition(instruction) {
		state.readHeld = appendUniqueString(state.readHeld, identity)
	}
	state.held = acquireLock(flow.pass, instruction, state.held, identity, flow.keys, flow.relations)
	return state
}

func lockSuccessorStates(
	pass *analysis.Pass,
	block *ssa.BasicBlock,
	held, readHeld, deferred []string,
	guards map[string]lockGuard,
) []lockFlowState {
	states := make([]lockFlowState, 0, len(block.Succs))
	for index, successor := range block.Succs {
		nextCondition, nextValue := "", false
		if condition, ok := blockCondition(block); ok && len(block.Succs) == 2 {
			nextCondition, nextValue = condition, index == 0
			if guardConflicts(held, guards, condition, nextValue) {
				traceRepeatedConditionPruning(pass, block)
				continue
			}
		}
		states = append(states, lockFlowState{
			block: successor, held: held, readHeld: readHeld, deferred: deferred, guards: guards,
			condition: nextCondition, conditionValue: nextValue,
		})
	}
	return states
}

func traceRepeatedConditionPruning(pass *analysis.Pass, block *ssa.BasicBlock) {
	checkID := string(check.LockMissingRelease)
	if !analysisTrace.Enabled("lockorder", checkID) || len(block.Instrs) == 0 {
		return
	}
	branch := block.Instrs[len(block.Instrs)-1]
	analysisTrace.For(pass, "lockorder", checkID, branch.Pos()).Evidence(analysisTrace.Step{
		Reason:   "repeated-condition-infeasible",
		Outcome:  analysisTrace.OutcomeAccepted,
		Pos:      branch.Pos(),
		Function: branch.Parent().String(),
	})
}

// transferOpaqueUnlocks drops a held lock whose owner is handed across a
// boundary the analysis cannot see through: an interface method or a
// function value. The callee may unlock, so the obligation is unknown rather
// than violated from that point. A static callee is judged by the completion
// proof instead.
func transferOpaqueUnlocks(
	instruction ssa.Instruction,
	held []string,
	guards map[string]lockGuard,
	lockValues map[string][]ssa.Value,
	released map[string]bool,
) []string {
	common := ssaflow.InstructionCall(instruction)
	if common == nil || !opaqueCallee(common) {
		return held
	}
	for _, identity := range slices.Clone(held) {
		for _, value := range lockValues[identity] {
			if !lockHandedTo(common, value) {
				continue
			}
			released[identity] = true
			held = releaseLock(held, identity)
			delete(guards, identity)
			break
		}
	}
	return held
}

// opaqueCallee reports whether the call is dispatched at run time: an
// interface method or a function value. A static callee, even one whose body
// is not loaded, is a known function the completion proof can judge.
func opaqueCallee(common *ssa.CallCommon) bool {
	if common.IsInvoke() {
		return true
	}
	if _, ok := common.Value.(*ssa.Builtin); ok {
		return false
	}
	return common.StaticCallee() == nil
}

// lockHandedTo reports whether an argument is the lock, its owner, or a
// value the lock derives from, such as the struct whose field it is.
func lockHandedTo(common *ssa.CallCommon, lock ssa.Value) bool {
	for _, argument := range common.Args {
		if ssaflow.SameValue(argument, lock) || ssaflow.ValueDerivesFrom(lock, argument, map[ssa.Value]bool{}) {
			return true
		}
	}
	return false
}
