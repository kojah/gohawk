// Package channelsafety implements the channelsafety gohawk analyzer.
package channelsafety

import (
	"go/token"

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
		Name: "channelsafety", Doc: "checks channel operations for reachable use after close",
		Requires: []*analysis.Analyzer{buildssa.Analyzer}, Run: runChannelSafety,
	}
}

func runChannelSafety(pass *analysis.Pass) (any, error) {
	functions, err := ssautil.SourceSSAFunctions(pass)
	if err != nil {
		return nil, err
	}
	for _, function := range functions {
		reportSendsAfterClose(pass, function)
	}
	return nil, nil
}

func reportSendsAfterClose(pass *analysis.Pass, function *ssa.Function) {
	reported := map[token.Pos]bool{}
	for _, block := range function.Blocks {
		for _, instruction := range block.Instrs {
			common := ssautil.InstructionCall(instruction)
			if common == nil || ssautil.CallName(common) != analysisutil.BuiltinClose || len(common.Args) != 1 {
				continue
			}
			if _, deferred := instruction.(*ssa.Defer); deferred {
				continue
			}
			for _, candidate := range reachableInstructions(instruction) {
				send, ok := candidate.(*ssa.Send)
				if !ok || !ssautil.SameValue(send.Chan, common.Args[0]) || reported[send.Pos()] {
					continue
				}
				reported[send.Pos()] = true
				check.Reportf(pass, check.ChannelSendAfterClose, send.Pos(), "send follows close of channel")
			}
		}
	}
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
	type flowKey struct{ block, index int }
	queue := []location{{block: start.Block(), index: index + 1}}
	seen := map[flowKey]bool{}
	var result []ssa.Instruction
	for len(queue) > 0 {
		current := queue[0]
		queue = queue[1:]
		key := flowKey{block: current.block.Index, index: current.index}
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
