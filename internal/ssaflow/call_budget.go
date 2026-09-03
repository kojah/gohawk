package ssaflow

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

// SearchBudget bounds one interprocedural question by the number of
// instructions it may examine.
type SearchBudget struct {
	remaining int
	exhausted bool
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
	budget.remaining--
	return true
}

// Exhausted reports whether the budget ran out, so a caller can trace the
// bailout and decline to retain an answer that was cut short.
func (budget *SearchBudget) Exhausted() bool {
	return budget != nil && budget.exhausted
}
