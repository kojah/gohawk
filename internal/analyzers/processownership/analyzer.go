// Package processownership implements the processownership gohawk analyzer.
package processownership

import (
	"go/types"
	"strings"

	"github.com/kojah/gohawk/analysisutil"
	ssautil "github.com/kojah/gohawk/analysisutil/ssa"
	"github.com/kojah/gohawk/internal/analyzerbase"

	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/analysis/passes/buildssa"
	"golang.org/x/tools/go/ssa"
)

// Analyzer returns this package's configured Go analysis pass.
func Analyzer() *analysis.Analyzer {
	return &analysis.Analyzer{
		Name:     "processownership",
		Doc:      "checks that started os/exec commands are waited on or transferred to a wait owner",
		Requires: []*analysis.Analyzer{buildssa.Analyzer},
		Run:      runProcessOwnership,
	}
}

func runProcessOwnership(pass *analysis.Pass) (any, error) {
	functions, err := ssautil.SourceSSAFunctions(pass)
	if err != nil {
		return nil, err
	}
	for _, function := range functions {
		for _, block := range function.Blocks {
			for _, instruction := range block.Instrs {
				start, ok := instruction.(*ssa.Call)
				if !ok || ssautil.CallName(start.Common()) != "Start" || !execCommandValue(ssautil.CallReceiver(start.Common())) {
					continue
				}
				command := ssautil.CallReceiver(start.Common())
				owners := processOwnersRegisteredBefore(function, start, command)
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
				if ssautil.AliasesAny(command, parameterValues(function.Params)) || ssautil.ExternallyOwnedValue(command) {
					continue
				}
				// Cleanup may be registered before Start. This is common when a
				// constructor builds a teardown closure first, then starts the
				// process and returns that closure to its caller.
				if processOwnershipDominatesStart(function, start, command) || processOwnerDominatesStart(function, start, owners) {
					continue
				}
				if successfulStartCannotReturn(start) {
					continue
				}
				if ssautil.UnownedReturn(start, func(candidate ssa.Instruction) bool {
					return processOwnershipAction(candidate, command)
				}, func(returned *ssa.Return) bool {
					// Returning an aggregate that contains the command transfers Wait
					// responsibility just as directly as returning *exec.Cmd itself.
					return startFailureReturn(returned, start) || ssautil.ReturnedValueOwnsValue(returned, command)
				}) {
					analyzerbase.Reportf(pass, analyzerbase.CheckProcessWait, start.Pos(), "started command is not waited on every successful return path")
				}
			}
		}
	}
	return nil, nil
}

func processOwnerDominatesStart(function *ssa.Function, start *ssa.Call, owners []ssa.Value) bool {
	startIndex := ssautil.InstructionIndex(start)
	for _, block := range function.Blocks {
		if !block.Dominates(start.Block()) {
			continue
		}
		limit := len(block.Instrs)
		if block == start.Block() {
			limit = startIndex
		}
		for _, instruction := range block.Instrs[:limit] {
			for _, owner := range owners {
				for _, method := range []string{"close", "Close", "kill", "Kill", "Wait", "wait"} {
					if ssautil.DeferredClosureCalls(instruction, method, owner) {
						return laterProcessOwnerWatcher(function, start, owners)
					}
				}
			}
		}
	}
	return false
}

func laterProcessOwnerWatcher(function *ssa.Function, start *ssa.Call, owners []ssa.Value) bool {
	for _, block := range function.Blocks {
		for _, instruction := range block.Instrs {
			spawn, ok := instruction.(*ssa.Go)
			if !ok || spawn.Pos() <= start.Pos() {
				continue
			}
			closure, _ := spawn.Common().Value.(*ssa.MakeClosure)
			if closure == nil {
				continue
			}
			for _, owner := range owners {
				if ssautil.ValueOwnsValue(closure, owner) {
					return true
				}
			}
		}
	}
	return false
}

func successfulStartCannotReturn(start *ssa.Call) bool {
	block := start.Block()
	for _, successor := range block.Succs {
		if success, known := ssautil.SuccessBranch(block, successor, start); known && success {
			return !ssautil.NormalReturnReachableFrom(successor)
		}
	}
	return false
}

func processOwnershipDominatesStart(function *ssa.Function, start *ssa.Call, command ssa.Value) bool {
	startIndex := ssautil.InstructionIndex(start)
	for _, block := range function.Blocks {
		if !block.Dominates(start.Block()) {
			continue
		}
		limit := len(block.Instrs)
		if block == start.Block() {
			limit = startIndex
		}
		for _, instruction := range block.Instrs[:limit] {
			if ssautil.DeferredClosureCalls(instruction, "Wait", command) || ssautil.ClosureCapturesValue(instruction, command) {
				return true
			}
		}
	}
	return false
}

func processOwnersRegisteredBefore(function *ssa.Function, start *ssa.Call, command ssa.Value) []ssa.Value {
	var owners []ssa.Value
	startIndex := ssautil.InstructionIndex(start)
	for _, block := range function.Blocks {
		if !block.Dominates(start.Block()) {
			continue
		}
		limit := len(block.Instrs)
		if block == start.Block() {
			limit = startIndex
		}
		for _, instruction := range block.Instrs[:limit] {
			call, ok := instruction.(*ssa.Call)
			if !ok || call.Common().StaticCallee() == nil {
				continue
			}
			for _, argument := range call.Common().Args {
				if ssautil.AliasesValue(argument, command) {
					owners = append(owners, call)
					if call.Referrers() != nil {
						for _, reference := range *call.Referrers() {
							if result, ok := reference.(*ssa.Extract); ok {
								owners = append(owners, result)
							}
						}
					}
					break
				}
			}
		}
	}
	return owners
}

func processOwnershipAction(instruction ssa.Instruction, command ssa.Value) bool {
	common := ssautil.InstructionCall(instruction)
	// os.Process.Release explicitly relinquishes the parent's wait/reap
	// obligation for deliberately detached daemons:
	// https://github.com/drn/argus/blob/9b4bb7e71217e22557f72531909bf803354d3ab4/internal/daemon/client/autostart_fork.go#L41-L45
	return waitsForCommand(instruction, command) ||
		ssautil.CallPackage(common) == "os" && ssautil.CallName(common) == "Release" && ssautil.ValueDerivesFrom(ssautil.CallReceiver(common), command, map[ssa.Value]bool{}) ||
		ssautil.DeferredClosureCalls(instruction, "Wait", command) ||
		ssautil.ClosureCallsMethod(instruction, "Wait", command) ||
		ssautil.ClosureCapturesValue(instruction, command) ||
		ssautil.StoresValueInField(instruction, command) ||
		ssautil.StoresOwnerOfValueInField(instruction, command) ||
		ssautil.CallTransfersValueToField(instruction, command) ||
		ssautil.CallCallsMethodOnArgumentOnEveryReturn(instruction, "Wait", command) ||
		ssautil.CallPackage(common) == "os" && ssautil.CallName(common) == "Exit"
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
		return ssautil.CallPackage(typed.Common()) != "os/exec"
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
	common := ssautil.InstructionCall(instruction)
	if common == nil {
		return false
	}
	if ssautil.CallName(common) == "Wait" && ssautil.AliasesValue(ssautil.CallReceiver(common), command) {
		return true
	}
	lower := strings.ToLower(ssautil.CallName(common))
	if !strings.Contains(lower, "wait") && !strings.Contains(lower, "join") {
		return false
	}
	for _, argument := range common.Args {
		if ssautil.AliasesValue(argument, command) {
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
		if success, known := ssautil.SuccessBranch(predecessor, returned.Block(), start); known {
			return !success
		}
	}
	for _, successor := range start.Block().Succs {
		success, known := ssautil.SuccessBranch(start.Block(), successor, start)
		if !known || success {
			continue
		}
		return blockReachable(successor, returned.Block()) && !successBranchReaches(start, returned.Block())
	}
	return false
}

func successBranchReaches(start *ssa.Call, target *ssa.BasicBlock) bool {
	for _, successor := range start.Block().Succs {
		if success, known := ssautil.SuccessBranch(start.Block(), successor, start); known && success {
			return blockReachable(successor, target)
		}
	}
	return false
}

func blockReachable(from, target *ssa.BasicBlock) bool {
	seen := map[*ssa.BasicBlock]bool{}
	queue := []*ssa.BasicBlock{from}
	for len(queue) > 0 {
		block := queue[0]
		queue = queue[1:]
		if block == target {
			return true
		}
		if seen[block] {
			continue
		}
		seen[block] = true
		queue = append(queue, block.Succs...)
	}
	return false
}
