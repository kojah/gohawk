// Package channelpolicy implements the channelpolicy gohawk analyzer.
package channelpolicy

import (
	"go/ast"
	"go/constant"
	"go/token"
	"go/types"
	"strings"

	"github.com/kojah/gohawk/internal/analysisutil"
	ssautil "github.com/kojah/gohawk/internal/analysisutil/ssa"
	"github.com/kojah/gohawk/internal/check"

	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/analysis/passes/buildssa"
	"golang.org/x/tools/go/ssa"
)

// Analyzer returns this package's configured Go analysis pass.
func Analyzer() *analysis.Analyzer {
	config := channelPolicyConfig{maxUnexplainedCapacity: 1}
	analyzer := &analysis.Analyzer{
		Name:     "channelpolicy",
		Doc:      "checks channel capacity and closing ownership",
		Requires: []*analysis.Analyzer{buildssa.Analyzer},
	}
	analyzer.Flags.Int64Var(&config.maxUnexplainedCapacity, "max-unexplained-capacity", 1, "largest channel capacity allowed without a rationale; negative disables the check")
	analyzer.Run = func(pass *analysis.Pass) (any, error) {
		return runChannelPolicy(pass, config)
	}
	return analyzer
}

type channelPolicyConfig struct {
	maxUnexplainedCapacity int64
}

func runChannelPolicy(pass *analysis.Pass, config channelPolicyConfig) (any, error) {
	functions, err := ssautil.SourceSSAFunctions(pass)
	if err != nil {
		return nil, err
	}
	for _, file := range pass.Files {
		if !analysisutil.AnalyzeFile(pass, file) {
			continue
		}
		ast.Inspect(file, func(node ast.Node) bool {
			if call, ok := node.(*ast.CallExpr); ok {
				checkChannelCapacity(pass, file, call, config.maxUnexplainedCapacity)
			}
			return true
		})
	}
	callsites := ssautil.StaticCallsites(functions)
	for _, function := range functions {
		checkSSAChannelOwnership(pass, function, callsites)
	}
	return nil, nil
}

func checkChannelCapacity(pass *analysis.Pass, file *ast.File, call *ast.CallExpr, maximum int64) {
	if maximum < 0 {
		return
	}
	// Capacity rationale is an operational ownership policy for production
	// queues. A channel created in a test file is fixture synchronization with a
	// test-scoped lifetime; the caller-close and send-after-close checks still
	// analyze it. ccLoad uses fixed buffers to collect known concurrent results:
	// https://github.com/caidaoli/ccLoad/blob/9ed11fe1b1dd2bfed12a32c9290354ff3cdc9b77/internal/app/admin_codex_auth_test.go#L3768-L3784
	if strings.HasSuffix(pass.Fset.Position(file.Pos()).Filename, "_test.go") {
		return
	}
	builtin, ok := call.Fun.(*ast.Ident)
	if !ok || builtin.Name != "make" || len(call.Args) < 2 {
		return
	}
	if _, ok := pass.TypesInfo.TypeOf(call).Underlying().(*types.Chan); !ok {
		return
	}
	value := pass.TypesInfo.Types[call.Args[1]].Value
	if value == nil {
		return
	}
	capacity, exact := constant.Int64Val(value)
	if !exact || capacity <= maximum || channelRationale(pass, file, call.Pos()) {
		return
	}
	check.Reportf(pass, check.ChannelCapacityRationale, call.Args[1].Pos(), "channel capacity %d requires a bounded rationale comment", capacity)
}

func channelRationale(pass *analysis.Pass, file *ast.File, position token.Pos) bool {
	line := pass.Fset.Position(position).Line
	for _, group := range file.Comments {
		commentLine := pass.Fset.Position(group.Pos()).Line
		if commentLine != line && commentLine != line-1 {
			continue
		}
		text := strings.ToLower(group.Text())
		if strings.Contains(text, "bounded:") || strings.Contains(text, "capacity:") {
			return true
		}
	}
	return false
}

func checkSSAChannelOwnership(pass *analysis.Pass, function *ssa.Function, callsites map[*ssa.Function][]ssa.CallInstruction) {
	var parameters []ssa.Value
	for _, parameter := range function.Params {
		if ssautil.ChannelType(parameter) {
			parameters = append(parameters, parameter)
		}
	}
	reportedSends := map[token.Pos]bool{}
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
			// Scheduling a deferred close does not close the channel at this
			// program point; sends before the function returns remain valid.
			if _, deferred := instruction.(*ssa.Defer); deferred {
				continue
			}
			for _, candidate := range reachableInstructions(instruction) {
				send, ok := candidate.(*ssa.Send)
				if !ok || !ssautil.SameValue(send.Chan, channel) || reportedSends[send.Pos()] {
					continue
				}
				reportedSends[send.Pos()] = true
				check.Reportf(pass, check.ChannelSendAfterClose, send.Pos(), "send follows close of channel")
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
