// Package dense propagates analyzer-defined facts through control-flow points.
// It owns joining and fixed-point iteration; callers own transfer semantics,
// feasible successors, and the meaning of an incomplete result.
package dense

// State supplies an incoming fact at a control-flow point. A point may be a
// block, an edge, or a block paired with path context that must remain separate.
type State[Point comparable, Fact comparable] struct {
	Point Point
	Fact  Fact
}

// Result contains the joined incoming facts at reached points. An absent point
// is unreachable; a present zero fact is reachable. Only a complete result is
// a fixed point. Partial facts must not be used to prove absence of a path.
type Result[Point comparable, Fact comparable] struct {
	In       map[Point]Fact
	Complete bool
	Steps    int
}

// Forward propagates initial facts until no incoming fact changes or maxSteps
// transfer calls have run. A nonpositive maxSteps performs no work. Callers
// choose a finite budget, including a derived bound for finite-height domains.
//
// Join must be associative, commutative, and idempotent. Transfer must be
// monotone and return incoming facts for the feasible successors of its point.
// Both functions must treat their inputs as immutable. Equality must reflect
// fact equality, so pointers to mutable state are not suitable facts.
//
// Successors are processed in the order supplied, with one pending entry per
// point. A changed fact reschedules an already processed point, including a
// self-loop. The engine never widens facts or interprets analyzer policy.
func Forward[Point comparable, Fact comparable](
	initial []State[Point, Fact],
	join func(Fact, Fact) Fact,
	transfer func(Point, Fact) []State[Point, Fact],
	maxSteps int,
) Result[Point, Fact] {
	result := Result[Point, Fact]{In: make(map[Point]Fact)}
	pending := make(map[Point]bool)
	queue := make([]Point, 0, len(initial))
	propagate := func(state State[Point, Fact]) {
		previous, reached := result.In[state.Point]
		fact := state.Fact
		if reached {
			fact = join(previous, fact)
			if fact == previous {
				return
			}
		}
		result.In[state.Point] = fact
		if !pending[state.Point] {
			pending[state.Point] = true
			queue = append(queue, state.Point)
		}
	}
	for _, state := range initial {
		propagate(state)
	}
	for len(queue) > 0 {
		if result.Steps >= maxSteps {
			return result
		}
		point := queue[0]
		queue = queue[1:]
		delete(pending, point)
		result.Steps++
		for _, state := range transfer(point, result.In[point]) {
			propagate(state)
		}
	}
	result.Complete = true
	return result
}
