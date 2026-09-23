package ssaflow

import "go/token"

// An interprocedural question can be asked of a call graph too large to walk.
// Mutual recursion is the usual cause: the cycle guard keeps the walk finite,
// but a memo cannot retain an answer the guard cut short, so a densely
// recursive package is re-walked once per route rather than once per function.
// A budget bounds that work by the instructions one question may examine.
//
// The budget deliberately does not decide what exhaustion means. Whether an
// undecided question suppresses a diagnostic or merely fails to prove one
// depends on the polarity of the proof being sought, and that is the caller's
// policy: a walk that claims an obligation was met must not claim it on a
// guess, while a walk that claims an obligation remains open must not invent
// one. Callers ask Spend before each step and choose their own answer when it
// reports false.

// The two shared bounds name how much one question may cost. They are the
// defaults a caller reaches for when it has no reason of its own; a caller
// with one, such as a whole-package caller-set walk, declares a named
// constant beside the proof that explains it. A bare number at a
// construction site is not a decision, so the architecture tests reject it.
const (
	// QueryBudget bounds one local question: a storage identity, a
	// projection, a value's uses, or one callee walked for a completion. It
	// is also what NewStorage and NewCallEffects assume for a nil budget.
	QueryBudget = 1000
	// SummaryBudget bounds a question that consults or computes a function
	// summary, or decides feasibility from one: twice a local question,
	// because it walks the callee as well as the caller.
	SummaryBudget = 2000
)

// SearchBudget bounds one interprocedural question by the number of
// instructions it may examine.
type SearchBudget struct {
	remaining int
	exhausted bool
	observer  Observer
	// parent, when set, is the candidate-wide pool this budget also charges;
	// see Within.
	parent *SearchBudget
}

// NewSearchBudget returns a budget allowing limit instructions.
func NewSearchBudget(limit int) *SearchBudget {
	return &SearchBudget{remaining: limit}
}

// Spend charges one instruction and reports whether the walk may continue. A
// nil budget is unbounded, so a caller that does not need one passes nothing.
func (budget *SearchBudget) Spend() bool {
	if budget == nil {
		return true
	}
	if budget.remaining <= 0 {
		budget.exhausted = true
		return false
	}
	if budget.parent != nil && !budget.parent.Spend() {
		budget.exhausted = true
		return false
	}
	budget.remaining--
	return true
}

// Observed attaches an observer that hears each give-up of a proof spending
// this budget, and returns the budget so a query can be built inline. A nil
// observer leaves the budget silent; a nil budget stays unbounded and silent.
func (budget *SearchBudget) Observed(observer Observer) *SearchBudget {
	if budget != nil {
		budget.observer = observer
	}
	return budget
}

// observe reports one give-up. Details are built only when someone is
// listening, so a silent budget costs one nil check at the give-up point and
// nothing on the path that spends it.
func (budget *SearchBudget) observe(reason EvidenceReason, at token.Pos, build func() map[string]string) {
	if budget == nil || budget.observer == nil {
		return
	}
	var details map[string]string
	if build != nil {
		details = build()
	}
	budget.observer(string(reason), at, details)
}

// Exhausted reports whether the budget ran out, so a caller can trace the
// bailout and decline to retain an answer that was cut short.
func (budget *SearchBudget) Exhausted() bool {
	return budget != nil && budget.exhausted
}
