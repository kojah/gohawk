package concurrencyfacts

import (
	"go/types"
	"slices"

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
// The returned effect slice is detached from the cached publication.
func (engine *Engine) Declaration(function *ssa.Function, budget *ssaflow.SearchBudget) (Fact, bool) {
	engine.mu.Lock()
	defer engine.mu.Unlock()
	if function == nil || !budget.Spend() {
		return Fact{}, false
	}
	if len(function.Blocks) != 0 {
		return exportSummary(function, engine.summaries.Function(function, budget))
	}
	object, _ := function.Object().(*types.Func)
	fact, ok := engine.facts[object]
	if !ok || fact.Version != factVersion {
		return Fact{}, false
	}
	fact.Effects = cloneFactEffects(fact.Effects)
	fact.CancellationInputs = slices.Clone(fact.CancellationInputs)
	fact.Workers = slices.Clone(fact.Workers)
	for index := range fact.Workers {
		fact.Workers[index].Effects = cloneFactEffects(fact.Workers[index].Effects)
	}
	return fact, true
}

func cloneFactEffects(effects []Effect) []Effect {
	cloned := slices.Clone(effects)
	for index := range cloned {
		cloned[index].Fields = slices.Clone(cloned[index].Fields)
	}
	return cloned
}
