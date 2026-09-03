package ssaflow

import "golang.org/x/tools/go/ssa"

// A recursive walk over the call graph needs a cycle guard, and the usual one
// marks a function on entry and un-marks it on the way out. That guard is
// scoped to the current path, so on its own it makes the walk enumerate call
// paths rather than the call graph: a helper reachable by N paths is re-walked
// N times, which is exponential in a mutually recursive package.
//
// Pairing the guard with a memo fixes the cost, but not naively. An answer the
// guard cut short holds only for the path that produced it, because a
// different path may reach the same question without the cycle and prove more.
// Caching such an answer would silently change which proofs succeed. The two
// rules therefore travel together, and CallGraphMemo owns both so that a
// caller cannot honor one and forget the other.
//
// What the memo deliberately does not own is evidence policy: the caller
// chooses the key that identifies its question, and the answer a cut returns.
// Those differ by proof and belong beside the analyzer.

// CallGraphMemo answers a recursive call-graph question once per distinct key
// rather than once per call path that reaches it.
type CallGraphMemo[Key comparable, Answer any] struct {
	entered map[*ssa.Function]bool
	answers map[Key]Answer
	// cut records that the answer being computed was shortened by the cycle
	// guard, which makes it valid only on the path that produced it.
	cut bool
}

func NewCallGraphMemo[Key comparable, Answer any]() *CallGraphMemo[Key, Answer] {
	return &CallGraphMemo[Key, Answer]{
		entered: map[*ssa.Function]bool{},
		answers: map[Key]Answer{},
	}
}

// Answer returns the memoized answer for key, computing it once. An answer
// that the cycle guard cut short is returned to this caller but not retained,
// so a later path that reaches the same question without the cycle can still
// prove more.
func (memo *CallGraphMemo[Key, Answer]) Answer(key Key, compute func() Answer) Answer {
	if answer, ok := memo.answers[key]; ok {
		return answer
	}
	// The flag is per computation, not per memo: a nested cut must not stop
	// an enclosing answer from being retained unless it also reached the cut.
	outer := memo.cut
	memo.cut = false
	answer := compute()
	if !memo.cut {
		memo.answers[key] = answer
	}
	memo.cut = outer || memo.cut
	return answer
}

// Enter marks function as being on the current path. It reports false when the
// function is already on that path, which is the cycle; the caller then returns
// its own conservative answer, and Enter has recorded that the answer is cut.
// A true result must be paired with a deferred Leave. Enter reports rather than
// returning a cleanup closure because it sits on the walk's hot path, where an
// allocation per visited function is exactly the cost the memo exists to avoid.
func (memo *CallGraphMemo[Key, Answer]) Enter(function *ssa.Function) bool {
	if function == nil || memo.entered[function] {
		memo.cut = true
		return false
	}
	memo.entered[function] = true
	return true
}

// Entered reports whether function is already on the current path, for a
// caller that must answer a cycle differently from the value Enter implies.
// Such a caller records the shortened answer with Cut.
func (memo *CallGraphMemo[Key, Answer]) Entered(function *ssa.Function) bool {
	return memo.entered[function]
}

// Leave un-marks function as the walk returns past it.
func (memo *CallGraphMemo[Key, Answer]) Leave(function *ssa.Function) {
	delete(memo.entered, function)
}

// Cut records that an answer was shortened for a reason of the caller's own,
// such as an exhausted budget, so that answer is not retained either.
func (memo *CallGraphMemo[Key, Answer]) Cut() {
	memo.cut = true
}
