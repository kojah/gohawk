package lockorder

import (
	"go/token"
	"slices"
	"strings"

	"github.com/kojah/gohawk/internal/check"
	"github.com/kojah/gohawk/internal/ssaflow"
	analysisTrace "github.com/kojah/gohawk/internal/trace"

	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/ssa"
)

type lockFlowContext struct {
	pass        *analysis.Pass
	relations   map[lockRelation]token.Pos
	lockValues  map[string][]ssa.Value
	acquiredAt  map[string]token.Pos
	released    map[string]bool
	callerOwned map[string]bool
}

func walkLockOrder(
	pass *analysis.Pass,
	function *ssa.Function,
	relations map[lockRelation]token.Pos,
	evidence *ssaflow.LocalEvidence,
) {
	if len(function.Blocks) == 0 {
		return
	}
	queue := []lockFlowState{{block: function.Blocks[0]}}
	seen := map[string]bool{}
	released := map[string]bool{}
	acquiredAt := map[string]token.Pos{}
	lockValues := map[string][]ssa.Value{}
	unreleasedReturns := map[string][]token.Pos{}
	callerOwned := callerOwnedLocks(function)
	flow := lockFlowContext{
		pass:        pass,
		relations:   relations,
		lockValues:  lockValues,
		acquiredAt:  acquiredAt,
		released:    released,
		callerOwned: callerOwned,
	}
	for len(queue) > 0 {
		state := queue[0]
		queue = queue[1:]
		key := lockStateKey(state)
		if seen[key] {
			continue
		}
		seen[key] = true
		held := slices.Clone(state.held)
		deferred := slices.Clone(state.deferred)
		guards := cloneLockGuards(state.guards)
		condition := state.condition
		if len(state.block.Preds) > 1 {
			condition = ""
		}
		for _, instruction := range state.block.Instrs {
			recordUnreleasedLocks(instruction, held, deferred, lockValues, unreleasedReturns)
			held = transferCalledUnlocks(evidence, instruction, held, guards, lockValues, released)
			// An unconditional unlock at the start of a spawned closure transfers
			// the held lock to that goroutine. Requiring it before any branch keeps
			// conditional handoffs from hiding a genuinely unreleased return path.
			held = transferSpawnedUnlocks(evidence, instruction, held, guards, lockValues, released)
			// Treat an Unlock inside a deferred closure as return-path cleanup even
			// when guarded by state. This supports early-unlock patterns where the
			// defer handles only earlier returns:
			// https://github.com/containerd/containerd/blob/716cbaf51212adb5e80ca1c30b644bfeb9c9d779/integration/nri_test.go#L1287-L1300
			deferred = recordDeferredUnlocks(evidence, instruction, held, deferred, lockValues, released)
			operation, identity, receiver, ok := mutexAction(instruction)
			if !ok {
				continue
			}
			actionState := lockFlowState{
				held: held, deferred: deferred, guards: guards,
				condition: condition, conditionValue: state.conditionValue,
			}
			actionState = flow.applyMutexAction(instruction, operation, identity, receiver, actionState)
			held, deferred, guards = actionState.held, actionState.deferred, actionState.guards
		}
		queue = append(queue, lockSuccessorStates(pass, state.block, held, deferred, guards)...)
	}
	// Lock/Unlock helpers commonly and intentionally return while transferring
	// the critical section to their caller. Without interprocedural call-site
	// evidence, reporting those helpers would trade precision for recall.
	functionName := strings.ToLower(function.Name())
	if strings.HasPrefix(functionName, "lock") || strings.HasPrefix(functionName, "unlock") {
		return
	}
	for identity, returns := range unreleasedReturns {
		if !released[identity] {
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
) {
	returned, ok := instruction.(*ssa.Return)
	if !ok {
		return
	}
	for _, identity := range held {
		if !slices.Contains(deferred, identity) && !returnedUnlockOwner(returned, lockValues[identity]) {
			unreleased[identity] = appendUniquePosition(unreleased[identity], returned.Pos())
		}
	}
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
				Modes:       ssaflow.CompletionByHelper | ssaflow.CompletionInCalledClosureOnEveryReturn,
			})
			if !proof.Proven() {
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
				Modes:       ssaflow.CompletionBeforeBranch | ssaflow.CompletionInStartedClosure,
			})
			if proof.Proven() {
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
			proof := evidence.Completion(ssaflow.CompletionRequest{
				Instruction: instruction,
				Target:      value,
				Methods:     []string{"Unlock", "RUnlock"},
				Modes:       ssaflow.CompletionDeferred | ssaflow.CompletionByDeferredCallback,
			})
			if proof.Proven() {
				released[identity] = true
				deferred = appendUniqueString(deferred, identity)
				break
			}
		}
	}
	return deferred
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
	state.held = acquireLock(flow.pass, instruction, state.held, identity, flow.relations)
	return state
}

func lockSuccessorStates(
	pass *analysis.Pass,
	block *ssa.BasicBlock,
	held, deferred []string,
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
			block: successor, held: held, deferred: deferred, guards: guards,
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
	analysisTrace.Emit(pass, analysisTrace.Event{
		Analyzer: "lockorder",
		Check:    checkID,
		Phase:    "evidence",
		Reason:   "repeated-condition-infeasible",
		Outcome:  analysisTrace.OutcomeAccepted,
		Pos:      branch.Pos(),
		Function: branch.Parent().String(),
	})
}
