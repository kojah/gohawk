package lockorder

import (
	"github.com/kojah/gohawk/internal/heapmodel"
	"github.com/kojah/gohawk/internal/ssaflow"
	"golang.org/x/tools/go/ssa"
)

// Possible writer witnesses are selected from completed setup and checked at
// each mutation under the function allowance. They supply uncertainty only:
// neither a held-lock state nor an ordering edge can be inferred from them.
// Alias/type/graph internals retain their separately owned queries.

// One possible writer leaves the mutation uncertain. Interrupted witness
// selection is also unknown; it cannot impersonate an absent writer guard.
func provePossibleWriterAt(writers []*ssa.Defer, instruction ssa.Instruction, calls []*ssa.Call, budget *ssaflow.SearchBudget) ssaflow.Proof {
	if budget.Exhausted() || budget.PoolExhausted() {
		return ssaflow.Proof{Reason: ssaflow.EvidenceBudgetExhausted}
	}
	for _, deferred := range writers {
		if !budget.Spend() {
			return ssaflow.Proof{Reason: ssaflow.EvidenceBudgetExhausted}
		}
		proof := possibleWriterAt(deferred, instruction, calls, budget)
		if !proof.Known() || proof.Proven() {
			return proof
		}
	}
	return ssaflow.Proof{State: ssaflow.EvidenceDisproven, Reason: ssaflow.EvidenceNotFound}
}

// The caller supplies the completed setup census. Rechecking temporal and alias
// evidence here shares the request allowance and must not rediscover the body.
func possibleWriterAt(deferred *ssa.Defer, instruction ssa.Instruction, calls []*ssa.Call, budget *ssaflow.SearchBudget) ssaflow.Proof {
	unknown := ssaflow.Proof{Reason: ssaflow.EvidenceBudgetExhausted}
	absent := ssaflow.Proof{State: ssaflow.EvidenceDisproven, Reason: ssaflow.EvidenceNotFound}
	dominates := ssaflow.InstructionDominatesWithin(deferred, instruction, budget)
	if budget.Exhausted() || budget.PoolExhausted() {
		return unknown
	}
	if !dominates {
		return absent
	}
	_, _, writer, _ := mutexAction(deferred)
	for _, call := range calls {
		if !budget.Spend() {
			return unknown
		}
		operation, _, receiver, direct := mutexAction(call)
		if !direct || operation != mutexRelease || readModeRelease(call) || !heapmodel.MayAlias(receiver, writer) {
			continue
		}
		afterDefer := ssaflow.InstructionMayFollowWithin(deferred, call, budget)
		beforeWrite := afterDefer && ssaflow.InstructionMayFollowWithin(call, instruction, budget)
		if budget.Exhausted() || budget.PoolExhausted() {
			return unknown
		}
		if beforeWrite {
			// An explicit intervening release defeats the possible-held guard;
			// the still-registered defer must not hide an unprotected write.
			return absent
		}
	}
	return ssaflow.Proof{State: ssaflow.EvidenceProven, Reason: ssaflow.EvidenceStructuralWalk}
}

// An imported wrapper can acquire its embedded mutex while doing bookkeeping
// that the complete-effect summary cannot model. A deferred standard exclusive
// Unlock of that same wrapper is positive evidence of a possibly held writer.
// This is uncertainty, not guard-to-field inference or an acquisition effect;
// it must never enter order or recursive-lock proofs. Distinct wrapper receivers,
// known empty calls, and a release already executed provide no such evidence.
// https://github.com/rfjakob/gocryptfs/blob/842af4463989ee6808d397433e9aba8517e49c89/internal/fusefrontend/file.go#L418-L430
func (setup *lockFunctionSetup) deferredWriterWitnesses(budget *ssaflow.SearchBudget) []*ssa.Defer {
	var writers []*ssa.Defer
	for _, deferred := range setup.defers {
		if !budget.Spend() {
			return nil
		}
		effect, direct := setup.direct[deferred]
		operation, receiver := effect.operation, effect.receiver
		if !direct || operation != mutexRelease || readModeRelease(deferred) {
			continue
		}
		field, embedded := receiver.(*ssa.FieldAddr)
		if !embedded {
			continue
		}
		for _, call := range setup.calls {
			if !budget.Spend() {
				return nil
			}
			callee := call.Common().StaticCallee()
			if callee == nil || len(callee.Blocks) != 0 {
				continue
			}
			dominates := ssaflow.InstructionDominatesWithin(call, deferred, budget)
			if budget.Exhausted() {
				return nil
			}
			if !dominates {
				continue
			}
			if _, complete := setup.summaries[call]; complete {
				continue
			}
			// Only the embedded lock's exact wrapper may explain this deferred
			// writer release. Aliasing supplies an unknown witness, never a held
			// lock or ordering edge; interrupted setup discards all witnesses.
			calledReceiver := ssaflow.CallReceiver(call.Common())
			if calledReceiver != nil && heapmodel.MayAlias(calledReceiver, field.X) {
				writers = append(writers, deferred)
				break
			}
		}
	}
	return writers
}
