package concurrencyfacts

import (
	"go/types"

	"github.com/kojah/gohawk/internal/ssaflow"
	"golang.org/x/tools/go/ssa"
)

// BindDeclaration instantiates published formal effects with the same binding
// rules as imported calls. Declaration identity must belong to this callee;
// the summary provider owns that lookup, while this pass owns field mapping.
func (engine *Engine) BindDeclaration(call ssa.CallInstruction, fact Fact, budget *ssaflow.SearchBudget) Summary {
	engine.mu.Lock()
	defer engine.mu.Unlock()
	return engine.query(budget).bindDeclaration(call, fact)
}

// Declaration returns the same formal-parameter vocabulary for local and
// imported functions. Local allocations and captures cannot be represented;
// callers needing local identities should use Function or AtCall instead.
// All returned slices are detached from the cached publication. Copying shares
// the supplied allowance; cutoff returns no declaration, never a partial fact.
func (engine *Engine) Declaration(function *ssa.Function, budget *ssaflow.SearchBudget) (Fact, bool) {
	engine.mu.Lock()
	defer engine.mu.Unlock()
	if function == nil || !budget.Spend() {
		return Fact{}, false
	}
	if len(function.Blocks) != 0 {
		return exportSummary(function, engine.summaries.Function(function, budget), budget)
	}
	object, _ := function.Object().(*types.Func)
	fact, ok := engine.facts[object]
	if !ok || fact.Version != factVersion {
		return Fact{}, false
	}
	return cloneDeclaration(fact, budget)
}
