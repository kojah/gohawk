package summaries

import (
	"go/constant"
	"go/token"

	"github.com/kojah/gohawk/internal/passes/resultfacts"
	"github.com/kojah/gohawk/internal/ssaflow"
	"golang.org/x/tools/go/ssa"
)

// ResultOf maps a direct result to its function-summary slot. This is not
// context-sensitive inference: arguments do not strengthen the guarantee.
func (provider *Provider) ResultOf(value ssa.Value, budget *ssaflow.SearchBudget) resultfacts.Guarantee {
	call, index, ok := ssaflow.CallResultSource(value)
	if !ok {
		return resultfacts.Unknown
	}
	result, available := provider.ForFunction(ssaflow.ResolvedCallee(call.Common())).Results(budget)
	if available != Available {
		return resultfacts.Unknown
	}
	return result.Result(index)
}

// FeasibleSuccessors augments existing predecessor-sensitive literal evidence
// with the requested result component. Unknown never eliminates a successor;
// nilness here establishes neither resource ownership nor a cleanup duty.
func (provider *Provider) FeasibleSuccessors(block, predecessor *ssa.BasicBlock, budget *ssaflow.SearchBudget) []*ssa.BasicBlock {
	successors := ssaflow.FeasibleSuccessors(block, predecessor)
	if len(successors) != 2 || len(block.Instrs) == 0 {
		return successors
	}
	branch, ok := block.Instrs[len(block.Instrs)-1].(*ssa.If)
	if !ok {
		return successors
	}
	value, known := provider.resultCondition(branch.Cond, block, budget)
	if !known || budget.Exhausted() {
		return successors
	}
	if value {
		return successors[:1]
	}
	return successors[1:]
}

// Successors adapts FeasibleSuccessors to the obligation walk's hook with a
// bounded budget per query. A nil provider yields no hook, so the walk keeps
// its default feasibility.
func (provider *Provider) Successors() func(block, predecessor *ssa.BasicBlock) []*ssa.BasicBlock {
	if provider == nil {
		return nil
	}
	return func(block, predecessor *ssa.BasicBlock) []*ssa.BasicBlock {
		return provider.FeasibleSuccessors(block, predecessor, ssaflow.NewSearchBudget(ssaflow.SummaryBudget))
	}
}

func (provider *Provider) resultCondition(value ssa.Value, block *ssa.BasicBlock, budget *ssaflow.SearchBudget) (bool, bool) {
	guarantee := provider.ResultOf(value, budget)
	if guarantee == resultfacts.AlwaysTrue || guarantee == resultfacts.AlwaysFalse {
		return guarantee == resultfacts.AlwaysTrue, true
	}
	comparison, ok := value.(*ssa.BinOp)
	if !ok || comparison.Op != token.EQL && comparison.Op != token.NEQ {
		return false, false
	}
	left, right := comparison.X, comparison.Y
	if _, constantLeft := left.(*ssa.Const); constantLeft {
		left, right = right, left
	}
	literal, ok := right.(*ssa.Const)
	if !ok {
		return false, false
	}
	equal, known := resultEqualsLiteral(provider.ResultOf(left, budget), literal)
	if !known && literal.IsNil() {
		equal, known = provider.pairedNilness(left, block, budget)
	}
	if comparison.Op == token.NEQ {
		equal = !equal
	}
	return equal, known
}

// pairedNilness decides a result's nilness from its callee's result-pair
// relation and the error branch already taken on the path: a result proven
// non-nil whenever its error is nil cannot be nil below the success arm of
// that error check, and one proven nil whenever its error is non-nil cannot
// be non-nil below the failure arm. Only a comparison that dominates the
// asking block counts, so the branch was taken on every path here.
func (provider *Provider) pairedNilness(value ssa.Value, block *ssa.BasicBlock, budget *ssaflow.SearchBudget) (isNil bool, known bool) {
	call, index, ok := ssaflow.CallResultSource(value)
	if !ok || block == nil {
		return false, false
	}
	summary, available := provider.ForFunction(ssaflow.ResolvedCallee(call.Common())).Results(budget)
	if available != Available {
		return false, false
	}
	for _, relation := range summary.Relations() {
		if relation.Result != index || relation.Kind != resultfacts.NonNilWhenResultNil && relation.Kind != resultfacts.NilWhenResultNonNil {
			continue
		}
		errorValue := ssaflow.CallResult(call, relation.Operand)
		errorNil, decided := errorNilnessOnPath(block, errorValue)
		if !decided {
			continue
		}
		if errorNil && relation.Kind == resultfacts.NonNilWhenResultNil {
			return false, true
		}
		if !errorNil && relation.Kind == resultfacts.NilWhenResultNonNil {
			return true, true
		}
	}
	return false, false
}

// errorNilnessOnPath reports the nilness a dominating nil comparison of
// errorValue established for every path into block.
func errorNilnessOnPath(block *ssa.BasicBlock, errorValue ssa.Value) (bool, bool) {
	if errorValue == nil || block.Parent() == nil {
		return false, false
	}
	for _, candidate := range block.Parent().Blocks {
		if candidate == block || len(candidate.Succs) != 2 {
			continue
		}
		for _, successor := range candidate.Succs {
			if success, decided := ssaflow.SuccessBranch(candidate, successor, errorValue); decided && successor.Dominates(block) {
				return success, true
			}
		}
	}
	return false, false
}

func resultEqualsLiteral(guarantee resultfacts.Guarantee, literal *ssa.Const) (bool, bool) {
	if literal.IsNil() {
		return guarantee == resultfacts.AlwaysNil, guarantee == resultfacts.AlwaysNil || guarantee == resultfacts.AlwaysNonNil
	}
	if literal.Value != nil && literal.Value.Kind() == constant.Bool &&
		(guarantee == resultfacts.AlwaysTrue || guarantee == resultfacts.AlwaysFalse) {
		return (guarantee == resultfacts.AlwaysTrue) == constant.BoolVal(literal.Value), true
	}
	return false, false
}

// ArgumentReturnedUnchanged resolves a call result to the argument the callee
// is proven to return unchanged, under the same static type. It answers
// identity only: what the caller passed in is what it got back. Ownership,
// release, and every other lifecycle question about that value stay with
// the caller's own evidence.
//
//nolint:ireturn // SSA values keep their concrete forms.
func (provider *Provider) ArgumentReturnedUnchanged(value ssa.Value, budget *ssaflow.SearchBudget) (ssa.Value, bool) {
	call, index, ok := ssaflow.CallResultSource(value)
	if !ok {
		return nil, false
	}
	summary, available := provider.ForFunction(ssaflow.ResolvedCallee(call.Common())).Results(budget)
	if available != Available {
		return nil, false
	}
	for _, relation := range summary.Relations() {
		if relation.Kind == resultfacts.ReturnsParameter && relation.Result == index && relation.Operand < len(call.Common().Args) {
			return call.Common().Args[relation.Operand], true
		}
	}
	return nil, false
}
