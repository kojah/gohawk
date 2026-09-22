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
	value, known := provider.resultCondition(branch.Cond, budget)
	if !known || budget.Exhausted() {
		return successors
	}
	if value {
		return successors[:1]
	}
	return successors[1:]
}

func (provider *Provider) resultCondition(value ssa.Value, budget *ssaflow.SearchBudget) (bool, bool) {
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
	if comparison.Op == token.NEQ {
		equal = !equal
	}
	return equal, known
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
