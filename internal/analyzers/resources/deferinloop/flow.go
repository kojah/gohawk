package deferinloop

import (
	"go/types"
	"slices"
	"strconv"

	"github.com/kojah/gohawk/internal/passes/lifecyclefacts"
	"github.com/kojah/gohawk/internal/ssaflow"
	"github.com/kojah/gohawk/internal/summaries"
	analysisTrace "github.com/kojah/gohawk/internal/trace"

	"golang.org/x/tools/go/ssa"
)

type resourceStatus uint8

const (
	resourceLive resourceStatus = iota
	resourceSettled
	resourceUnknown
)

type deferFlowState struct {
	block       *ssa.BasicBlock
	predecessor *ssa.BasicBlock
	index       int
	status      resourceStatus
}

// resourceLiveAtNextIteration asks the narrow reporting question directly:
// can this exact acquired value reach a block dominating the defer's block
// while still definitely live? Returning is safe because Go runs the defer;
// settled and unknown paths stop at the backedge without producing a claim.
func resourceLiveAtNextIteration(
	evidence *lifecyclefacts.LifecycleEvidence,
	knowledge *summaries.Provider,
	probe analysisTrace.Probe,
	deferred *ssa.Defer,
	obligation deferObligation,
) bool {
	index := ssaflow.InstructionIndex(deferred)
	if index < 0 {
		return false
	}
	// A resource retained before its defer is no more iteration-local than one
	// retained afterward. In particular append lowers to an indexed store
	// before the append call; the containing collection may be consumed later.
	// https://github.com/protomaps/go-pmtiles/blob/a3e4951ea6a0477b784c27c1dcbfd9c130878c5a/pmtiles/merge.go#L206-L215
	for _, store := range ssaflow.InstructionsOf[*ssa.Store](deferred.Parent()) {
		if ssaflow.InstructionDominates(store, deferred) && opaqueResourceUse(store, obligation.target) {
			probe.Decision(analysisTrace.Step{Reason: "retained-before-defer", Outcome: analysisTrace.OutcomeUnknown, Pos: store.Pos()})
			return false
		}
	}
	liveAtBackedge := false
	initial := []deferFlowState{{block: deferred.Block(), index: index + 1, status: resourceLive}}
	ssaflow.WalkStates(initial, func(state deferFlowState) deferFlowState { return state }, func(state deferFlowState) ([]deferFlowState, bool) {
		state = advanceDeferState(evidence, probe, state, obligation)
		// A branch a callee's proven result rules out is not a path to the
		// backedge; feasibility only removes successors, it never adds one.
		feasible := knowledge.FeasibleSuccessors(state.block, state.predecessor, ssaflow.NewSearchBudget(ssaflow.SummaryBudget))
		successors := make([]deferFlowState, 0, len(feasible))
		for _, successor := range feasible {
			status := iteratorSuccessorStatus(state, successor, obligation)
			if successor.Dominates(deferred.Block()) {
				if status == resourceLive {
					liveAtBackedge = true
					probe.Decision(analysisTrace.Step{
						Reason: "live-at-backedge", Outcome: analysisTrace.OutcomeRejected, Pos: deferred.Pos(),
						Details: map[string]string{"block": strconv.Itoa(state.block.Index)},
					})
					return nil, false
				}
				if status != state.status {
					probe.Evidence(analysisTrace.Step{
						Reason: "iterator-exhausted", Outcome: analysisTrace.OutcomeUnknown, Pos: deferred.Pos(),
						Details: map[string]string{"block": strconv.Itoa(state.block.Index)},
					})
				}
				continue
			}
			successors = append(successors, deferFlowState{
				block: successor, predecessor: state.block, status: status,
			})
		}
		return successors, true
	})
	if !liveAtBackedge {
		probe.Decision(analysisTrace.Step{Reason: "settled-or-unknown-before-backedge", Outcome: analysisTrace.OutcomeAccepted, Pos: deferred.Pos()})
	}
	return liveAtBackedge
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
		if ok && ssaflow.MayAlias(condition, call) {
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
	probe analysisTrace.Probe,
	state deferFlowState,
	obligation deferObligation,
) deferFlowState {
	for ; state.index < len(state.block.Instrs); state.index++ {
		if state.status != resourceLive {
			continue
		}
		instruction := state.block.Instrs[state.index]
		status, reason := classifyResourceInstruction(evidence, probe, instruction, obligation)
		if status == resourceLive {
			continue
		}
		state.status = status
		outcome := analysisTrace.OutcomeAccepted
		if status == resourceUnknown {
			outcome = analysisTrace.OutcomeUnknown
		}
		probe.Evidence(analysisTrace.Step{
			Reason: reason, Outcome: outcome, Pos: instruction.Pos(),
			Details: map[string]string{"instruction": instruction.String()},
		})
	}
	return state
}

// Classification is deliberately asymmetric: explicit cleanup and proven
// transfer settle, opaque use suppresses, and ordinary operations on the
// receiver leave it live. Merely failing to find a cleanup proves nothing.
// The reason names the rule that moved the status, so a trace can attribute
// a settled or unknown resource to the instruction and rule that decided it.
func classifyResourceInstruction(
	evidence *lifecyclefacts.LifecycleEvidence,
	probe analysisTrace.Probe,
	instruction ssa.Instruction,
	obligation deferObligation,
) (resourceStatus, string) {
	if transfersResource(evidence, instruction, obligation.target) {
		return resourceSettled, "resource-transferred"
	}
	common := ssaflow.InstructionCall(instruction)
	if common == nil {
		if opaqueResourceUse(instruction, obligation.target) {
			return resourceUnknown, "resource-captured-or-stored"
		}
		return resourceLive, ""
	}
	receiver := ssaflow.CallReceiver(common)
	if sameObligationValue(receiver, obligation.target) {
		if slices.Contains(obligation.cleanup, ssaflow.CallName(common)) {
			return resourceSettled, "explicit-cleanup"
		}
		return resourceLive, ""
	}
	return resourceUseStatus(evidence, probe, instruction, obligation.target)
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
	probe analysisTrace.Probe,
	instruction ssa.Instruction,
	target ssa.Value,
) (resourceStatus, string) {
	common := ssaflow.InstructionCall(instruction)
	if common == nil {
		return resourceLive, ""
	}
	used := false
	for index, argument := range common.Args {
		alias := ssaflow.ProveMayAlias(argument, target)
		contains := !alias.Aliases && ssaflow.MayContainValue(argument, target)
		probe.Evidence(analysisTrace.Step{
			Reason: "argument-carries-resource", Outcome: analysisTrace.OutcomeObserved, Pos: instruction.Pos(),
			Details: map[string]string{
				"argument": argument.Name(), "alias": strconv.FormatBool(alias.Aliases), "alias-reason": string(alias.Reason),
				"contains": strconv.FormatBool(contains),
			},
		})
		if contains {
			// The callee received a wrapper holding the resource, not the
			// resource itself. Its summary describes the argument, so it can
			// neither release nor be proven to leave the contents alone:
			// unknown, not cleanup. Constructing the wrapper alone does not
			// settle the obligation either.
			// https://github.com/replicatedhq/troubleshoot/blob/eacd376c1245fe2ebcc3581f015f692d77d89af4/pkg/supportbundle/aftercollection.go#L39-L53
			return resourceUnknown, "wrapper-passed-to-callee"
		}
		if !alias.Aliases {
			if pointer, ok := argument.Type().Underlying().(*types.Pointer); ok {
				if _, aggregate := pointer.Elem().Underlying().(*types.Struct); aggregate &&
					ssaflow.ValueDerivesFrom(argument, target, map[ssa.Value]bool{}) {
					return resourceUnknown, "wrapper-passed-to-callee"
				}
			}
			continue
		}
		used = true
		if released, summarized := evidence.CalleeClaims(instruction, index, lifecyclefacts.ClaimReleases); summarized {
			if released {
				return resourceSettled, "callee-releases-argument"
			}
		}
	}
	if used && !evidence.CalleeSummarized(instruction) {
		return resourceUnknown, "unsummarized-callee-uses-resource"
	}
	return resourceLive, ""
}

// Capturing the resource in a closure or storing it in an aggregate is opaque:
// the lifetime of that containing owner is outside this loop-local proof.
func opaqueResourceUse(instruction ssa.Instruction, target ssa.Value) bool {
	if store, ok := instruction.(*ssa.Store); ok {
		switch store.Addr.(type) {
		case *ssa.FieldAddr, *ssa.IndexAddr:
			// A local aggregate whose address never leaves the function
			// lives no longer than this iteration, so a resource stored in
			// it, and closed through it, is still iteration-local.
			if ssaflow.AddressIsUnescapedLocal(store.Addr) {
				return false
			}
			return ssaflow.MayAlias(store.Val, target) || ssaflow.MayContainValue(store.Val, target)
		}
	}
	closure, ok := instruction.(*ssa.MakeClosure)
	if !ok {
		return false
	}
	for _, binding := range closure.Bindings {
		if ssaflow.MayAlias(binding, target) || ssaflow.MayContainValue(binding, target) {
			return true
		}
	}
	return false
}
