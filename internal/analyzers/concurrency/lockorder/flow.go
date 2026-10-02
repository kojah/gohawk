package lockorder

import (
	"go/token"
	"go/types"
	"slices"

	"github.com/kojah/gohawk/internal/heapmodel"
	"github.com/kojah/gohawk/internal/lifecycle"
	"github.com/kojah/gohawk/internal/ssaflow"
	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/ssa"
)

type lockFlowContext struct {
	pass            *analysis.Pass
	function        *ssa.Function
	setup           *lockFunctionSetup
	budget          *ssaflow.SearchBudget
	exclusive       *exclusiveCallers
	releases        *lockReleaseQueries
	relations       *lockOrders
	calleeLocks     *calleeLockSearch
	lockValues      map[string][]ssa.Value
	acquiredAt      map[string]token.Pos
	released        map[string]bool
	acquisitions    map[string][]ssa.Instruction
	uncertainGuards map[string]bool
	// unprovenRelease marks locks a callee may release without proving it does
	// so on every path. The lock stays held, because a return that leaves it
	// held is still worth reporting, but it is no longer proven held, so it
	// cannot serve as an exclusive guard.
	unprovenRelease map[string]bool
	callerOwned     map[string]bool
	defers          []*ssa.Defer
	// releaseAttempts records, only while tracing, each call the completion
	// search could not prove releases a held lock, so a reported return can
	// say which helpers were asked and why each did not count.
	releaseAttempts *releaseAttempts
	readLockWrites  map[readLockWriteWitness]bool
}

func appendUniqueInstruction(instructions []ssa.Instruction, instruction ssa.Instruction) []ssa.Instruction {
	if slices.Contains(instructions, instruction) {
		return instructions
	}
	return append(instructions, instruction)
}

// recordCalledOrder orders the locks held at a call before every lock the
// callee takes. The held set is the one the transfer rules above already
// adjusted, so a lock handed to a goroutine or to code the analysis cannot see
// through is no longer held here and orders nothing.
func (flow lockFlowContext) recordCalledOrder(instruction ssa.Instruction, held []string, origins map[string]lockAcquisition) {
	if len(held) == 0 {
		return
	}
	call, ok := instruction.(*ssa.Call)
	if !ok {
		return
	}
	locks := flow.calleeLocks.locksAt(call)
	for _, owner := range held {
		for _, acquired := range locks.acquires {
			flow.relations.record(flow.pass, origins[owner], acquired.through(call))
		}
	}
}

func (flow lockFlowContext) applyMutexAction(
	instruction ssa.Instruction,
	effect mutexEffect,
	state lockFlowState,
) lockFlowState {
	flow.releaseAttempts.recordAction(instruction, effect)
	operation, identity, receiver := effect.operation, effect.identity, effect.receiver
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
		return flow.transferPossiblyAliasedUnlock(instruction, effect, state)
	}
	// Registering a deferred acquisition does not acquire the lock now. In
	// particular, a temporary upgrade can defer restoring the reader state
	// after its writer unlock. This walk does not execute pending acquisitions
	// at return, so it must not invent their held state in the function body.
	// https://github.com/refraction-networking/utls/blob/23b1dac19c06c51e278468e29ac329eec605a31f/common.go#L1100-L1119
	if _, deferredAcquisition := instruction.(*ssa.Defer); deferredAcquisition {
		return state
	}
	// Acquisitions, whether direct or summarized, participate in the same
	// acquire-for-caller contract and loaded-condition uncertainty boundary.
	traceFreshMutexIdentity(flow.pass, instruction, receiver)
	flow.acquisitions[identity] = appendUniqueInstruction(flow.acquisitions[identity], instruction)
	flow.uncertainGuards[identity] = flow.uncertainGuards[identity] || optionalLoadedGuard(instruction, identity)
	flow.lockValues[identity] = appendLockValue(flow.lockValues[identity], receiver)
	// A mutex selected from a map, slice, or loop-carried value may represent a
	// different runtime lock on every iteration. Collapsing those values into one
	// SSA identity creates missing-release and ordering false positives:
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
	if flow.possiblyDeferredUnlock(instruction, flow.lockValues[identity]) {
		flow.released[identity] = true
		state.deferred = appendUniqueString(state.deferred, identity)
	}
	acquired := effect.acquired
	if !slices.Contains(state.held, identity) {
		guards := flow.exclusiveGlobalGuards(state.held, state.readHeld)
		// An object nobody else can reach yet is locked without ordering
		// anything; the lock is still held from here on.
		if len(state.held) == 0 || !flow.exclusive.acquisitionExclusive(flow.function, instruction, receiver) {
			for _, owner := range state.held {
				flow.relations.record(flow.pass, state.origins[owner], acquired, guards...)
			}
		}
		state.origins[identity] = acquired
	}
	if acquired.read {
		state.readHeld = appendUniqueString(state.readHeld, identity)
	} else if !slices.Contains(state.held, identity) {
		// A helper or an opaque release can remove held without carrying its
		// mode forward. A fresh writer acquisition establishes the new mode.
		state.readHeld = releaseLock(state.readHeld, identity)
	}
	state.held = appendUniqueString(state.held, identity)
	return state
}

// A release capability consumed through a possible alias makes the held state
// unknown. It supplies no exact release witness or caller-transfer guarantee.
// Separate identities can arise when a captured owner cell becomes opaque:
// https://github.com/centrifugal/centrifuge-go/blob/080126041ccc71654718bd0601b920ff8b22a8bf/client.go#L1435-L1762
func (flow lockFlowContext) transferPossiblyAliasedUnlock(
	instruction ssa.Instruction, effect mutexEffect, state lockFlowState,
) lockFlowState {
	for _, identity := range slices.Clone(state.held) {
		if identity == effect.identity || !heapmodel.MayAliasAny(effect.receiver, flow.lockValues[identity]) {
			continue
		}
		flow.releaseAttempts.traceUnknownRelease(flow.pass, identity, flow.acquiredAt[identity], instruction)
		delete(state.guards, identity)
		state.held = releaseLock(state.held, identity)
		state.readHeld = releaseLock(state.readHeld, identity)
	}
	return state
}

// transferOpaqueUnlocks drops a held lock handed across an unknown boundary,
// including a static dependency without a complete imported summary. Such a
// callee may release the lock; missing evidence cannot establish it stayed held.
func transferOpaqueUnlocks(
	instruction ssa.Instruction,
	held []string,
	guards map[string]lockGuard,
	lockValues map[string][]ssa.Value,
	released map[string]bool,
	budget *ssaflow.SearchBudget,
) []string {
	common := ssaflow.InstructionCall(instruction)
	if _, _, _, direct := mutexAction(instruction); direct {
		return held
	}
	for _, identity := range slices.Clone(held) {
		if !budget.Spend() {
			return held
		}
		for _, value := range lockValues[identity] {
			if !budget.Spend() {
				return held
			}
			opaqueArgument := common != nil && opaqueCallee(common) && lockHandedTo(common, value)
			if !opaqueArgument && !handedUnlockCallback(instruction, value, budget) {
				continue
			}
			if opaqueArgument {
				released[identity] = true
			}
			// Callback retention supplies uncertainty, not a release witness.
			// Otherwise reacquiring a private completion gate would invent a
			// missing-release contract on the caller's final return.
			held = releaseLock(held, identity)
			delete(guards, identity)
			break
		}
	}
	return held
}

// handedUnlockCallback identifies a release capability handed to another owner,
// not an executed release. A receiver field or callback consumer may invoke it
// asynchronously; that makes the previous held-lock state unknown. A local
// callback merely created or saved in a local variable establishes no handoff.
// https://github.com/Control-D-Inc/ctrld/blob/37c33315591632c5f08df8062d1c77e07b3a465f/resolver_test.go#L348-L364
func handedUnlockCallback(instruction ssa.Instruction, lock ssa.Value, budget *ssaflow.SearchBudget) bool {
	var values []ssa.Value
	switch typed := instruction.(type) {
	case *ssa.Store:
		if _, field := typed.Addr.(*ssa.FieldAddr); !field {
			return false
		}
		values = []ssa.Value{typed.Val}
	case *ssa.Call:
		values = typed.Common().Args
	default:
		return false
	}
	return slices.ContainsFunc(values, func(value ssa.Value) bool {
		if !budget.Spend() {
			return false
		}
		if _, callback := value.Type().Underlying().(*types.Signature); !callback {
			return false
		}
		return lifecycle.ProveValueCallsMethodWithin(value, "Unlock", lock, budget).Proven() ||
			lifecycle.ProveValueCallsMethodWithin(value, "RUnlock", lock, budget).Proven()
	})
}

// opaqueCallee reports whether the call is dispatched at run time: an
// interface method, a function value, or a static call without a body. Complete
// imported mutex effects have already been consumed; a missing summary cannot
// establish that an unavailable helper leaves the lock held.
func opaqueCallee(common *ssa.CallCommon) bool {
	if common.IsInvoke() {
		return true
	}
	if _, ok := common.Value.(*ssa.Builtin); ok {
		return false
	}
	callee := common.StaticCallee()
	return callee == nil || len(callee.Blocks) == 0
}

// lockHandedTo reports whether an argument is the lock, its owner, or a
// value the lock derives from, such as the struct whose field it is.
func lockHandedTo(common *ssa.CallCommon, lock ssa.Value) bool {
	for _, argument := range common.Args {
		if heapmodel.MayAlias(argument, lock) || heapmodel.ValueDerivesFrom(lock, argument) {
			return true
		}
	}
	return false
}
