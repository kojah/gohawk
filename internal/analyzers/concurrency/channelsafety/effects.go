package channelsafety

import (
	"github.com/kojah/gohawk/internal/passes/concurrencyfacts"
	"github.com/kojah/gohawk/internal/ssaflow"
	"github.com/kojah/gohawk/internal/syntax"

	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/ssa"
)

// Effects expose synchronous operations at their caller instruction. A defer
// registration and a launch do not execute the callee here. Unknown summaries
// contribute no witness; they never prove that a helper leaves a channel open.
func channelEffects(pass *analysis.Pass, function *ssa.Function) map[ssa.Instruction][]concurrencyfacts.Operation {
	provider := summaryKnowledge.Provider(pass)
	budget := ssaflow.NewSearchBudget(2000)
	effects := make(map[ssa.Instruction][]concurrencyfacts.Operation)
	for _, block := range function.Blocks {
		for _, instruction := range block.Instrs {
			switch instruction := instruction.(type) {
			case *ssa.Send:
				effects[instruction] = []concurrencyfacts.Operation{{
					Kind: concurrencyfacts.Send, Resource: concurrencyfacts.Reference{Value: instruction.Chan},
				}}
			case *ssa.Call:
				common := instruction.Common()
				if ssaflow.CallMatchesSymbol(common, syntax.Builtin("close")) && len(common.Args) == 1 {
					effects[instruction] = []concurrencyfacts.Operation{{
						Kind: concurrencyfacts.Close, Resource: concurrencyfacts.Reference{Value: common.Args[0]},
					}}
					continue
				}
				summary, _ := provider.ConcurrencyAtCall(instruction, budget)
				if summary.Complete() {
					for _, operation := range summary.Operations {
						if !operation.Resource.Indirect {
							effects[instruction] = append(effects[instruction], operation)
						}
					}
				}
			}
		}
	}
	return effects
}
