package processownership

import (
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

func commandReturnedByHelper(command ssa.Value) bool {
	return commandReturnedByHelperSeen(command, map[ssa.Value]bool{})
}

func commandReturnedByHelperSeen(command ssa.Value, seen map[ssa.Value]bool) bool {
	// Commands produced by a helper may carry a lifecycle contract the caller
	// cannot see. Track only transparent SSA wrappers and merges; a direct
	// os/exec constructor remains locally owned and must still be waited for.
	if command == nil || seen[command] {
		return false
	}
	seen[command] = true
	switch typed := command.(type) {
	case *ssa.Call:
		return !ssaflow.CallMatchesAnySymbol(
			typed.Common(),
			syntax.PackageFunction("os/exec", "Command"),
			syntax.PackageFunction("os/exec", "CommandContext"),
		)
	case *ssa.ChangeInterface:
		return commandReturnedByHelperSeen(typed.X, seen)
	case *ssa.ChangeType:
		return commandReturnedByHelperSeen(typed.X, seen)
	case *ssa.Convert:
		return commandReturnedByHelperSeen(typed.X, seen)
	case *ssa.MakeInterface:
		return commandReturnedByHelperSeen(typed.X, seen)
	case *ssa.UnOp:
		if typed.X.Referrers() == nil {
			return false
		}
		for _, reference := range *typed.X.Referrers() {
			store, ok := reference.(*ssa.Store)
			if ok && store.Addr == typed.X && commandReturnedByHelperSeen(store.Val, seen) {
				return true
			}
		}
	case *ssa.Phi:
		for _, edge := range typed.Edges {
			if commandReturnedByHelperSeen(edge, seen) {
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
	if ssaflow.CallMatchesSymbol(common, syntax.PackageMethod(syntax.MethodSymbol{PackagePath: "os/exec", Receiver: "Cmd", Name: "Wait"})) &&
		(ssaflow.ValueDerivesFrom(ssaflow.CallReceiver(common), command, map[ssa.Value]bool{}) ||
			osProcessDerivedFromCommand(ssaflow.CallReceiver(common), command)) {
		return true
	}
	return false
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
