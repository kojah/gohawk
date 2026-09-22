package processownership

import (
	"go/token"
	"go/types"

	"github.com/kojah/gohawk/internal/ssaflow"
	"github.com/kojah/gohawk/internal/syntax"

	"golang.org/x/tools/go/ssa"
)

func osProcessDerivedFromCommand(value, command ssa.Value) bool {
	if value == nil || value.Type() == nil {
		return false
	}
	pointer, ok := value.Type().Underlying().(*types.Pointer)
	return ok && syntax.NamedType(pointer.Elem(), "os", "Process") && ssaflow.ValueDerivesFrom(value, command, map[ssa.Value]bool{})
}

// returnsProcessHandle reports whether a return hands the caller the exact
// os.Process projected from the started command.
func returnsProcessHandle(returned *ssa.Return, command ssa.Value) bool {
	for _, result := range returned.Results {
		if osProcessDerivedFromCommand(result, command) {
			return true
		}
	}
	return false
}

// commandForms are the wrappers a command keeps its provenance through.
const commandForms = ssaflow.TransparentChangeInterface | ssaflow.TransparentChangeType | ssaflow.TransparentConvert | ssaflow.TransparentMakeInterface

func commandReturnedByHelper(command ssa.Value) bool {
	// Commands produced by a helper may carry a lifecycle contract the caller
	// cannot see. Track only transparent SSA wrappers and merges; a direct
	// os/exec constructor remains locally owned and must still be waited for.
	return ssaflow.NewReachingWalk(commandForms).Any(command, commandReturnedByHelperLeaf)
}

func commandReturnedByHelperLeaf(walk ssaflow.ReachingWalk, command ssa.Value) bool {
	switch typed := command.(type) {
	case *ssa.Call:
		return !ssaflow.CallMatchesAnySymbol(
			typed.Common(),
			syntax.PackageFunction("os/exec", "Command"),
			syntax.PackageFunction("os/exec", "CommandContext"),
		)
	case *ssa.UnOp:
		if typed.X.Referrers() == nil {
			return false
		}
		for _, reference := range *typed.X.Referrers() {
			store, ok := reference.(*ssa.Store)
			if ok && store.Addr == typed.X && walk.Any(store.Val, commandReturnedByHelperLeaf) {
				return true
			}
		}
	}
	return false
}

func parameterValues(parameters []*ssa.Parameter) []ssa.Value {
	values := make([]ssa.Value, len(parameters))
	for index, parameter := range parameters {
		values[index] = parameter
	}
	return values
}

func execCommandValue(value ssa.Value) bool {
	if value == nil {
		return false
	}
	pointer, ok := value.Type().Underlying().(*types.Pointer)
	return ok && syntax.NamedType(pointer.Elem(), "os/exec", "Cmd")
}

func waitsForCommand(instruction ssa.Instruction, command ssa.Value) bool {
	common := ssaflow.InstructionCall(instruction)
	if common == nil {
		return false
	}
	receiver := ssaflow.CallReceiver(common)
	if ssaflow.CallMatchesSymbol(common, syntax.PackageMethod(syntax.MethodSymbol{PackagePath: "os/exec", Receiver: "Cmd", Name: "Wait"})) {
		// A closure-local FreeVar is the mapped capture cell, not the command
		// value. Its caller maps the captured command into this frame.
		if _, captured := command.(*ssa.FreeVar); captured {
			return ssaflow.ValueDerivesFrom(receiver, command, map[ssa.Value]bool{})
		}
		return ssaflow.NewStorage(ssaflow.NewSearchBudget(1000)).Same(receiver, command).Proven()
	}
	// Waiting through cmd.Process reaps the same operating-system child. Mache
	// uses the lower-level handle after signaling an entire process group:
	// https://github.com/agentic-research/mache/blob/ccaf44c3688c12324af57747b6fb0c6a33ca93e0/internal/leyline/procgroup_unix_test.go#L30-L51
	return ssaflow.CallMatchesSymbol(common, syntax.PackageMethod(syntax.MethodSymbol{PackagePath: "os", Receiver: "Process", Name: "Wait"})) &&
		osProcessDerivedFromCommand(receiver, command)
}

// impossibleStartedProcessNilReturn recognizes only the immediate defensive
// guard after successful Start. Start guarantees Process is non-nil, but later
// stores or opaque calls can invalidate that fact, so require mutation-free
// adjacent blocks instead of assuming it throughout the function.
// https://github.com/stacktower-io/stacktower/blob/69ff07430089898cc79af381f6e0c3a927a7d149/internal/cli/auth_device.go#L118-L124
func impossibleStartedProcessNilReturn(returned *ssa.Return, start *ssa.Call, command ssa.Value) bool {
	predecessors := returned.Block().Preds
	if len(predecessors) != 1 || ssaflow.InstructionIndex(start) != len(start.Block().Instrs)-3 {
		return false
	}
	guard := predecessors[0]
	if len(guard.Preds) != 1 || guard.Preds[0] != start.Block() || len(guard.Instrs) != 4 {
		return false
	}
	if success, known := ssaflow.SuccessBranch(start.Block(), guard, start); !known || !success {
		return false
	}
	comparison := immediateProcessNilComparison(guard, command)
	if comparison == nil {
		return false
	}
	switch comparison.Op {
	case token.EQL:
		return guard.Succs[0] == returned.Block()
	case token.NEQ:
		return guard.Succs[1] == returned.Block()
	default:
		return false
	}
}

// immediateProcessNilComparison matches a block that only reads the command's
// Process and branches on its nilness, with no intervening call or mutation.
func immediateProcessNilComparison(guard *ssa.BasicBlock, command ssa.Value) *ssa.BinOp {
	field, fieldOK := guard.Instrs[0].(*ssa.FieldAddr)
	load, loadOK := guard.Instrs[1].(*ssa.UnOp)
	comparison, comparisonOK := guard.Instrs[2].(*ssa.BinOp)
	branch, branchOK := guard.Instrs[3].(*ssa.If)
	if !fieldOK || !loadOK || !comparisonOK || !branchOK || len(guard.Succs) != 2 {
		return nil
	}
	if !ssaflow.SameValue(field.X, command) || load.X != field || load.Op != token.MUL ||
		!osProcessDerivedFromCommand(load, command) || branch.Cond != comparison {
		return nil
	}
	comparesProcessNil := comparison.X == load && ssaflow.DefinitelyNil(comparison.Y) ||
		comparison.Y == load && ssaflow.DefinitelyNil(comparison.X)
	if !comparesProcessNil {
		return nil
	}
	return comparison
}

func startFailureReturn(returned *ssa.Return, start *ssa.Call) bool {
	// Wait is not required on the path where Start itself failed. Accept that
	// exception only when SSA branch evidence separates failure from every path
	// that can reach the same return.
	if returned.Block() == start.Block() {
		return false
	}
	for _, predecessor := range returned.Block().Preds {
		if success, known := ssaflow.SuccessBranch(predecessor, returned.Block(), start); known {
			return !success
		}
	}
	for _, successor := range start.Block().Succs {
		success, known := ssaflow.SuccessBranch(start.Block(), successor, start)
		if !known || success {
			continue
		}
		return ssaflow.BlockReachable(successor, returned.Block()) && !successBranchReaches(start, returned.Block())
	}
	return false
}

func successBranchReaches(start *ssa.Call, target *ssa.BasicBlock) bool {
	for _, successor := range start.Block().Succs {
		if success, known := ssaflow.SuccessBranch(start.Block(), successor, start); known && success {
			return ssaflow.BlockReachable(successor, target)
		}
	}
	return false
}
