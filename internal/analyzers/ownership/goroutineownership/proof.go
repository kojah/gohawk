package goroutineownership

import (
	"github.com/kojah/gohawk/internal/check"
	"github.com/kojah/gohawk/internal/ssaflow"

	"golang.org/x/tools/go/ssa"
)

// GoroutineOutcome distinguishes proven lifecycle behavior from an opaque
// handoff. Unknown evidence suppresses correctness diagnostics.
type GoroutineOutcome uint8

const (
	GoroutineUnknown GoroutineOutcome = iota
	GoroutineLifecycleHonored
	GoroutineLifecycleViolated
	GoroutineTransferred
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
	var evidence ssaflow.LocalEvidence
	ownership := newGoroutineAnalysis(
		nil,
		spawn.Parent(),
		spawn,
		goroutineOwnershipConfig{mode: goroutineModeContext},
		&evidence,
	)
	ownership.testFunction = true
	proof := ownership.prove()
	return proof.Outcome == GoroutineLifecycleHonored || proof.Outcome == GoroutineTransferred ||
		proof.Outcome == GoroutineUnknown && proof.Reason != ownershipReasonDetachedUnknown
}

func (analysis goroutineAnalysis) prove() GoroutineProof {
	if proof := analysis.immediateProof(); proof.Outcome != GoroutineUnknown || proof.Reason != "" {
		return proof
	}
	leaks := ssaflow.UnownedReturn(analysis.spawn, analysis.instructionOwnsGoroutine, analysis.returnOwnsGoroutine)
	if !leaks {
		return GoroutineProof{Outcome: GoroutineLifecycleHonored, Reason: ownershipReasonJoinProven}
	}
	// A method receiver or lifecycle-shaped value may own completion outside
	// this function. Even when the worker also exposes a channel, that is not
	// enough to prove the caller must receive it. Vekil's websocket session
	// closes through its receiver while a test observes the socket protocol:
	// https://github.com/sozercan/vekil/blob/842f12f7875143274378fcbb80d411295edf3d28/proxy/responses_websocket_test.go#L5325-L5356
	if analysis.config.mode != goroutineModeJoin && analysis.checkID == check.GoroutineJoin &&
		(len(analysis.owners) > 0 || ssaflow.CallReceiver(analysis.spawn.Common()) != nil) ||
		localBufferedCompletionSignal(analysis.function, analysis.signals) ||
		analysis.config.mode == goroutineModeContext && analysis.config.acceptContextLifecycle &&
			goroutineHasContextLifecycle(analysis.spawn) ||
		matchingCountedJoin(analysis.function, analysis.spawn, analysis.signals) ||
		nestedCallbackReceivesAny(analysis.function, analysis.signals) || analysis.hasAmbiguousPreSpawnOwnershipUse() ||
		analysis.checkID == check.GoroutineDetached && analysis.hasAmbiguousPostSpawnOwnershipUse() {
		return GoroutineProof{Outcome: GoroutineUnknown, Reason: ownershipReasonOpaqueTransfer}
	}
	if !ssaflow.UnownedReturn(analysis.spawn, analysis.instructionOwnsOrAmbiguouslyTransfers, analysis.returnMayOwnGoroutine) {
		return GoroutineProof{Outcome: GoroutineUnknown, Reason: ownershipReasonOpaqueTransfer}
	}
	if analysis.checkID == check.GoroutineDetached {
		return GoroutineProof{Outcome: GoroutineUnknown, Reason: ownershipReasonDetachedUnknown}
	}
	if analysis.unsettledDone != nil {
		return GoroutineProof{Outcome: GoroutineLifecycleViolated, Reason: ownershipReasonDoneBeforeCompletion}
	}
	return GoroutineProof{Outcome: GoroutineLifecycleViolated, Reason: ownershipReasonUnownedReturn}
}

func (analysis goroutineAnalysis) hasAmbiguousPostSpawnOwnershipUse() bool {
	for _, block := range analysis.function.Blocks {
		for _, instruction := range block.Instrs {
			if ssaflow.InstructionMayFollow(analysis.spawn, instruction) && ambiguouslyTransfersGoroutineOwnership(
				analysis.evidence, instruction, analysis.signals, analysis.groups, ownershipCandidates(analysis.config, analysis.owners),
			) {
				return true
			}
		}
	}
	return false
}

func (analysis goroutineAnalysis) hasAmbiguousPreSpawnOwnershipUse() bool {
	for _, block := range analysis.function.Blocks {
		for _, instruction := range block.Instrs {
			if ssaflow.InstructionDominates(instruction, analysis.spawn) && analysis.preSpawnMayOwnGoroutine(instruction) {
				return true
			}
		}
	}
	return false
}

func (analysis goroutineAnalysis) preSpawnMayOwnGoroutine(instruction ssa.Instruction) bool {
	switch instruction.(type) {
	case *ssa.Defer, *ssa.MapUpdate, *ssa.Send:
		return analysis.instructionMayOwnGoroutine(instruction)
	}
	common := ssaflow.InstructionCall(instruction)
	if common == nil {
		return false
	}
	callee := common.StaticCallee()
	return (callee == nil || callee.Syntax() == nil || len(callee.Blocks) == 0) && ambiguouslyTransfersGoroutineOwnership(
		analysis.evidence, instruction, analysis.signals, nil, nil,
	)
}
