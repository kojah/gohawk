package resourcelifetime

import (
	"go/token"

	"github.com/kojah/gohawk/internal/ssaflow"
	"golang.org/x/tools/go/ssa"
)

// Visible default-client and transport effects share one scan for HEAD and
// local header-only acquisitions. Only HEAD's root permits a default-client
// load used exclusively by Do; that allowance never enters helper summaries.
// Only visible bodies are expanded; recursive or shortened evidence cannot
// establish absence of visible changes. Hidden package effects stay opaque.

// httpEffectsBudget bounds visible-body effect queries. Both HTTP acquisition
// families retain this quota; exhaustion supplies the conservative may-modify
// answer rather than evidence that a global default is unchanged.
const httpEffectsBudget = 4000

// defaultClientVisiblyModified reports visible configuration or escapes of the
// package defaults. HEAD permits direct default-client Do loads in the root,
// while nested visible callees retain the strict default-effect policy.
func defaultClientVisiblyModified(function *ssa.Function) bool {
	budget := ssaflow.NewSearchBudget(httpEffectsBudget)
	return newHTTPWriterEffects().scanDefaultOverrides(function, budget, true)
}

func (effects *httpWriterEffects) visibleOverrides(function *ssa.Function, budget *ssaflow.SearchBudget) bool {
	return effects.scanDefaultOverrides(function, budget, false)
}

func (effects *httpWriterEffects) scanDefaultOverrides(function *ssa.Function, budget *ssaflow.SearchBudget, allowRootDo bool) bool {
	for instruction := range ssaflow.InstructionsWithin(function, budget) {
		if allowRootDo {
			if load, ok := instruction.(*ssa.UnOp); ok && load.Op == token.MUL &&
				ssaflow.ValueMatchesSymbol(load.X, httpDefaultClient) && onlyHTTPDoUses(load) {
				continue
			}
		}
		for _, operand := range instruction.Operands(nil) {
			if operand != nil && ssaflow.ValueMatchesAnySymbol(*operand, httpDefaultClient, httpDefaultTransport) {
				return true
			}
		}
		callee, _ := ssaflow.DirectCallee(ssaflow.InstructionCall(instruction))
		if callee != nil && len(callee.Blocks) != 0 && effects.overrides.Function(callee, budget) {
			return true
		}
	}
	return resourceFlowExhausted(budget)
}
