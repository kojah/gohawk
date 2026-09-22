package ssaflow

import "golang.org/x/tools/go/ssa"

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
		return unavailable(SummaryBodyUnavailable, empty)
	}
	if budget.Exhausted() {
		memo.Cut()
		return unavailable(SummaryBudgetExhausted, empty)
	}
	return memo.Answer(key, func() Answer {
		if !memo.Enter(function) {
			return unavailable(SummaryRecursive, empty)
		}
		defer memo.Leave(function)
		answer := compute()
		if budget.Exhausted() {
			memo.Cut()
			return unavailable(SummaryBudgetExhausted, answer)
		}
		return answer
	})
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
		return summaries.unavailable(SummaryBodyUnavailable)
	}
	answer := summaries.Function(function, budget)
	result := bind(answer, CallBindings(instruction.Common(), function, closure))
	if budget.Exhausted() {
		summaries.memo.Cut()
		return summaries.unavailable(SummaryBudgetExhausted)
	}
	return result
}
