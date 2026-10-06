package concurrencyfacts

import (
	"go/token"
	"go/types"
	"slices"

	proofs "github.com/kojah/gohawk/internal/proof"
	"github.com/kojah/gohawk/internal/ssaflow"
	"golang.org/x/tools/go/ssa"
)

// Effect collection accounts for each instruction in execution order under
// the query budget and summary bound. The passive whitelist is part of the
// completeness proof: an unmodeled effect stops collection as unknown.

func (engine *Engine) collectEffects(function *ssa.Function, root bool) Summary {
	if function == nil || len(function.Blocks) == 0 {
		return Summary{Reason: ReasonBodyUnavailable}
	}
	if !root {
		if result, handled := engine.collectSelectFunction(function); handled {
			return result
		}
	}
	if !straightLineBody(function) {
		return engine.collectBranches(function, root)
	}
	var result Summary
	if reason := engine.collectBlock(&result, function.Blocks[0], root); reason != ReasonNone {
		if reason == ReasonSelectAlternatives {
			result.Reason = reason
			return result
		}
		return Summary{Reason: reason}
	}
	if len(result.deferred) != 0 {
		return Summary{Reason: ReasonDeferredEffectsUnknown}
	}
	if result.hasWorkerAlternatives() {
		result.Reason = ReasonSelectAlternatives
		result.AlternativesComplete = true
	}
	return result
}

func (engine *Engine) collectBlock(result *Summary, block *ssa.BasicBlock, root bool) Reason {
	for _, instruction := range block.Instrs {
		if !engine.budget.Spend() {
			engine.recordCutoff(instruction, cutoffInstruction)
			return ReasonBudgetExhausted
		}
		if reason := engine.appendInstruction(result, instruction, root); reason != ReasonNone {
			return reason
		}
		if result.operationCount() > maxOperations {
			engine.recordCutoff(instruction, cutoffInstruction)
			return ReasonSummaryLimit
		}
	}
	return ReasonNone
}

func (engine *Engine) appendInstruction(result *Summary, instruction ssa.Instruction, root bool) Reason {
	engine.cutoff = nil
	reason := engine.instructionEffects(result, instruction, root)
	if reason != ReasonNone {
		engine.recordCutoff(instruction, cutoffInstruction)
	} else {
		engine.cutoff = nil
	}
	return reason
}

func (engine *Engine) instructionEffects(result *Summary, instruction ssa.Instruction, root bool) Reason {
	switch instruction := instruction.(type) {
	case *ssa.Send:
		// Passing a reference in a message can add a participant or expose
		// shared storage. Only scalar payloads belong to this first proof.
		if !scalarType(instruction.X.Type()) {
			return ReasonPayloadUnknown
		}
		return engine.appendOperation(result, Send, instruction.Chan, instruction.Pos())
	case *ssa.UnOp:
		return engine.appendUnOp(result, instruction)
	case *ssa.Select:
		return engine.appendSelect(result, instruction)
	case *ssa.Call:
		if isCancelConstructor(instruction.Common()) && !root {
			return ReasonLocalContextUnknown
		}
		return engine.appendCall(result, instruction)
	case *ssa.Defer:
		return engine.deferCompletion(result, instruction)
	case *ssa.RunDefers:
		for _, deferred := range slices.Backward(result.deferred) {
			result.Operations = append(result.Operations, deferred)
		}
		result.deferred = nil
	case *ssa.Go:
		return engine.appendGo(result, instruction)
	case *ssa.Extract:
		if call, ok := instruction.Tuple.(*ssa.Call); ok && isCancelConstructor(call.Common()) {
			return ReasonNone
		}
		return passiveInstruction(instruction, root, engine.budget)
	case *ssa.FieldAddr:
		// A mutex reached through a write-once field is named by its path.
		if MutexPointer(instruction.Type()) && !synchronizationPointer(instruction.X.Type()) {
			if _, exact := engine.identityPath(instruction); exact {
				return ReasonNone
			}
		}
		return passiveInstruction(instruction, root, engine.budget)
	default:
		return passiveInstruction(instruction, root, engine.budget)
	}
	return ReasonNone
}

func (engine *Engine) appendUnOp(result *Summary, instruction *ssa.UnOp) Reason {
	switch instruction.Op {
	case token.ARROW:
		return engine.appendOperation(result, Receive, instruction.X, instruction.Pos())
	case token.MUL:
		return loadEffect(instruction, engine.budget)
	case token.NOT, token.SUB, token.XOR:
		// Scalar negation and complement cannot block, panic, or synchronize.
		if scalarType(instruction.Type()) {
			return ReasonNone
		}
	default:
	}
	return ReasonLoadUnknown
}

func loadEffect(load *ssa.UnOp, budget *proofs.SearchBudget) Reason {
	if ssaflow.ChannelType(load) {
		if path, exact := embeddedPathWithin(load.X, budget); exact && path.Depth > 0 {
			return ReasonNone
		}
	}
	if isGlobal(load.X) {
		// Reading a package variable cannot block, panic, or synchronize. The
		// loaded value is not a stable resource identity: any later operation
		// on it must resolve one through reference, which declines globals.
		return ReasonNone
	}
	if readableAddress(load.X) || inertValue(load.Type()) {
		return ReasonNone
	}
	return ReasonLoadUnknown
}

func (summary Summary) operationCount() int {
	count := len(summary.Operations) + len(summary.deferred) + len(summary.CancellationInputs)
	for _, worker := range summary.Workers {
		count += len(worker.Operations)
		for _, alternative := range worker.Alternatives {
			count += len(alternative)
		}
	}
	return count
}

func (engine *Engine) appendOperation(result *Summary, kind Kind, value ssa.Value, pos token.Pos) Reason {
	resource, ok := engine.reference(value)
	if !ok {
		return ReasonChannelIdentityUnknown
	}
	result.Operations = append(result.Operations, Operation{Kind: kind, Resource: resource, Source: pos, Site: pos})
	return ReasonNone
}

func passiveInstruction(instruction ssa.Instruction, root bool, budget *proofs.SearchBudget) Reason {
	if effectFree(instruction, root) {
		return ReasonNone
	}
	switch instruction := instruction.(type) {
	case *ssa.If, *ssa.Jump:
		// The acyclic collector checks every successor and requires identical
		// ordered effects at joins and returns.
		return ReasonNone
	case *ssa.Phi:
		// Inert values cannot change resource identity. Every incoming
		// computation is still checked by the instruction whitelist.
		if inertValue(instruction.Type()) {
			return ReasonNone
		}
	case *ssa.DebugRef, *ssa.Alloc, *ssa.MakeClosure:
		return ReasonNone
	case *ssa.FieldAddr:
		if _, exact := embeddedPathWithin(instruction, budget); exact && !synchronizationPointer(instruction.X.Type()) {
			return ReasonNone
		}
	case *ssa.Store:
		// A fresh group's zero state is part of the counter proof. Resetting
		// or copying it invalidates that proof, even through a local address.
		// Spilling a *pointer* to a primitive into a local closure cell does
		// not copy the primitive; binding later proves the cell is stable.
		if localSynchronizationPointerStore(instruction) ||
			localAddress(instruction.Addr) && !containsSynchronization(instruction.Val.Type()) &&
				!synchronizationPointer(instruction.Addr.Type()) {
			return ReasonNone
		}
	case *ssa.ChangeType:
		if ssaflow.ChannelType(instruction) {
			return ReasonNone
		}
	case *ssa.MakeInterface, *ssa.ChangeInterface:
		// Boxing or converting an interface does not itself publish the
		// value. Every subsequent use must still resolve to a complete callee;
		// opaque dispatch is unknown.
		return ReasonNone
	case *ssa.MakeChan:
		// Callee allocation sites cannot identify runtime instances across
		// separate calls. Only channels created by the root are admitted.
		if root {
			return ReasonNone
		}
	}
	// This whitelist is also the scope-completeness proof: no unmodelled
	// call, publication, launch, panic, or blocking action is skipped.
	return ReasonEffectUnknown
}

// effectFree groups the instruction families that need no ordered effect:
// scalar arithmetic, an admitted return, and inert caller-owned data.
func effectFree(instruction ssa.Instruction, root bool) bool {
	return scalarInstruction(instruction) || admittedReturn(instruction, root) || inertDataInstruction(instruction)
}

// A root's results reach its caller only after the root returns, and root
// consumers prove waits that block before any return. A helper's result can
// instead add its caller as a participant, so a helper may return only inert
// values: a caller cannot reach a resource through one without an operation
// that would itself stop the summary.
func admittedReturn(instruction ssa.Instruction, root bool) bool {
	returned, ok := instruction.(*ssa.Return)
	if !ok {
		return false
	}
	if root {
		return true
	}
	for _, result := range returned.Results {
		if !inertValue(result.Type()) {
			return false
		}
	}
	return true
}

func localSynchronizationPointerStore(store *ssa.Store) bool {
	if !localAddress(store.Addr) {
		return false
	}
	pointer, ok := store.Addr.Type().Underlying().(*types.Pointer)
	return ok && synchronizationPointer(pointer.Elem())
}

// Empty protocol effects require positive evidence for every instruction, not
// just scalar arguments. This admits scalar arithmetic and comparisons.
// Division and shifts are left out; no measured cutoff has needed them.
func scalarInstruction(instruction ssa.Instruction) bool {
	binary, ok := instruction.(*ssa.BinOp)
	if !ok || !scalarType(binary.X.Type()) || !scalarType(binary.Y.Type()) {
		return false
	}
	return slices.Contains([]token.Token{
		token.ADD, token.SUB, token.MUL, token.AND, token.OR, token.XOR, token.AND_NOT,
		token.EQL, token.NEQ, token.LSS, token.LEQ, token.GTR, token.GEQ,
	}, binary.Op)
}

func scalarType(value types.Type) bool {
	basic, ok := value.Underlying().(*types.Basic)
	return ok && basic.Info()&(types.IsBoolean|types.IsNumeric|types.IsString) != 0
}

func isGlobal(address ssa.Value) bool {
	_, ok := address.(*ssa.Global)
	return ok
}
