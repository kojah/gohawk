package goroutineownership

import (
	"go/token"

	"github.com/kojah/gohawk/internal/check"
	"github.com/kojah/gohawk/internal/ssaflow"

	"golang.org/x/tools/go/ssa"
)

// This file owns the single decision path for every goroutineownership check.
// The proof reverses the burden of evidence: the worker must first establish an
// obligation (a completion signal, a settling WaitGroup, or, for the detached
// audit, a lifecycle owner), and the default diagnostic then needs a feasible
// return path on which nothing joins, transfers, or ambiguously consumes it.
// Every instruction after the spawn is classified once; the flow query asks
// only whether an exact action, or any action at all, covers every return.

// GoroutineOutcome distinguishes proven lifecycle behavior from an opaque
// handoff. Unknown evidence suppresses correctness diagnostics.
type GoroutineOutcome uint8

const (
	GoroutineUnknown GoroutineOutcome = iota
	GoroutineLifecycleHonored
	GoroutineLifecycleViolated
	GoroutineTransferred
)

type goroutineOwnershipReason string

const (
	reasonJoinProven              goroutineOwnershipReason = "join-proven"
	reasonDeferredJoinBeforeSpawn goroutineOwnershipReason = "deferred-join-before-spawn"
	reasonGuardedLocalJoin        goroutineOwnershipReason = "guarded-local-join"
	reasonStopLifecycle           goroutineOwnershipReason = "stop-lifecycle"
	reasonContextLifecycle        goroutineOwnershipReason = "context-lifecycle"
	reasonSynctestBubbleOwner     goroutineOwnershipReason = "synctest-bubble-owner"
	reasonCallerOrExternalOwner   goroutineOwnershipReason = "caller-or-external-owner"
	reasonOwnershipTransfer       goroutineOwnershipReason = "ownership-transfer"
	reasonOpaqueTransfer          goroutineOwnershipReason = "opaque-ownership-transfer"
	reasonLoopJoinUnproven        goroutineOwnershipReason = "loop-join-unproven"
	reasonBufferedSignal          goroutineOwnershipReason = "buffered-completion-signal"
	reasonDetachedUnknown         goroutineOwnershipReason = "detached-lifecycle-unknown"
	reasonUnownedReturn           goroutineOwnershipReason = "unowned-return"
	reasonDoneBeforeCompletion    goroutineOwnershipReason = "waitgroup-done-before-completion"
)

// GoroutineProof is the single result consumed by reporting, tracing, and
// cross-analyzer ownership queries.
type GoroutineProof struct {
	Outcome GoroutineOutcome
	Reason  goroutineOwnershipReason
}

// GoroutineOwnershipMayBeHandledInTest conservatively reports whether an exact
// proof or an opaque handoff may own spawn independently of context
// cancellation. Unknown handoffs suppress testlifecycle: they are not positive
// ownership evidence, but neither analyzer can prove a detached test worker.
func GoroutineOwnershipMayBeHandledInTest(spawn *ssa.Go) bool {
	if spawn == nil || spawn.Parent() == nil {
		return false
	}
	proof := newSpawnAnalysis(spawn.Parent(), spawn, goroutineOwnershipConfig{mode: goroutineModeContext}).prove()
	return proof.Outcome == GoroutineLifecycleHonored || proof.Outcome == GoroutineTransferred ||
		proof.Outcome == GoroutineUnknown && proof.Reason != reasonDetachedUnknown
}

func (analysis *spawnAnalysis) prove() GoroutineProof {
	if proof, decided := analysis.lifecycleProof(); decided {
		return proof
	}
	if proof, decided := analysis.dominatingProof(); decided {
		return proof
	}
	exact := func(instruction ssa.Instruction) bool {
		action := analysis.action(instruction)
		return action == actionJoin || action == actionTransfer
	}
	if !ssaflow.UnownedReturn(analysis.spawn, exact, analysis.returnTransfers) {
		return GoroutineProof{Outcome: GoroutineLifecycleHonored, Reason: reasonJoinProven}
	}
	if analysis.guardedLocalJoin(exact) {
		return GoroutineProof{Outcome: GoroutineLifecycleHonored, Reason: reasonGuardedLocalJoin}
	}
	any := func(instruction ssa.Instruction) bool {
		return analysis.action(instruction) != actionNone
	}
	if !ssaflow.UnownedReturn(analysis.spawn, any, analysis.returnTransfers) {
		return GoroutineProof{Outcome: GoroutineUnknown, Reason: reasonOpaqueTransfer}
	}
	if analysis.joinInsideLoop() {
		// A receive loop that may run zero times is not a proven skip: the
		// spawn loop may have run zero times too. Matching those counts is not
		// modeled, so counted joins stay unknown rather than reported.
		return GoroutineProof{Outcome: GoroutineUnknown, Reason: reasonLoopJoinUnproven}
	}
	if analysis.checkID == check.GoroutineDetached {
		return GoroutineProof{Outcome: GoroutineUnknown, Reason: reasonDetachedUnknown}
	}
	if analysis.bufferedSignals() {
		// A buffered completion send lets the worker finish after the caller
		// stops receiving, so it does not by itself establish a join protocol.
		// Buildkite uses a one-slot result channel to let its collector finish:
		// https://github.com/buildkite/agent/blob/e206ddf806af50a1ba8c9a6dd501dfda0b730818/internal/artifact/downloader.go#L96-L177
		return GoroutineProof{Outcome: GoroutineUnknown, Reason: reasonBufferedSignal}
	}
	if analysis.unsettledDone != nil && len(analysis.groups) == 0 && len(analysis.signals) == 0 {
		return GoroutineProof{Outcome: GoroutineLifecycleViolated, Reason: reasonDoneBeforeCompletion}
	}
	return GoroutineProof{Outcome: GoroutineLifecycleViolated, Reason: reasonUnownedReturn}
}

func (analysis *spawnAnalysis) joinInsideLoop() bool {
	for _, block := range analysis.function.Blocks {
		if !ssaflow.BlockInCycle(block) {
			continue
		}
		for _, instruction := range block.Instrs {
			if ssaflow.InstructionMayFollow(analysis.spawn, instruction) && analysis.action(instruction) == actionJoin {
				return true
			}
		}
	}
	return false
}

// lifecycleProof settles workers whose completion is owned outside the
// spawning function before any local flow is consulted.
func (analysis *spawnAnalysis) lifecycleProof() (GoroutineProof, bool) {
	if analysis.config.mode == goroutineModeContext {
		if goroutineReceivesCallerSignal(analysis.spawn) {
			return GoroutineProof{Outcome: GoroutineLifecycleHonored, Reason: reasonStopLifecycle}, true
		}
		if goroutineReceivesCallerContext(analysis.spawn) {
			return GoroutineProof{Outcome: GoroutineLifecycleHonored, Reason: reasonContextLifecycle}, true
		}
	}
	// A goroutine that completes through a caller-owned channel or wait group
	// transfers its join obligation across the call boundary.
	for _, tracked := range analysis.tracked {
		if tracked.kind != trackedOwner && ssaflow.ExternallyOwnedValue(tracked.value) {
			return GoroutineProof{Outcome: GoroutineTransferred, Reason: reasonCallerOrExternalOwner}, true
		}
	}
	if synctestOwnsGoroutine(analysis.function) {
		return GoroutineProof{Outcome: GoroutineLifecycleHonored, Reason: reasonSynctestBubbleOwner}, true
	}
	return GoroutineProof{}, false
}

// dominatingProof classifies the instructions that run before every spawn. A
// deferred join registered on every path to the spawn runs after the worker
// settles, as in Zap's pool race test:
// https://github.com/uber-go/zap/blob/bb1a55dd13257cf7cbd06b4146674c67ca614dea/internal/pool/pool_test.go#L85-L105
// A transfer or opaque handoff before the spawn likewise settles the
// obligation before this function could observe completion.
func (analysis *spawnAnalysis) dominatingProof() (GoroutineProof, bool) {
	unknown := false
	for _, block := range analysis.function.Blocks {
		if !block.Dominates(analysis.spawn.Block()) {
			continue
		}
		for _, instruction := range block.Instrs {
			if !ssaflow.InstructionDominates(instruction, analysis.spawn) || instruction == analysis.spawn {
				continue
			}
			_, deferred := instruction.(*ssa.Defer)
			switch analysis.action(instruction) {
			case actionJoin:
				if deferred {
					return GoroutineProof{Outcome: GoroutineLifecycleHonored, Reason: reasonDeferredJoinBeforeSpawn}, true
				}
			case actionTransfer:
				return GoroutineProof{Outcome: GoroutineTransferred, Reason: reasonOwnershipTransfer}, true
			case actionUnknown:
				unknown = true
			case actionNone:
			}
		}
	}
	if unknown {
		return GoroutineProof{Outcome: GoroutineUnknown, Reason: reasonOpaqueTransfer}, true
	}
	return GoroutineProof{}, false
}

// guardedLocalJoin re-runs the exact flow query with the fact that a channel
// created once before the spawn is non-nil afterwards. An optional worker is
// commonly stopped and waited beneath `if stop != nil`; the launch proves that
// guard true on every path that reaches it. Rainier:
// https://github.com/tokencanopy/rainier/blob/855b2e7c276a60a2f65f141d1071cf03be38d6e3/internal/attachio/attachio.go#L267-L287
func (analysis *spawnAnalysis) guardedLocalJoin(exact func(ssa.Instruction) bool) bool {
	if len(analysis.signals)+len(analysis.groups) == 0 {
		return false
	}
	for _, created := range analysis.channelsCreatedOnceBeforeSpawn() {
		if !ssaflow.UnownedReturnAssumingNonNil(analysis.spawn, created, exact, analysis.returnTransfers) {
			return true
		}
	}
	return false
}

// channelsCreatedOnceBeforeSpawn returns captured channel locals whose only
// store is one MakeChan that dominates the spawn outside any loop. Any other
// use of the captured address, or a store that can execute repeatedly, means
// the guard may observe a different channel instance than the worker.
func (analysis *spawnAnalysis) channelsCreatedOnceBeforeSpawn() []ssa.Value {
	closure, ok := analysis.spawn.Common().Value.(*ssa.MakeClosure)
	if !ok || ssaflow.BlockInCycle(analysis.spawn.Block()) {
		return nil
	}
	var created []ssa.Value
	for _, binding := range closure.Bindings {
		if channel := singleDominatingChannelStore(analysis.function, analysis.spawn, closure, binding); channel != nil {
			created = append(created, channel)
		}
	}
	return created
}

func singleDominatingChannelStore(
	parent *ssa.Function,
	spawn *ssa.Go,
	closure *ssa.MakeClosure,
	binding ssa.Value,
) ssa.Value { //nolint:ireturn // Preserve the concrete channel value.
	if binding == nil || binding.Referrers() == nil {
		return nil
	}
	var stored ssa.Value
	for _, reference := range *binding.Referrers() {
		switch typed := reference.(type) {
		case *ssa.DebugRef:
			continue
		case *ssa.UnOp:
			if typed.Op == token.MUL && typed.X == binding {
				continue
			}
		case *ssa.MakeClosure:
			if typed == closure {
				continue
			}
		case *ssa.Store:
			channel, ok := typed.Val.(*ssa.MakeChan)
			if ok && typed.Addr == binding && channel.Parent() == parent && stored == nil &&
				ssaflow.InstructionDominates(typed, spawn) && !ssaflow.BlockInCycle(typed.Block()) {
				stored = channel
				continue
			}
		}
		return nil
	}
	return stored
}
