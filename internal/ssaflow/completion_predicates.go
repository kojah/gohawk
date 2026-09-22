package ssaflow

import (
	"go/types"

	"github.com/kojah/gohawk/internal/syntax"
	"golang.org/x/tools/go/ssa"
)

// CompletionOutcome names the observed result of a synchronous helper call.
// Zero means unconditional; the other values require the named result to have
// the corresponding Boolean or error-interface value.
type CompletionOutcome uint8

const (
	CompletionAlways CompletionOutcome = iota
	CompletionWhenTrue
	CompletionWhenFalse
	CompletionWhenNil
	CompletionWhenNonNil
)

// CompletionPredicate is a serializable condition on one function result.
type CompletionPredicate struct {
	Result  int
	Outcome CompletionOutcome
}

// CompletionSummaryLookup supplies positive guarantees for unavailable callees.
// The implementation must bind target to an exact argument, distinguish the
// requested method from callback invocation, and match the complete predicate.
// False means no guarantee, never proof that the callee has no effect.
type CompletionSummaryLookup func(ssa.Instruction, ssa.Value, string, bool, CompletionPredicate) bool

func (condition completionCondition) predicate() CompletionPredicate {
	return CompletionPredicate{Result: condition.result, Outcome: CompletionOutcome(condition.kind)}
}

// ProveCompletionForResult summarizes exact parameter cleanup on normal returns
// matching predicate. It reuses the completion engine and its shared budget,
// callback bindings, and recursion guard; it does not invent a caller or SSA.
func ProveCompletionForResult(function *ssa.Function, predicate CompletionPredicate, request CompletionRequest) CompletionProof {
	unknown := CompletionProof{Proof{State: EvidenceUnknown, Reason: EvidenceUnavailable}}
	parameter, ok := request.Target.(*ssa.Parameter)
	if !ok || function == nil || parameter.Parent() != function || len(function.Blocks) == 0 ||
		!predicate.valid(function.Signature) || request.InvokeTarget && len(request.Methods) != 0 {
		return unknown
	}
	methods := request.Methods
	if request.InvokeTarget {
		methods = []string{""}
	}
	for _, method := range methods {
		search := newCompletionSearch(method, CoverageEveryReturn, request.Budget)
		search.exactTarget, search.exactInvocation, search.invokeTarget = true, request.InvokeTarget, request.InvokeTarget
		search.summarized = request.Summarized
		condition := completionCondition{result: predicate.Result, kind: completionConditionKind(predicate.Outcome)}
		proven := false
		search.memo.WithFunction(function, func() {
			proven = search.conditionalCoverage(function, []mappedLocal{{local: parameter, kind: localExact}}, parameter, condition)
		})
		if proven && !request.Budget.Exhausted() {
			return CompletionProof{Proof{
				State: EvidenceProven, Reason: EvidenceCalledCompletion, Method: method, Provenance: EvidenceFromLocalSSA,
			}}
		}
	}
	return unknown
}

func (predicate CompletionPredicate) valid(signature *types.Signature) bool {
	if signature == nil || predicate.Result < 0 || predicate.Result >= signature.Results().Len() {
		return false
	}
	result := signature.Results().At(predicate.Result).Type()
	switch predicate.Outcome {
	case CompletionWhenTrue, CompletionWhenFalse:
		basic, ok := result.Underlying().(*types.Basic)
		return ok && basic.Kind() == types.Bool
	case CompletionWhenNil, CompletionWhenNonNil:
		return syntax.IsErrorType(result)
	case CompletionAlways:
	}
	return false
}
