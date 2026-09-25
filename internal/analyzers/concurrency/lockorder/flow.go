package lockorder

import (
	"go/token"
	"go/types"
	"maps"
	"slices"

	"github.com/kojah/gohawk/internal/heapmodel"
	"github.com/kojah/gohawk/internal/lifecycle"
	"github.com/kojah/gohawk/internal/ssaflow"
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
	pass            *analysis.Pass
	function        *ssa.Function
	exclusive       *exclusiveCallers
	evidence        *lifecycle.LocalEvidence
	relations       *lockOrders
	calleeLocks     *calleeLockSearch
	lockValues      map[string][]ssa.Value
	acquiredAt      map[string]token.Pos
	released        map[string]bool
	acquisitions    map[string][]ssa.Instruction
	uncertainGuards map[string]bool
	// unprovenRelease marks locks a callee may release without proving it does
	// so on every path. The lock stays held, because a return that leaves it
	// held is still worth reporting, but it is no longer proven held, which is
	// what a recursive acquisition has to claim.
	unprovenRelease  map[string]bool
	callerOwned      map[string]bool
	defers           []*ssa.Defer
	recursiveReports map[lockReportSite]bool
}

type lockReportSite struct {
	instruction ssa.Instruction
	identity    string
}

func walkLockOrderBounded(
	pass *analysis.Pass,
	function *ssa.Function,
	relations *lockOrders,
	calleeLocks *calleeLockSearch,
	evidence *lifecycle.LocalEvidence,
	callers map[*ssa.Function]conditionalCallerSet,
	exclusive *exclusiveCallers,
	summaries map[ssa.Instruction][]mutexEffect,
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
	heldAtReturn := map[string]map[*ssa.Return]bool{}
	acquisitions := map[string][]ssa.Instruction{}
	uncertainGuards := map[string]bool{}
	possibleWriters := possibleDeferredWriters(function, summaries)
	callerOwned := callerOwnedLocks(function, summaries)
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
		unprovenRelease:  map[string]bool{},
		callerOwned:      callerOwned,
		defers:           functionDefers,
		recursiveReports: make(map[lockReportSite]bool),
	}
	// Each predecessor selects its own phi values before any instruction runs.
	// Clone the lock collections so one successor's release cannot discharge
	// another successor's obligation; the immutable branch facts travel with it.
	remaining := 4096
	terminates := summaryKnowledge.Provider(pass).Terminates()
	ssaflow.WalkStates([]lockFlowState{{block: function.Blocks[0]}}, lockStateKey, func(state lockFlowState) ([]lockFlowState, bool) {
		remaining--
		if remaining < 0 {
			return nil, false
		}
		state.constants = lockPhiConstants(state)
		held := slices.Clone(state.held)
		readHeld := slices.Clone(state.readHeld)
		deferred := slices.Clone(state.deferred)
		guards := cloneLockGuards(state.guards)
		origins := maps.Clone(state.origins)
		if origins == nil {
			origins = map[string]lockAcquisition{}
		}
		condition := state.condition
		if len(state.block.Preds) > 1 {
			condition = ""
		}
		for _, instruction := range state.block.Instrs {
			// A call that never returns, whether os.Exit or a project's own
			// fatal wrapper the summaries prove, ends this path: no return
			// with the lock held follows it.
			if ssaflow.InstructionTerminatesWith(instruction, terminates) {
				return nil, true
			}
			recordUnreleasedLocks(instruction, held, deferred, lockValues, unreleasedReturns, heldAtReturn)
			// A complete call sequence replaces the fallback release search:
			// releasing and reacquiring within one helper must leave the lock
			// held, not erase it as a may-release witness would.
			if effects, complete := summaries[instruction]; complete {
				actionState := lockFlowState{
					held: held, readHeld: readHeld, deferred: deferred, guards: guards,
					origins: origins, condition: condition, conditionValue: state.conditionValue,
				}
				for _, effect := range effects {
					actionState = flow.applyMutexAction(instruction, effect, actionState)
				}
				held, readHeld, deferred, guards = actionState.held, actionState.readHeld, actionState.deferred, actionState.guards
				continue
			}
			held = transferCalledUnlocks(evidence, instruction, held, guards, lockValues, released, flow.unprovenRelease)
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
			effect, ok := directMutexEffect(instruction)
			if !ok {
				flow.recordCalledOrder(instruction, held, origins)
				reportReadLockWrites(pass, instruction, held, readHeld, lockValues, possibleWriters)
				continue
			}
			actionState := lockFlowState{
				held: held, readHeld: readHeld, deferred: deferred, guards: guards, origins: origins,
				condition: condition, conditionValue: state.conditionValue,
			}
			actionState = flow.applyMutexAction(instruction, effect, actionState)
			held, readHeld, guards = actionState.held, actionState.readHeld, actionState.guards
			deferred = actionState.deferred
		}
		return lockSuccessorStates(pass, state, held, readHeld, deferred, guards, origins), true
	})
	if remaining < 0 {
		return false
	}
	flow.reportMissingReleases(function, unreleasedReturns, heldAtReturn, callers[function])
	return true
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
	result := lifecycle.ReturnedResult(returned, len(returned.Results)-1)
	if types.Identical(last, types.Universe.Lookup("error").Type()) {
		return ssaflow.DefinitelyNil(result) || nilGuardDominatesReturn(result, returned)
	}
	if basic, ok := last.Underlying().(*types.Basic); ok && basic.Kind() == types.Bool {
		return !constantFalse(result)
	}
	return true
}

func possiblyDeferredUnlock(
	evidence *lifecycle.LocalEvidence,
	acquisition ssa.Instruction,
	functionDefers []*ssa.Defer,
	values []ssa.Value,
) bool {
	for _, deferred := range functionDefers {
		if !ssaflow.InstructionDominates(deferred, acquisition) {
			continue
		}
		for _, value := range values {
			proof := evidence.Completion(lifecycle.CompletionRequest{
				Instruction: deferred,
				Target:      value,
				Methods:     []string{"Unlock", "RUnlock"},
				Coverage:    lifecycle.CoverageAnywhere,
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
	evidence *lifecycle.LocalEvidence,
	instruction ssa.Instruction,
	held []string,
	guards map[string]lockGuard,
	lockValues map[string][]ssa.Value,
	released map[string]bool,
	unproven map[string]bool,
) []string {
	for _, identity := range slices.Clone(held) {
		for _, value := range lockValues[identity] {
			proof := evidence.Completion(lifecycle.CompletionRequest{
				Instruction: instruction,
				Target:      value,
				Methods:     []string{"Unlock", "RUnlock"},
				Budget:      ssaflow.NewSearchBudget(lockCompletionBudget),
			})
			if !releaseSettled(proof, ssaflow.EvidenceCalledCompletion) {
				// A callee that releases the lock on some paths and not others
				// leaves it held but no longer proven held. A return that still
				// holds it stays reportable, deliberately, so a conditional
				// handoff cannot hide a leak. A later acquisition of it must
				// not be called recursive, because that claim needs positive
				// evidence the lock IS held. vekil wraps its mutex in a type
				// whose Unlock returns early on a nil receiver, which left a
				// Lock and Unlock paired inside a loop body reported as a
				// recursive acquisition on the next iteration:
				// https://github.com/sozercan/vekil/blob/842f12f7875143274378fcbb80d411295edf3d28/proxy/route_executor.go#L697
				if mayRelease(evidence, instruction, value) {
					unproven[identity] = true
				}
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
	evidence *lifecycle.LocalEvidence,
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
			proof := evidence.Completion(lifecycle.CompletionRequest{
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
	evidence *lifecycle.LocalEvidence,
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
			proof := evidence.Completion(lifecycle.CompletionRequest{
				Instruction: instruction,
				Target:      value,
				Methods:     []string{"Unlock", "RUnlock"},
				Coverage:    lifecycle.CoverageAnywhere,
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
		return state
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
	possibleRelease := flow.unprovenRelease[identity]
	if slices.Contains(state.held, identity) {
		proof := flow.loadedLoopRelease(instruction, receiver, state.origins[identity].position)
		possibleRelease = possibleRelease || proof.possible
	}
	state.held = flow.acquireLock(instruction, state.held, identity, possibleRelease, acquired.variant)
	return state
}

// mayRelease reports whether the call releases the lock on at least one path.
// It is the weaker companion to the proof transferCalledUnlocks requires, and
// answers only whether the caller may still claim the lock is held.
func mayRelease(evidence *lifecycle.LocalEvidence, instruction ssa.Instruction, value ssa.Value) bool {
	proof := evidence.Completion(lifecycle.CompletionRequest{
		Instruction: instruction,
		Target:      value,
		Methods:     []string{"Unlock", "RUnlock"},
		Coverage:    lifecycle.CoverageAnywhere,
		Budget:      ssaflow.NewSearchBudget(lockCompletionBudget),
	})
	return proof.Proven() && proof.Reason == ssaflow.EvidenceCalledCompletion
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
) []string {
	common := ssaflow.InstructionCall(instruction)
	if _, _, _, direct := mutexAction(instruction); direct {
		return held
	}
	for _, identity := range slices.Clone(held) {
		for _, value := range lockValues[identity] {
			opaqueArgument := common != nil && opaqueCallee(common) && lockHandedTo(common, value)
			if !opaqueArgument && !handedUnlockCallback(instruction, value) {
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
func handedUnlockCallback(instruction ssa.Instruction, lock ssa.Value) bool {
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
		if _, callback := value.Type().Underlying().(*types.Signature); !callback {
			return false
		}
		return lifecycle.ValueCallsMethod(value, "Unlock", lock) || lifecycle.ValueCallsMethod(value, "RUnlock", lock)
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
		if heapmodel.MayAlias(argument, lock) || heapmodel.ValueDerivesFrom(lock, argument, map[ssa.Value]bool{}) {
			return true
		}
	}
	return false
}
