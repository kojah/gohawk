package ssaflow

import (
	"go/token"

	"golang.org/x/tools/go/ssa"
)

// Function summaries compose analyzer-defined evidence, not a universal effect
// language. An ordered protocol and a set of lock-acquisition witnesses need
// different merge rules. This layer owns their common computation boundary:
// visible bodies, recursion, memo lifetime, budgets, and direct call bindings.

// SummaryUnavailable identifies why a function summary could not be computed.
type SummaryUnavailable uint8

const (
	// SummaryBodyUnavailable means dispatch or the callee body is opaque.
	SummaryBodyUnavailable SummaryUnavailable = iota
	// SummaryRecursive means the callee is already on the active call path.
	SummaryRecursive
	// SummaryBudgetExhausted means the query exceeded its shared work budget.
	SummaryBudgetExhausted
)

// evidence names the give-up for observers and proofs.
func (reason SummaryUnavailable) evidence() EvidenceReason {
	switch reason {
	case SummaryRecursive:
		return EvidenceSummaryRecursive
	case SummaryBudgetExhausted:
		return EvidenceBudgetExhausted
	case SummaryBodyUnavailable:
	}
	return EvidenceSummaryBodyUnavailable
}

// observeUnavailable reports a summary give-up to the budget's observer with
// the function it concerned. It reports nothing when nobody is listening.
func observeUnavailable(budget *SearchBudget, reason SummaryUnavailable, function *ssa.Function, at ssa.Instruction) {
	var position token.Pos
	if at != nil {
		position = at.Pos()
	} else if function != nil {
		position = function.Pos()
	}
	budget.Observe(reason.evidence(), position, func() map[string]string {
		details := map[string]string{}
		if function != nil {
			details["function"] = function.String()
		}
		if at != nil {
			details["instruction"] = at.String()
		}
		return details
	})
}

// FunctionSummaries computes context-independent summaries for one fixed
// analysis policy and SSA program. Construct a separate instance for each
// policy; caller-specific arguments belong in AtCall's binding step, not in
// the cached computation. It is not safe for concurrent use.
//
// Answers, including their slices and maps, are immutable after publication.
// Recursive or budget-shortened answers are never cached. An analyzer may
// retain independent positive witnesses after a recursive cut, but must not
// interpret missing witnesses as proof that an effect is absent.
type FunctionSummaries[Summary any] struct {
	memo        *CallGraphMemo[*ssa.Function, Summary]
	compute     func(*ssa.Function, *SearchBudget) Summary
	unavailable func(SummaryUnavailable) Summary
}

// NewFunctionSummaries fixes the computation and conservative fallback for a
// summary family. Both callbacks must be non-nil. compute must charge the
// supplied budget for each examined instruction and each expanded effect,
// and pass that same budget to nested queries and storage proofs. A nil
// budget is unbounded, following SearchBudget's contract.
func NewFunctionSummaries[Summary any](
	compute func(*ssa.Function, *SearchBudget) Summary,
	unavailable func(SummaryUnavailable) Summary,
) *FunctionSummaries[Summary] {
	return &FunctionSummaries[Summary]{
		memo: NewCallGraphMemo[*ssa.Function, Summary](), compute: compute, unavailable: unavailable,
	}
}

// Function returns the symbolic summary of function. Completed answers can be
// reused with a fresh budget; exhaustion during a computation discards its
// entire answer, including any partial effects. Recursion is not a fixed-point
// solver: the analyzer's fallback determines what a cut can safely contribute.
func (summaries *FunctionSummaries[Summary]) Function(function *ssa.Function, budget *SearchBudget) Summary {
	return summaries.memo.Summarize(function, function, budget, func() Summary {
		return summaries.compute(function, budget)
	}, func(reason SummaryUnavailable, _ Summary) Summary {
		return summaries.unavailable(reason)
	})
}

// Summarize composes one context-keyed question about a function body. key
// must include every varying input to compute (target, mode, callback bindings,
// and so on). Use FunctionSummaries when function identity is the entire key.
// The memo, recursion guard, and budget invalidation are shared by both forms.
//
// unavailable selects the evidence polarity at a boundary. Its partial answer
// is nonzero only when compute exhausted budget. It may preserve independent
// positive witnesses, but must mark incomplete evidence or otherwise refuse
// to establish absence. Cut answers and all dependent answers are not cached.
// compute charges budget for its work; nil leaves existing unbounded queries
// unchanged. Answers are immutable after publication, and use is sequential.
func (memo *CallGraphMemo[Key, Answer]) Summarize(
	key Key, function *ssa.Function, budget *SearchBudget,
	compute func() Answer, unavailable func(SummaryUnavailable, Answer) Answer,
) Answer {
	var empty Answer
	if function == nil || len(function.Blocks) == 0 {
		observeUnavailable(budget, SummaryBodyUnavailable, function, nil)
		return unavailable(SummaryBodyUnavailable, empty)
	}
	return memo.Compose(key, budget, func() Answer {
		var answer Answer
		if !memo.WithFunction(function, func() { answer = compute() }) {
			observeUnavailable(budget, SummaryRecursive, function, nil)
			return unavailable(SummaryRecursive, empty)
		}
		return answer
	}, unavailable)
}

// Compose memoizes a context-keyed question that may inspect multiple function
// bodies. Unlike Summarize, the cache key does not name one guarded body:
// compute enters each callee through WithFunction. Recursive and budget cuts
// invalidate every dependent answer, while independent completed answers remain
// reusable. Budget, immutability, and fallback contracts match Summarize.
func (memo *CallGraphMemo[Key, Answer]) Compose(
	key Key, budget *SearchBudget, compute func() Answer, unavailable func(SummaryUnavailable, Answer) Answer,
) Answer {
	var empty Answer
	if budget.Exhausted() {
		memo.Cut()
		observeUnavailable(budget, SummaryBudgetExhausted, nil, nil)
		return unavailable(SummaryBudgetExhausted, empty)
	}
	return memo.Answer(key, func() Answer {
		answer := compute()
		if budget.Exhausted() {
			memo.Cut()
			observeUnavailable(budget, SummaryBudgetExhausted, nil, nil)
			return unavailable(SummaryBudgetExhausted, answer)
		}
		return answer
	})
}

// WithFunction executes visit inside one callee's recursion scope and always
// releases that scope on return. It returns false without visiting an opaque
// body or an active callee. A recursive rejection also invalidates dependent
// summaries. The caller chooses the conservative meaning of a rejected visit;
// this operation does not cache an answer or reset the enclosing work budget.
func (memo *CallGraphMemo[Key, Answer]) WithFunction(function *ssa.Function, visit func()) bool {
	if function == nil || len(function.Blocks) == 0 || !memo.Enter(function) {
		return false
	}
	defer memo.Leave(function)
	visit()
	return true
}

// Incomplete reports a policy-specific truncation, such as a revisited callback
// value outside the function recursion guard. The current computation and its
// dependents are not cached. It does not choose an answer: the evidence policy
// must still distinguish partial positive witnesses from proof of absence.
// Ordinary budget exhaustion and function recursion are handled automatically.
func (memo *CallGraphMemo[Key, Answer]) Incomplete() {
	memo.Cut()
}

// AtCall resolves a direct function or closure, obtains its symbolic summary,
// and supplies parameter/capture bindings to instantiate it. bind must return
// a fresh answer without mutating cached evidence, preserve unavailable
// answers, and charge budget for its work. It owns identity, captured-cell
// stability, provenance, and effect semantics. Binding failures are local to
// this invocation and never replace the cached symbolic summary.
func (summaries *FunctionSummaries[Summary]) AtCall(
	instruction ssa.CallInstruction,
	budget *SearchBudget,
	bind func(Summary, []CallBinding) Summary,
) Summary {
	function, closure := DirectCallee(instruction.Common())
	if function == nil || len(function.Blocks) == 0 {
		observeUnavailable(budget, SummaryBodyUnavailable, function, instruction)
		return summaries.unavailable(SummaryBodyUnavailable)
	}
	answer := summaries.Function(function, budget)
	result := bind(answer, CallBindings(instruction.Common(), function, closure))
	if budget.Exhausted() {
		summaries.memo.Cut()
		observeUnavailable(budget, SummaryBudgetExhausted, function, instruction)
		return summaries.unavailable(SummaryBudgetExhausted)
	}
	return result
}
