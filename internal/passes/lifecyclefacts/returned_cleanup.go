package lifecyclefacts

import (
	"fmt"
	"go/types"

	"github.com/kojah/gohawk/internal/lifecycle"
	"github.com/kojah/gohawk/internal/ssaflow"
	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/ssa"
)

// Returned cleanup facts describe relationships between values, not acquisition
// or ownership by themselves. Consumers still establish the obligation and
// require actual invocation of the callback before crediting its exact effect.
const returnedCleanupVersion = 1

// ReturnedCleanupSummary contains exact callback-result relations for a factory.
type ReturnedCleanupSummary struct {
	Version int
	Effects []ReturnedCleanupEffect
}

// ReturnedCleanupEffect associates a factory relation with one completion verb.
type ReturnedCleanupEffect struct {
	Relation lifecycle.ReturnedCleanupRelation
	Method   string
	Invoke   bool
}

func summarizeReturnedCleanup(pass *analysis.Pass, function *ssa.Function) *ReturnedCleanupSummary {
	budget := ssaflow.NewSearchBudget(ssaflow.SummaryBudget)
	summary := &ReturnedCleanupSummary{Version: returnedCleanupVersion}
	results := function.Signature.Results()
	for callback := range min(results.Len(), 4) {
		if _, ok := results.At(callback).Type().Underlying().(*types.Signature); !ok {
			continue
		}
		for target := range len(function.Params) + min(results.Len(), 4) {
			relation, targetType := returnedCleanupTarget(function, callback, target)
			if relation.TargetIsResult && relation.Target == callback {
				continue
			}
			for _, method := range conditionalMethods(targetType) {
				if !budget.Spend() {
					return summary
				}
				request := lifecycle.CompletionRequest{
					Budget: budget, InvokeTarget: method == "",
					ReturnedSummaries: returnedCleanupLookup(func(callee *ssa.Function) (Fact, bool) { return factForFunction(pass, callee) }),
					Summarized:        conditionalLookup(func(instruction ssa.Instruction) (Fact, bool) { return importFact(pass, instruction) }, budget, nil),
				}
				if !request.InvokeTarget {
					request.Methods = []string{method}
				}
				if lifecycle.ProveReturnedCleanup(function, relation, request).Proven() {
					summary.Effects = append(summary.Effects, ReturnedCleanupEffect{Relation: relation, Method: method, Invoke: request.InvokeTarget})
				}
			}
		}
	}
	if len(summary.Effects) == 0 {
		return nil
	}
	return summary
}

func returnedCleanupTarget(function *ssa.Function, callback, target int) (lifecycle.ReturnedCleanupRelation, types.Type) {
	relation := lifecycle.ReturnedCleanupRelation{CallbackResult: callback, Target: target}
	if target < len(function.Params) {
		return relation, function.Params[target].Type()
	}
	relation.TargetIsResult = true
	relation.Target -= len(function.Params)
	return relation, function.Signature.Results().At(relation.Target).Type()
}

func returnedCleanupLookup(lookup func(*ssa.Function) (Fact, bool)) lifecycle.ReturnedCleanupLookup {
	return func(function *ssa.Function, method string, invoke bool) []lifecycle.ReturnedCleanupRelation {
		fact, ok := lookup(function)
		if !ok || fact.ReturnedCleanup == nil || fact.ReturnedCleanup.Version != returnedCleanupVersion {
			return nil
		}
		var relations []lifecycle.ReturnedCleanupRelation
		for _, effect := range fact.ReturnedCleanup.Effects {
			if effect.Method == method && effect.Invoke == invoke {
				relations = append(relations, effect.Relation)
			}
		}
		return relations
	}
}

func (evidence *LifecycleEvidence) returnedCleanupLookup() lifecycle.ReturnedCleanupLookup {
	return returnedCleanupLookup(func(function *ssa.Function) (Fact, bool) {
		if evidence.pass == nil {
			return Fact{}, false
		}
		summaries, ok := evidence.pass.ResultOf[Analyzer].(Summaries)
		if !ok {
			return Fact{}, false
		}
		fact, found := summaries[function]
		return fact, found
	})
}

func (fact *Fact) returnedCleanupDescriptions() []string {
	if fact.ReturnedCleanup == nil {
		return nil
	}
	var lines []string
	for _, effect := range fact.ReturnedCleanup.Effects {
		space, method := "parameter", effect.Method
		if effect.Relation.TargetIsResult {
			space = "result"
		}
		if effect.Invoke {
			method = "invoke"
		}
		lines = append(lines, fmt.Sprintf("callback result %d: %s %s %d", effect.Relation.CallbackResult, method, space, effect.Relation.Target))
	}
	return lines
}
