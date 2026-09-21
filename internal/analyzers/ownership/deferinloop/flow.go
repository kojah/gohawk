package deferinloop

import (
	"slices"

	"github.com/kojah/gohawk/internal/passes/lifecyclefacts"
	"github.com/kojah/gohawk/internal/ssaflow"
	"github.com/kojah/gohawk/internal/ssaflow/dense"

	"golang.org/x/tools/go/ssa"
)

type resourceStatus uint8

const (
	resourceLive resourceStatus = 1 << iota
	resourceSettled
	resourceUnknown
)

type deferFlowState struct {
	block  *ssa.BasicBlock
	index  int
	status resourceStatus
}

// resourceLiveAtNextIteration asks the narrow reporting question directly:
// can this exact acquired value reach a block dominating the defer's block
// while still definitely live? Returning is safe because Go runs the defer;
// settled and unknown paths stop at the backedge without producing a claim.
func resourceLiveAtNextIteration(
	evidence *lifecyclefacts.LifecycleEvidence,
	deferred *ssa.Defer,
	obligation deferObligation,
) bool {
	index := ssaflow.InstructionIndex(deferred)
	if index < 0 {
		return false
	}
	// The fact is a set of possible statuses, joined by union. Keeping live
	// separate from unknown preserves a proven live path when another path
	// crosses an opaque operation. Transfers distribute over that union.
	initial := []dense.State[*ssa.BasicBlock, resourceStatus]{{Point: deferred.Block(), Fact: resourceLive}}
	transfer := func(block *ssa.BasicBlock, statuses resourceStatus) []dense.State[*ssa.BasicBlock, resourceStatus] {
		if block == nil {
			return nil // The nil point collects states reaching an outer backedge.
		}
		start := 0
		if block == deferred.Block() {
			start = index + 1
		}
		return deferSuccessors(evidence, deferred, obligation, block, start, statuses)
	}
	// Each block and the backedge sink can gain at most three status bits.
	// That bounds the number of transfers even when inner loops do not exit.
	limit := 3 * (len(deferred.Parent().Blocks) + 1)
	result := dense.Forward(initial, func(left, right resourceStatus) resourceStatus { return left | right }, transfer, limit)
	return result.Complete && result.In[nil]&resourceLive != 0
}

func deferSuccessors(
	evidence *lifecyclefacts.LifecycleEvidence,
	deferred *ssa.Defer,
	obligation deferObligation,
	block *ssa.BasicBlock,
	start int,
	statuses resourceStatus,
) []dense.State[*ssa.BasicBlock, resourceStatus] {
	var successors []dense.State[*ssa.BasicBlock, resourceStatus]
	for _, status := range []resourceStatus{resourceLive, resourceSettled, resourceUnknown} {
		if statuses&status == 0 {
			continue
		}
		state := advanceDeferState(evidence, deferFlowState{block: block, index: start, status: status}, obligation)
		for _, successor := range block.Succs {
			fact := iteratorSuccessorStatus(state, successor, obligation)
			// A return to the defer's own block is a backedge too, so the
			// initial block is never re-entered with an incorrect start offset.
			if successor.Dominates(deferred.Block()) {
				successor = nil
			}
			successors = append(successors, dense.State[*ssa.BasicBlock, resourceStatus]{Point: successor, Fact: fact})
		}
	}
	return successors
}

// Without a general contract proving what Next does on exhaustion, its false
// branch leaves the lifetime unknown and suppresses a claim. The true branch
// stays live, so an early break can still reach the outer backedge and report.
func iteratorSuccessorStatus(
	state deferFlowState,
	successor *ssa.BasicBlock,
	obligation deferObligation,
) resourceStatus {
	if state.status != resourceLive || len(state.block.Succs) != 2 || successor != state.block.Succs[1] {
		return state.status
	}
	branch, ok := state.block.Instrs[len(state.block.Instrs)-1].(*ssa.If)
	if !ok {
		return state.status
	}
	iterator := conditionCall(state.block, branch.Cond)
	if iterator == nil || ssaflow.CallName(iterator.Common()) != "Next" ||
		!sameObligationValue(ssaflow.CallReceiver(iterator.Common()), obligation.target) {
		return state.status
	}
	return resourceUnknown
}

func conditionCall(block *ssa.BasicBlock, condition ssa.Value) *ssa.Call {
	for _, instruction := range block.Instrs {
		call, ok := instruction.(*ssa.Call)
		if ok && ssaflow.SameValue(condition, call) {
			return call
		}
	}
	return nil
}

// Each instruction advances a monotone three-state obligation. Once evidence
// settles the value, or an opaque boundary makes its lifetime unknowable,
// later instructions cannot restore the proof that it is definitely live.
func advanceDeferState(
	evidence *lifecyclefacts.LifecycleEvidence,
	state deferFlowState,
	obligation deferObligation,
) deferFlowState {
	for ; state.index < len(state.block.Instrs); state.index++ {
		if state.status != resourceLive {
			continue
		}
		switch classifyResourceInstruction(evidence, state.block.Instrs[state.index], obligation) {
		case resourceLive:
		case resourceSettled:
			state.status = resourceSettled
		case resourceUnknown:
			state.status = resourceUnknown
		}
	}
	return state
}

// Classification is deliberately asymmetric: explicit cleanup and proven
// transfer settle, opaque use suppresses, and ordinary operations on the
// receiver leave it live. Merely failing to find a cleanup proves nothing.
func classifyResourceInstruction(
	evidence *lifecyclefacts.LifecycleEvidence,
	instruction ssa.Instruction,
	obligation deferObligation,
) resourceStatus {
	if transfersResource(evidence, instruction, obligation.target) {
		return resourceSettled
	}
	common := ssaflow.InstructionCall(instruction)
	if common == nil {
		if opaqueResourceUse(instruction, obligation.target) {
			return resourceUnknown
		}
		return resourceLive
	}
	receiver := ssaflow.CallReceiver(common)
	if sameObligationValue(receiver, obligation.target) {
		if slices.Contains(obligation.cleanup, ssaflow.CallName(common)) {
			return resourceSettled
		}
		return resourceLive
	}
	return resourceUseStatus(evidence, instruction, obligation.target)
}

// Transfers reuse the shared structural ownership vocabulary, including
// imported facts for callees that retain the exact resource beyond the call.
func transfersResource(
	evidence *lifecyclefacts.LifecycleEvidence,
	instruction ssa.Instruction,
	target ssa.Value,
) bool {
	if ssaflow.StoresValueInGlobal(instruction, target) ||
		ssaflow.StoresValueInEscapingField(instruction, target) ||
		ssaflow.SendsValue(instruction, target) ||
		ssaflow.CallTransfersArgumentToReturnedOwner(instruction, target) ||
		ssaflow.CallTransfersArgumentToReceiver(instruction, target) ||
		ssaflow.CallTransfersArgumentToLifecycleOwner(instruction, target) {
		return true
	}
	return evidence.ArgumentRetainedByCallee(instruction, target)
}

// A source-visible callee summarized as neither releasing nor retaining the
// argument is an ordinary use. An unreadable callee that receives the value
// makes the lifetime unknown, preserving the precision-first reporting rule.
func resourceUseStatus(
	evidence *lifecyclefacts.LifecycleEvidence,
	instruction ssa.Instruction,
	target ssa.Value,
) resourceStatus {
	common := ssaflow.InstructionCall(instruction)
	if common == nil {
		return resourceLive
	}
	used := false
	for index, argument := range common.Args {
		if !ssaflow.SameValue(argument, target) && !ssaflow.ValueContainsValue(argument, target) {
			continue
		}
		used = true
		if released, summarized := evidence.CalleeClaims(instruction, index, lifecyclefacts.ClaimReleases); summarized {
			if released {
				return resourceSettled
			}
		}
	}
	if used && !evidence.CalleeSummarized(instruction) {
		return resourceUnknown
	}
	return resourceLive
}

// Capturing the resource in a closure is opaque here: whether the closure
// runs, escapes, or releases the resource is outside this loop-local proof.
func opaqueResourceUse(instruction ssa.Instruction, target ssa.Value) bool {
	closure, ok := instruction.(*ssa.MakeClosure)
	if !ok {
		return false
	}
	for _, binding := range closure.Bindings {
		if ssaflow.SameValue(binding, target) || ssaflow.ValueContainsValue(binding, target) {
			return true
		}
	}
	return false
}
