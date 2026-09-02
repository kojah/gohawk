// Package channelsafety implements the channelsafety gohawk analyzer.
package channelsafety

import (
	"go/token"

	"github.com/kojah/gohawk/internal/check"
	"github.com/kojah/gohawk/internal/ssaflow"
	"github.com/kojah/gohawk/internal/syntax"

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
	functions, err := ssaflow.SourceSSAFunctions(pass)
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
			common := ssaflow.InstructionCall(instruction)
			if !ssaflow.CallMatchesSymbol(common, syntax.Builtin("close")) || len(common.Args) != 1 {
				continue
			}
			if _, deferred := instruction.(*ssa.Defer); deferred {
				continue
			}
			// Crossing a backedge may compare two different runtime channel
			// values represented by the same loop-carried SSA value. Reporting
			// that as send-after-close is not sufficiently precise, so only
			// instructions reachable without a back edge are candidates.
			for _, candidate := range ssaflow.InstructionsReachableAfter(instruction) {
				send, ok := candidate.(*ssa.Send)
				if !ok || !ssaflow.SameValue(send.Chan, common.Args[0]) || reported[send.Pos()] {
					continue
				}
				reported[send.Pos()] = true
				check.Reportf(pass, check.ChannelSendAfterClose, send.Pos(), "send follows close of channel")
			}
		}
	}
}
