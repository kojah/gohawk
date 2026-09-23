package ssaflow

// A proof asks many bounded questions, and a bound on each question is not
// a bound on the proof: a candidate in a large function can ask thousands
// of them. A pool is the candidate's total. A budget drawn from it keeps
// its own per-question limit and also charges the pool, so the proof stops
// when either runs out, and every give-up still reaches the observer the
// pool was given. Exhaustion means what it means for the question being
// asked; the pool decides nothing about polarity.

// Within returns a budget limited to limit instructions that also charges
// every instruction to this pool. A nil pool yields a plain budget. The
// child inherits the pool's observer, so an analyzer attaches the tracer
// once, to the pool.
func (pool *SearchBudget) Within(limit int) *SearchBudget {
	child := NewSearchBudget(limit)
	if pool == nil {
		return child
	}
	child.parent = pool
	child.observer = pool.observer
	return child
}

// PoolExhausted reports whether the pool this budget draws from ran out,
// as opposed to the budget's own limit. A proof reports the difference so
// a trace distinguishes an expensive question from an expensive candidate.
func (budget *SearchBudget) PoolExhausted() bool {
	return budget != nil && budget.parent != nil && budget.parent.exhausted
}
