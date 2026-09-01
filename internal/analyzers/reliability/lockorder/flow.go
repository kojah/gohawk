package lockorder

import (
	"go/token"
	"slices"
	"strings"

	ssautil "github.com/kojah/gohawk/internal/analysisutil/ssa"
	"github.com/kojah/gohawk/internal/check"

	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/ssa"
)

func walkLockOrder(pass *analysis.Pass, function *ssa.Function, relations map[lockRelation]token.Pos) {
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
			if returned, ok := instruction.(*ssa.Return); ok {
				for _, identity := range held {
					if !slices.Contains(deferred, identity) && !returnedUnlockOwner(returned, lockValues[identity]) {
						unreleasedReturns[identity] = appendUniquePosition(unreleasedReturns[identity], returned.Pos())
					}
				}
			}
			// An unconditional unlock at the start of a spawned closure transfers
			// the held lock to that goroutine. Requiring it before any branch keeps
			// conditional handoffs from hiding a genuinely unreleased return path.
			if _, ok := instruction.(*ssa.Go); ok {
				for _, identity := range slices.Clone(held) {
					for _, value := range lockValues[identity] {
						if ssautil.ClosureCallsMethodBeforeBranch(instruction, "Unlock", value) ||
							ssautil.ClosureCallsMethodBeforeBranch(instruction, "RUnlock", value) {
							released[identity] = true
							held = releaseLock(held, identity)
							delete(guards, identity)
							break
						}
					}
				}
			}
			// Treat an Unlock inside a deferred closure as return-path cleanup even
			// when guarded by state. This supports early-unlock patterns where the
			// defer handles only earlier returns:
			// https://github.com/containerd/containerd/blob/716cbaf51212adb5e80ca1c30b644bfeb9c9d779/integration/nri_test.go#L1287-L1300
			if _, ok := instruction.(*ssa.Defer); ok {
				for _, identity := range slices.Clone(held) {
					for _, value := range lockValues[identity] {
						common := ssautil.InstructionCall(instruction)
						if ssautil.DeferredClosureCalls(instruction, "Unlock", value) || ssautil.DeferredClosureCalls(instruction, "RUnlock", value) ||
							common != nil &&
								(ssautil.ValueCallsMethod(common.Value, "Unlock", value) || ssautil.ValueCallsMethod(common.Value, "RUnlock", value)) {
							released[identity] = true
							deferred = appendUniqueString(deferred, identity)
							break
						}
					}
				}
			}
			operation, identity, receiver, ok := mutexAction(instruction)
			if !ok {
				continue
			}
			switch operation {
			case mutexAcquire:
				lockValues[identity] = appendLockValue(lockValues[identity], receiver)
				// A mutex selected from a map, slice, or loop-carried value may
				// represent a different runtime lock on every iteration. Collapsing
				// those values into one SSA identity creates both recursive-acquire
				// and missing-release false positives, so require a stable identity
				// before reasoning about its lifecycle. ccLoad coordinates a dynamic
				// set of pending calls this way:
				// https://github.com/caidaoli/ccLoad/blob/9ed11fe1b1dd2bfed12a32c9290354ff3cdc9b77/internal/cursorauth/sdk_runner.go#L410-L470
				// Kubernetes likewise acquires a dynamically selected set of lock
				// stripes:
				// https://github.com/kubernetes/kubernetes/blob/e72c2715ade37738aa5c029e8de5285cbe1c9441/pkg/kubelet/images/pullmanager/locks.go#L56-L65
				if dynamicIndexedMutex(receiver) {
					continue
				}
				// A release before the first acquisition means this helper borrowed
				// a caller-held lock. Reacquiring restores the caller's state; it does
				// not make the helper responsible for a subsequent unlock. Kubernetes
				// drops a device-manager mutex around an RPC using this pattern:
				// https://github.com/kubernetes/kubernetes/blob/e72c2715ade37738aa5c029e8de5285cbe1c9441/pkg/kubelet/cm/devicemanager/manager.go#L1065-L1075
				if callerOwned[identity] {
					continue
				}
				if condition != "" {
					guards[identity] = lockGuard{condition: condition, value: state.conditionValue}
				}
				if acquiredAt[identity] == token.NoPos {
					acquiredAt[identity] = instruction.Pos()
				}
				held = acquireLock(pass, instruction, held, identity, relations)
			case mutexRelease:
				released[identity] = true
				if _, isDefer := instruction.(*ssa.Defer); isDefer {
					// A deferred unlock remains effective on every later trip around a
					// control-flow loop. Keeping identities unique lets the dataflow
					// state reach a fixed point instead of growing without bound.
					deferred = appendUniqueString(deferred, identity)
				} else {
					held = releaseLock(held, identity)
					delete(guards, identity)
				}
			}
		}
		for index, successor := range state.block.Succs {
			nextCondition := ""
			nextValue := false
			if condition, ok := blockCondition(state.block); ok && len(state.block.Succs) == 2 {
				nextCondition = condition
				nextValue = index == 0
				if guardConflicts(held, guards, condition, nextValue) {
					continue
				}
			}
			queue = append(queue, lockFlowState{
				block: successor, held: held, deferred: deferred, guards: guards,
				condition: nextCondition, conditionValue: nextValue,
			})
		}
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
