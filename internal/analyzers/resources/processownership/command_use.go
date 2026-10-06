package processownership

import (
	"go/types"

	"github.com/kojah/gohawk/internal/heapmodel"
	"github.com/kojah/gohawk/internal/lifecycle"
	"github.com/kojah/gohawk/internal/ssaflow"

	proofs "github.com/kojah/gohawk/internal/proof"
	"golang.org/x/tools/go/ssa"
)

// Command-use discovery distinguishes a handle handoff from local scalar/pipe
// consumption. It supplies the final detached-intent boundary, not Wait proof.
// Instruction, CFG, binding, operand, reaching-value and stored-value visits
// share the caller allowance; interrupted absence stays unknown.
func proveCommandUseAfterStart(start *ssa.Call, command ssa.Value, budget *proofs.SearchBudget) proofs.Proof {
	for instruction := range ssaflow.InstructionsWithin(start.Parent(), budget) {
		// Preserve possible ordering through back edges. The runtime-value census
		// deliberately stops there and cannot replace this structural use policy.
		if instruction == start || !ssaflow.InstructionMayFollowWithin(start, instruction, budget) {
			continue
		}
		if instructionCarriesCommand(instruction, start, command, budget) {
			return commandUseProof(true, budget)
		}
		if budget.Exhausted() {
			break
		}
	}
	return commandUseProof(false, budget)
}

func instructionCarriesCommand(instruction ssa.Instruction, start *ssa.Call, command ssa.Value, budget *proofs.SearchBudget) bool {
	if _, ok := instruction.(*ssa.DebugRef); ok {
		return false
	}
	// A literal that captures the handle may wait on it later.
	if closure, ok := instruction.(*ssa.MakeClosure); ok {
		for _, binding := range closure.Bindings {
			if !budget.Spend() {
				return false
			}
			if heapmodel.CapturedBindingMatches(binding, command) {
				return true
			}
		}
	}
	// PID/name reads carry data rather than a handle another owner can reap.
	// https://github.com/redis/agent-filesystem/blob/62aebf8f4d4b3a3866f4034fc6501af7d5d4a133/mount/cmd/agent-filesystem-mount/main.go#L70-L90
	if !handsValueOn(instruction) {
		return false
	}
	for _, operand := range instruction.Operands(nil) {
		if !budget.Spend() {
			return false
		}
		if operand == nil || *operand == nil || heapmodel.ValueDerivesFrom(*operand, start) {
			continue
		}
		if proveHandleCarried(*operand, command, budget).Proven() {
			return true
		}
		if budget.Exhausted() {
			return false
		}
	}
	return false
}

func commandUseProof(found bool, budget *proofs.SearchBudget) proofs.Proof {
	if budget.Exhausted() {
		return proofs.Proof{Reason: proofs.EvidenceBudgetExhausted}
	}
	state, reason := proofs.EvidenceDisproven, proofs.EvidenceNotFound
	if found {
		state, reason = proofs.EvidenceProven, proofs.EvidenceStructuralWalk
	}
	return proofs.Proof{State: state, Reason: reason, Provenance: proofs.EvidenceFromLocalSSA}
}

// handsValueOn reports whether an instruction can pass a value it consumes
// to code or storage that outlives the instruction. Local standard IO on a
// command pipe does not hand on its process owner; Go and Defer retain that question.
func handsValueOn(instruction ssa.Instruction) bool {
	switch operation := instruction.(type) {
	case *ssa.Call:
		return !commandPipeOperation(operation.Common())
	case ssa.CallInstruction, *ssa.Store, *ssa.Return, *ssa.Send, *ssa.MapUpdate, *ssa.Panic:
		return true
	}
	return false
}

// proveHandleCarried reports whether value is the command handle or something
// bound to it: a projection such as cmd.Process, a pipe the command returned,
// or an aggregate holding either. Derivation stops at a scalar, because a PID
// or a name read from the handle is data the recipient cannot wait on or
// release.
func proveHandleCarried(value, command ssa.Value, budget *proofs.SearchBudget) proofs.Proof {
	forms := ssaflow.TransparentChangeInterface | ssaflow.TransparentChangeType | ssaflow.TransparentConvert | ssaflow.TransparentMakeInterface
	// All storage and operand edges share one visited set. Starting a new walk
	// on either edge loops forever on cyclic owner structures (seen while
	// auditing kiwifs with its swaggo/swag dependency).
	// https://github.com/kiwifs/kiwifs/blob/3961d5e70a9e0ef457e58e29c40c52c870d57e73/go.mod
	var leaf func(ssaflow.ReachingWalk, ssa.Value) bool
	leaf = func(walk ssaflow.ReachingWalk, value ssa.Value) bool {
		if call, _, result := ssaflow.CallResultSource(value); result && commandPipeOperation(call.Common()) {
			return false
		}
		if _, scalar := value.Type().Underlying().(*types.Basic); scalar {
			return false
		}
		if heapmodel.MayAlias(value, command) {
			return true
		}
		if load, ok := value.(*ssa.UnOp); ok {
			for stored := range lifecycle.StoredIntoWithin(load.X, budget) {
				if walk.Any(stored, leaf) {
					return true
				}
			}
		}
		instruction, ok := value.(ssa.Instruction)
		if !ok {
			return false
		}
		for _, operand := range instruction.Operands(nil) {
			if !budget.Spend() {
				return false
			}
			if operand != nil && walk.Any(*operand, leaf) {
				return true
			}
		}
		return false
	}
	found := ssaflow.NewReachingWalk(forms).Within(budget).Any(value, leaf)
	return commandUseProof(found, budget)
}
