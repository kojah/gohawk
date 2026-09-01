// Package channelownership implements the channelownership gohawk analyzer.
package channelownership

import (
	"go/types"

	"github.com/kojah/gohawk/internal/analysisutil"
	ssautil "github.com/kojah/gohawk/internal/analysisutil/ssa"
	"github.com/kojah/gohawk/internal/check"

	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/analysis/passes/buildssa"
	"golang.org/x/tools/go/ssa"
)

// Analyzer returns this package's configured Go analysis pass.
func Analyzer() *analysis.Analyzer {
	return &analysis.Analyzer{
		Name:     "channelownership",
		Doc:      "checks that channel closing remains with the channel owner",
		Requires: []*analysis.Analyzer{buildssa.Analyzer},
		Run:      runChannelOwnership,
	}
}

func runChannelOwnership(pass *analysis.Pass) (any, error) {
	functions, err := ssautil.SourceSSAFunctions(pass)
	if err != nil {
		return nil, err
	}
	callsites := ssautil.StaticCallsites(functions)
	for _, function := range functions {
		checkChannelOwnership(pass, function, callsites)
	}
	return nil, nil
}

func checkChannelOwnership(pass *analysis.Pass, function *ssa.Function, callsites map[*ssa.Function][]ssa.CallInstruction) {
	var parameters []ssa.Value
	for _, parameter := range function.Params {
		if ssautil.ChannelType(parameter) {
			parameters = append(parameters, parameter)
		}
	}
	for _, block := range function.Blocks {
		for _, instruction := range block.Instrs {
			common := ssautil.InstructionCall(instruction)
			if common == nil || ssautil.CallName(common) != analysisutil.BuiltinClose || len(common.Args) != 1 {
				continue
			}
			channel := common.Args[0]
			if closesBorrowedChannel(channel, parameters, callsites) {
				check.Reportf(pass, check.ChannelCallerClose, instruction.Pos(), "do not close a channel received from caller")
			}
		}
	}
}

func closesBorrowedChannel(channel ssa.Value, parameters []ssa.Value, callsites map[*ssa.Function][]ssa.CallInstruction) bool {
	for _, parameter := range parameters {
		if ssautil.SameValue(channel, parameter) && !channelOwnershipTransferredToGoroutine(parameter, callsites) {
			return true
		}
	}
	return false
}

func channelOwnershipTransferredToGoroutine(parameter ssa.Value, callsites map[*ssa.Function][]ssa.CallInstruction) bool {
	function := parameter.Parent()
	if guardedCloseContract(function, parameter) {
		return true
	}
	if function == nil || function.Object() != nil && function.Object().Exported() {
		return false
	}
	index := -1
	for candidate, current := range function.Params {
		if current == parameter {
			index = candidate
			break
		}
	}
	calls := callsites[function]
	if index < 0 || len(calls) == 0 {
		return false
	}
	for _, call := range calls {
		if index >= len(call.Common().Args) {
			return false
		}
		if _, ok := call.(*ssa.Go); ok {
			continue
		}
		argument := call.Common().Args[index]
		if ssautil.DefinitelyNil(argument) || channelRelinquishedAfterCall(call, argument) {
			continue
		}
		return false
	}
	// A channel passed only through `go helper(ch)` is an explicit producer
	// handoff: the spawning caller cannot perform the close after the helper
	// finishes. ElasticKV uses this contract for its refresh completion signal:
	// https://github.com/bootjp/elastickv/blob/ddbb0a5b60a691890cb5595c185cdb16fee478b3/proxy/leader_aware_backend.go#L195-L218
	return true
}

func guardedCloseContract(function *ssa.Function, parameter ssa.Value) bool {
	if function == nil {
		return false
	}
	hasGuard := false
	for _, block := range function.Blocks {
		for _, instruction := range block.Instrs {
			selection, ok := instruction.(*ssa.Select)
			if !ok {
				continue
			}
			for _, state := range selection.States {
				if state.Dir == types.RecvOnly && ssautil.SameValue(state.Chan, parameter) {
					hasGuard = true
				}
			}
		}
	}
	// A non-blocking receive immediately guarding close is an explicit safe-close
	// contract, not an ordinary callee closing a borrowed producer channel.
	// https://github.com/pancsta/asyncmachine-go/blob/cce9b31145cb07c1262ac0c71a696222b0119b75/internal/utils/utils.go#L44-L52
	return hasGuard
}

func channelRelinquishedAfterCall(call ssa.CallInstruction, channel ssa.Value) bool {
	// A private helper is also a designated close owner when every caller
	// relinquishes the channel at the call: after the helper returns there is no
	// send, close, or return of that channel. This covers synchronous acceptance
	// signals and producer methods invoked from an owned worker closure.
	for _, instruction := range reachableInstructions(call) {
		if returned, ok := instruction.(*ssa.Return); ok && ssautil.ReturnSameValue(returned, channel) {
			return false
		}
		if send, ok := instruction.(*ssa.Send); ok && ssautil.SameValue(send.Chan, channel) {
			return false
		}
		common := ssautil.InstructionCall(instruction)
		if common != nil && ssautil.CallName(common) == analysisutil.BuiltinClose && len(common.Args) == 1 && ssautil.SameValue(common.Args[0], channel) {
			return false
		}
	}
	return true
}

func reachableInstructions(start ssa.Instruction) []ssa.Instruction {
	index := ssautil.InstructionIndex(start)
	if index < 0 {
		return nil
	}
	type location struct {
		block *ssa.BasicBlock
		index int
	}
	type channelFlowKey struct {
		block int
		index int
	}
	queue := []location{{block: start.Block(), index: index + 1}}
	seen := map[channelFlowKey]bool{}
	var result []ssa.Instruction
	for len(queue) > 0 {
		current := queue[0]
		queue = queue[1:]
		key := channelFlowKey{block: current.block.Index, index: current.index}
		if seen[key] {
			continue
		}
		seen[key] = true
		result = append(result, current.block.Instrs[current.index:]...)
		for _, successor := range current.block.Succs {
			// Crossing a backedge may compare two different runtime channel
			// values represented by the same loop-carried SSA value. Reporting
			// that as send-after-close is not sufficiently precise.
			if successor.Dominates(current.block) {
				continue
			}
			queue = append(queue, location{block: successor})
		}
	}
	return result
}
