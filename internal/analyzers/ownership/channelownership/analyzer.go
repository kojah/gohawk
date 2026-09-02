// Package channelownership implements the channelownership gohawk analyzer.
package channelownership

import (
	"go/ast"
	"go/types"
	"strings"

	"github.com/kojah/gohawk/internal/check"
	"github.com/kojah/gohawk/internal/ssaflow"
	"github.com/kojah/gohawk/internal/syntax"
	analysisTrace "github.com/kojah/gohawk/internal/trace"

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
	functions, err := ssaflow.SourceSSAFunctions(pass)
	if err != nil {
		return nil, err
	}
	ssaResult := pass.ResultOf[buildssa.Analyzer].(*buildssa.SSA)
	callsites := buildChannelCallsites(channelCallerFunctions(ssaResult.Pkg, ssaResult.SrcFuncs), functions)
	for _, function := range functions {
		checkChannelOwnership(pass, function, callsites)
	}
	return nil, nil
}

type channelCloseReason string

const (
	channelReasonLocal                   channelCloseReason = "locally-owned-channel"
	channelReasonDocumented              channelCloseReason = "documented-close-owner"
	channelReasonGuarded                 channelCloseReason = "guarded-close-owner"
	channelReasonCallerRelinquished      channelCloseReason = "caller-relinquished"
	channelReasonFiniteBoundRelinquished channelCloseReason = "finite-bound-callers-relinquished"
	channelReasonUnresolved              channelCloseReason = "indirect-call-target-unresolved"
	channelReasonNoCallsites             channelCloseReason = "caller-ownership-not-proven"
	channelReasonCallerRetains           channelCloseReason = "caller-retains-channel"
)

type channelCloseProof struct {
	report bool
	reason channelCloseReason
}

func checkChannelOwnership(pass *analysis.Pass, function *ssa.Function, callsites map[*ssa.Function]*channelCalleeCalls) {
	var parameters []ssa.Value
	for _, parameter := range function.Params {
		if ssaflow.ChannelType(parameter) {
			parameters = append(parameters, parameter)
		}
	}
	for _, block := range function.Blocks {
		for _, instruction := range block.Instrs {
			common := ssaflow.InstructionCall(instruction)
			if !ssaflow.CallMatchesSymbol(common, syntax.Builtin("close")) || len(common.Args) != 1 {
				continue
			}
			channel := common.Args[0]
			proof := proveBorrowedChannelClose(pass, function, channel, parameters, callsites)
			emitChannelCloseTrace(pass, function, instruction, proof)
			if proof.report {
				check.Reportf(pass, check.ChannelCallerClose, instruction.Pos(), "do not close a channel received from caller")
			}
		}
	}
}

func proveBorrowedChannelClose(
	pass *analysis.Pass,
	function *ssa.Function,
	channel ssa.Value,
	parameters []ssa.Value,
	callsites map[*ssa.Function]*channelCalleeCalls,
) channelCloseProof {
	for _, parameter := range parameters {
		if !ssaflow.SameValue(channel, parameter) {
			continue
		}
		if documentedCloseOwnership(pass, function, parameter) {
			return channelCloseProof{reason: channelReasonDocumented}
		}
		if guardedCloseContract(function, parameter) {
			return channelCloseProof{reason: channelReasonGuarded}
		}
		return proveCallerRelinquishesChannel(parameter, callsites)
	}
	return channelCloseProof{reason: channelReasonLocal}
}

func documentedCloseOwnership(pass *analysis.Pass, function *ssa.Function, parameter ssa.Value) bool {
	if function == nil || function.Object() == nil || parameter == nil || parameter.Name() == "" {
		return false
	}
	for _, file := range pass.Files {
		for _, declaration := range file.Decls {
			source, ok := declaration.(*ast.FuncDecl)
			if !ok || pass.TypesInfo.Defs[source.Name] != function.Object() || source.Doc == nil {
				continue
			}
			documentation := strings.ToLower(source.Doc.Text())
			contract := strings.ToLower(parameter.Name()) + " is closed"
			// A parameter-specific API promise transfers close ownership to the
			// callee. Restic documents its producer channel this way:
			// https://github.com/restic/restic/blob/ba802d42b7294c98b62c16d1157ea3e80820c019/internal/checker/checker.go#L146-L155
			return strings.Contains(documentation, contract)
		}
	}
	return false
}

func proveCallerRelinquishesChannel(
	parameter ssa.Value,
	callsites map[*ssa.Function]*channelCalleeCalls,
) channelCloseProof {
	function := parameter.Parent()
	if function == nil || function.Object() != nil && function.Object().Exported() {
		return channelCloseProof{report: true, reason: channelReasonNoCallsites}
	}
	index := -1
	for candidate, current := range function.Params {
		if current == parameter {
			index = candidate
			break
		}
	}
	calleeCalls := callsites[function]
	if index < 0 || calleeCalls == nil {
		return channelCloseProof{report: true, reason: channelReasonNoCallsites}
	}
	if !calleeCalls.complete {
		return channelCloseProof{report: true, reason: channelReasonUnresolved}
	}
	if len(calleeCalls.calls) == 0 {
		return channelCloseProof{report: true, reason: channelReasonNoCallsites}
	}
	for _, callsite := range calleeCalls.calls {
		if index >= len(callsite.arguments) {
			return channelCloseProof{report: true, reason: channelReasonUnresolved}
		}
		if _, ok := callsite.instruction.(*ssa.Go); ok {
			continue
		}
		argument := callsite.arguments[index]
		if ssaflow.DefinitelyNil(argument) || channelRelinquishedAfterCall(callsite.instruction, argument) {
			continue
		}
		return channelCloseProof{report: true, reason: channelReasonCallerRetains}
	}
	if calleeCalls.finiteBound {
		// A finite phi of exact bound producer methods transfers close ownership
		// when every closure reference ends at an outer call where the caller
		// relinquishes the channel. Buildkite selects literal/glob producers this
		// way; the synthetic wrapper itself is not sufficient caller evidence:
		// https://github.com/buildkite/agent/blob/e206ddf806af50a1ba8c9a6dd501dfda0b730818/internal/artifact/uploader.go#L258-L338
		return channelCloseProof{reason: channelReasonFiniteBoundRelinquished}
	}
	// A channel passed only through `go helper(ch)` is an explicit producer
	// handoff: the spawning caller cannot perform the close after the helper
	// finishes. ElasticKV uses this contract for its refresh completion signal:
	// https://github.com/bootjp/elastickv/blob/ddbb0a5b60a691890cb5595c185cdb16fee478b3/proxy/leader_aware_backend.go#L195-L218
	return channelCloseProof{reason: channelReasonCallerRelinquished}
}

func emitChannelCloseTrace(pass *analysis.Pass, function *ssa.Function, instruction ssa.Instruction, proof channelCloseProof) {
	checkID := string(check.ChannelCallerClose)
	if !analysisTrace.Enabled("channelownership", checkID) {
		return
	}
	analysisTrace.Emit(pass, analysisTrace.Event{
		Analyzer: "channelownership",
		Check:    checkID,
		Phase:    "candidate",
		Reason:   "channel-close",
		Outcome:  analysisTrace.OutcomeObserved,
		Pos:      instruction.Pos(),
		Function: function.String(),
	})
	outcome := analysisTrace.OutcomeAccepted
	if proof.report {
		outcome = analysisTrace.OutcomeRejected
	}
	analysisTrace.Emit(pass, analysisTrace.Event{
		Analyzer: "channelownership",
		Check:    checkID,
		Phase:    "decision",
		Reason:   string(proof.reason),
		Outcome:  outcome,
		Pos:      instruction.Pos(),
		Function: function.String(),
	})
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
				if state.Dir == types.RecvOnly && ssaflow.SameValue(state.Chan, parameter) {
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
		if returned, ok := instruction.(*ssa.Return); ok && ssaflow.ReturnSameValue(returned, channel) {
			return false
		}
		if send, ok := instruction.(*ssa.Send); ok && ssaflow.SameValue(send.Chan, channel) {
			return false
		}
		common := ssaflow.InstructionCall(instruction)
		if ssaflow.CallMatchesSymbol(common, syntax.Builtin("close")) && len(common.Args) == 1 && ssaflow.SameValue(common.Args[0], channel) {
			return false
		}
	}
	return true
}

func reachableInstructions(start ssa.Instruction) []ssa.Instruction {
	index := ssaflow.InstructionIndex(start)
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
