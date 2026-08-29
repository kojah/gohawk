package general

import (
	"go/ast"
	"go/constant"
	"go/token"
	"go/types"
	"strings"

	"github.com/kojah/gohawk/analysisutil"

	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/analysis/passes/buildssa"
	"golang.org/x/tools/go/ssa"
)

func channelPolicyAnalyzer() *analysis.Analyzer {
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
	functions, err := analysisutil.SourceSSAFunctions(pass)
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
	for _, function := range functions {
		checkSSAChannelOwnership(pass, function, config)
	}
	return nil, nil
}

func checkChannelCapacity(pass *analysis.Pass, file *ast.File, call *ast.CallExpr, maximum int64) {
	if maximum < 0 {
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
	analysisutil.Reportf(pass, call.Args[1].Pos(), "channel capacity %d requires a bounded rationale comment", capacity)
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

func checkSSAChannelOwnership(pass *analysis.Pass, function *ssa.Function, config channelPolicyConfig) {
	var parameters []ssa.Value
	for _, parameter := range function.Params {
		if analysisutil.ChannelType(parameter) {
			parameters = append(parameters, parameter)
		}
	}
	reportedSends := map[token.Pos]bool{}
	for _, block := range function.Blocks {
		for _, instruction := range block.Instrs {
			common := analysisutil.InstructionCall(instruction)
			if common == nil || analysisutil.CallName(common) != analysisutil.BuiltinClose || len(common.Args) != 1 {
				continue
			}
			channel := common.Args[0]
			if aliasesAny(channel, parameters) {
				analysisutil.Reportf(pass, instruction.Pos(), "do not close a channel received from caller")
			}
			// Scheduling a deferred close does not close the channel at this
			// program point; sends before the function returns remain valid.
			if _, deferred := instruction.(*ssa.Defer); deferred {
				continue
			}
			for _, candidate := range reachableInstructions(instruction) {
				send, ok := candidate.(*ssa.Send)
				if !ok || !analysisutil.AliasesValue(send.Chan, channel) || reportedSends[send.Pos()] {
					continue
				}
				reportedSends[send.Pos()] = true
				analysisutil.Reportf(pass, send.Pos(), "send follows close of channel")
			}
		}
	}
}

func reachableInstructions(start ssa.Instruction) []ssa.Instruction {
	index := analysisutil.InstructionIndex(start)
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
