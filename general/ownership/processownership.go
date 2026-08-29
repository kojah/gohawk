package ownership

import (
	"go/types"
	"strings"

	"github.com/kojah/gohawk/analysisutil"

	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/analysis/passes/buildssa"
	"golang.org/x/tools/go/ssa"
)

func processOwnershipAnalyzer() *analysis.Analyzer {
	return &analysis.Analyzer{
		Name:     "processownership",
		Doc:      "checks that started os/exec commands are waited on or transferred to a wait owner",
		Requires: []*analysis.Analyzer{buildssa.Analyzer},
		Run:      runProcessOwnership,
	}
}

func runProcessOwnership(pass *analysis.Pass) (any, error) {
	functions, err := analysisutil.SourceSSAFunctions(pass)
	if err != nil {
		return nil, err
	}
	for _, function := range functions {
		for _, block := range function.Blocks {
			for _, instruction := range block.Instrs {
				start, ok := instruction.(*ssa.Call)
				if !ok || analysisutil.CallName(start.Common()) != "Start" || !execCommandValue(analysisutil.CallReceiver(start.Common())) {
					continue
				}
				command := analysisutil.CallReceiver(start.Common())
				// A helper returning *exec.Cmd may already have registered cleanup
				// or wait ownership. Without interprocedural evidence either way,
				// reporting here would trade precision for recall. containerd wraps
				// command construction and returns the started command in binaryIO:
				// https://github.com/containerd/containerd/blob/716cbaf51212adb5e80ca1c30b644bfeb9c9d779/cmd/containerd-shim-runc-v2/process/io.go#L288-L330
				if commandReturnedByHelper(command) {
					continue
				}
				// Caller retains a parameter command after this helper returns, so
				// helper-local Start does not transfer caller's Wait responsibility.
				if aliasesAny(command, parameterValues(function.Params)) || analysisutil.ExternallyOwnedValue(command) {
					continue
				}
				if analysisutil.UnownedReturn(start, func(candidate ssa.Instruction) bool {
					common := analysisutil.InstructionCall(candidate)
					return waitsForCommand(candidate, command) ||
						analysisutil.DeferredClosureCalls(candidate, "Wait", command) ||
						analysisutil.ClosureCallsMethod(candidate, "Wait", command) ||
						analysisutil.ClosureCapturesValue(candidate, command) ||
						analysisutil.StoresValueInField(candidate, command) ||
						analysisutil.CallTransfersValueToField(candidate, command) ||
						analysisutil.CallPackage(common) == "os" && analysisutil.CallName(common) == "Exit"
				}, func(returned *ssa.Return) bool {
					// Returning an aggregate that contains the command transfers Wait
					// responsibility just as directly as returning *exec.Cmd itself.
					return startFailureReturn(returned, start) || analysisutil.ReturnedValueOwnsValue(returned, command)
				}) {
					reportf(pass, checkProcessWait, start.Pos(), "started command is not waited on every successful return path")
				}
			}
		}
	}
	return nil, nil
}

func commandReturnedByHelper(command ssa.Value) bool {
	return commandReturnedByHelperSeen(command, map[ssa.Value]bool{})
}

func commandReturnedByHelperSeen(command ssa.Value, seen map[ssa.Value]bool) bool {
	if command == nil || seen[command] {
		return false
	}
	seen[command] = true
	switch typed := command.(type) {
	case *ssa.Call:
		return analysisutil.CallPackage(typed.Common()) != "os/exec"
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
	return ok && analysisutil.NamedType(pointer.Elem(), "os/exec", "Cmd")
}

func waitsForCommand(instruction ssa.Instruction, command ssa.Value) bool {
	common := analysisutil.InstructionCall(instruction)
	if common == nil {
		return false
	}
	if analysisutil.CallName(common) == "Wait" && analysisutil.AliasesValue(analysisutil.CallReceiver(common), command) {
		return true
	}
	lower := strings.ToLower(analysisutil.CallName(common))
	if !strings.Contains(lower, "wait") && !strings.Contains(lower, "join") {
		return false
	}
	for _, argument := range common.Args {
		if analysisutil.AliasesValue(argument, command) {
			return true
		}
	}
	return false
}

func startFailureReturn(returned *ssa.Return, start *ssa.Call) bool {
	if returned.Block() == start.Block() {
		return false
	}
	for _, predecessor := range returned.Block().Preds {
		if success, known := resourceSuccessBranch(predecessor, returned.Block(), start); known {
			return !success
		}
	}
	return false
}
