// Package resourcemodel connects exact resource relationships to per-path
// obligations and proven state transitions. It reuses the heap/storage model
// for identity and leaves diagnostic policy to the consuming analyzer.
package resourcemodel

import (
	"github.com/kojah/gohawk/internal/heapmodel"
	"github.com/kojah/gohawk/internal/lifecycle"
	"github.com/kojah/gohawk/internal/ssaflow"
	"github.com/kojah/gohawk/internal/syntax"
	"golang.org/x/tools/go/ssa"
)

var rowsNextResultSet = syntax.PackageMethod(syntax.MethodSymbol{
	PackagePath: "database/sql", Receiver: "Rows", Name: "NextResultSet",
})

// ConditionalReleases binds one search budget to the external state contracts.
func ConditionalReleases(budget *ssaflow.SearchBudget) lifecycle.CompletionSummaryLookup {
	return func(instruction ssa.Instruction, target ssa.Value, method string, invoke bool, predicate lifecycle.CompletionPredicate) bool {
		return ConditionalRelease(instruction, target, method, invoke, predicate, budget)
	}
}

// ConditionalRelease proves that a named external call releases target when
// its selected result has the requested outcome. A missing contract is not
// evidence that the call leaves target open.
func ConditionalRelease(
	instruction ssa.Instruction,
	target ssa.Value,
	method string,
	invoke bool,
	predicate lifecycle.CompletionPredicate,
	budget *ssaflow.SearchBudget,
) bool {
	call, synchronous := instruction.(*ssa.Call)
	if !synchronous || invoke || method != "Close" ||
		predicate.Result != 0 || predicate.Outcome != lifecycle.CompletionWhenFalse {
		return false
	}
	if budget == nil || !budget.Spend() {
		return false
	}
	if resultSetCall(call.Common(), target) {
		// Rows.NextResultSet closes Rows before returning false, both at the end
		// of the result sets and on a driver error.
		// https://go.dev/src/database/sql/sql.go (Rows.NextResultSet)
		return heapmodel.NewStorage(budget).Same(ssaflow.CallReceiver(call.Common()), target).Proven()
	}
	return forwardedConditionalRelease(call, target, budget, resultSetCall)
}

func resultSetCall(common *ssa.CallCommon, target ssa.Value) bool {
	if ssaflow.CallMatchesSymbol(common, rowsNextResultSet) {
		return true
	}
	// An interface invocation is equivalent only when the exact value bound
	// to that interface is a *sql.Rows. The receiver-identity proof is still
	// required by the caller; the method name alone proves nothing.
	return common != nil && common.IsInvoke() && common.Method != nil && common.Method.Name() == "NextResultSet" &&
		syntax.NamedType(target.Type(), "database/sql", "Rows")
}

// forwardedConditionalRelease binds a straight-line owner's field to the
// exact caller resource when its method returns one nested API call's result.
// Both the callee field path and the caller's resource relationship must be
// exact. A different field, mutation, branch, or extra call declines proof.
// https://github.com/ecodeclub/ekit/blob/a7e05db26220f9cef930579b39237cb09ae7b513/sqlx/scanner.go#L62-L64
func forwardedConditionalRelease(
	call *ssa.Call, target ssa.Value, budget *ssaflow.SearchBudget,
	contract func(*ssa.CallCommon, ssa.Value) bool,
) bool {
	callee := call.Common().StaticCallee()
	if callee == nil || len(callee.Blocks) != 1 || len(callee.Params) != 1 || len(call.Common().Args) != 1 {
		return false
	}
	block := callee.Blocks[0]
	var nested *ssa.Call
	var returned *ssa.Return
	for _, instruction := range block.Instrs {
		if !budget.Spend() {
			return false
		}
		switch typed := instruction.(type) {
		case *ssa.FieldAddr, *ssa.UnOp, *ssa.DebugRef:
		case *ssa.Call:
			if nested != nil || !contract(typed.Common(), target) {
				return false
			}
			nested = typed
		case *ssa.Return:
			returned = typed
		default:
			return false
		}
	}
	if nested == nil || returned == nil || len(returned.Results) != 1 || returned.Results[0] != nested {
		return false
	}
	receiver := ssaflow.CallReceiver(nested.Common())
	path, ok := ssaflow.AccessPathSteps(receiver, callee.Params[0], map[ssa.Value]bool{})
	if !ok {
		return false
	}
	relation := ProveRelation(call.Common().Args[0], target, call, budget)
	return relation.Proven() && relation.Relation.At(path)
}
