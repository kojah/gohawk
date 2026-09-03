package ssaflow

import (
	"maps"

	"golang.org/x/tools/go/ssa"
)

// Reaching-value folds own the recursion that analyzers need when they ask a
// question of every value that may flow into an SSA value: the visited set,
// the walk through the transparent wrapper forms a caller selected, and the
// fan-out over phi merges. The folds are policy-free. Each caller keeps its
// own leaf predicate and its own form set, and a leaf that wants to follow an
// operand the fold does not peel (a load, a tuple extraction, a call result)
// recurses through the walk it is handed so cycles stay bounded.
//
// A revisited value contributes no evidence in any fold: Any and Every treat
// it as false and Resolve treats it as unresolved. That keeps every fold
// conservative in the direction its callers rely on, because a proof that
// depends on a value already under proof would be circular.

// ReachingWalk carries the transparent forms and the visited set of one fold.
type ReachingWalk struct {
	forms TransparentValueForm
	seen  map[ssa.Value]bool
}

// NewReachingWalk starts a fold that looks through forms.
func NewReachingWalk(forms TransparentValueForm) ReachingWalk {
	return ReachingWalk{forms: forms, seen: map[ssa.Value]bool{}}
}

// Any reports whether some value reaching value satisfies leaf.
func (walk ReachingWalk) Any(value ssa.Value, leaf func(ReachingWalk, ssa.Value) bool) bool {
	if value == nil || walk.seen[value] {
		return false
	}
	walk.seen[value] = true
	if inner, ok := UnwrapTransparentValue(value, walk.forms); ok {
		return walk.Any(inner, leaf)
	}
	if phi, ok := value.(*ssa.Phi); ok {
		for _, edge := range phi.Edges {
			if walk.Any(edge, leaf) {
				return true
			}
		}
		return false
	}
	return leaf(walk, value)
}

// Every reports whether every value reaching value satisfies leaf. A phi with
// no edges proves nothing, and each edge is judged with its own visited set so
// one edge's walk cannot hide evidence from a sibling.
func (walk ReachingWalk) Every(value ssa.Value, leaf func(ReachingWalk, ssa.Value) bool) bool {
	if value == nil || walk.seen[value] {
		return false
	}
	walk.seen[value] = true
	if inner, ok := UnwrapTransparentValue(value, walk.forms); ok {
		return walk.Every(inner, leaf)
	}
	if phi, ok := value.(*ssa.Phi); ok {
		return walk.EveryOf(phi.Edges, leaf)
	}
	return leaf(walk, value)
}

// EveryOf reports whether every value in values satisfies leaf, judging each
// under its own visited set. No values proves nothing.
func (walk ReachingWalk) EveryOf(values []ssa.Value, leaf func(ReachingWalk, ssa.Value) bool) bool {
	if len(values) == 0 {
		return false
	}
	for _, value := range values {
		if !walk.branch().Every(value, leaf) {
			return false
		}
	}
	return true
}

// Mark records value as visited and reports whether this was its first visit.
// Leaves use it for values they examine without folding over them, such as
// the sibling element addresses of one slice.
func (walk ReachingWalk) Mark(value ssa.Value) bool {
	if walk.seen[value] {
		return false
	}
	walk.seen[value] = true
	return true
}

// ResolveReachingValue returns the one result that every value reaching value
// resolves to under leaf, where results agree when key maps them to the same
// key. Edges of a phi that resolve to different keys, or an edge that does not
// resolve at all, leave the value unresolved; the result of the last edge is
// returned for an agreed key.
func ResolveReachingValue[T any, K comparable](
	walk ReachingWalk,
	value ssa.Value,
	leaf func(ReachingWalk, ssa.Value) (T, bool),
	key func(T) K,
) (T, bool) {
	var zero T
	if value == nil || walk.seen[value] {
		return zero, false
	}
	walk.seen[value] = true
	if inner, ok := UnwrapTransparentValue(value, walk.forms); ok {
		return ResolveReachingValue(walk, inner, leaf, key)
	}
	phi, ok := value.(*ssa.Phi)
	if !ok {
		return leaf(walk, value)
	}
	if len(phi.Edges) == 0 {
		return zero, false
	}
	var resolved T
	var agreed K
	for index, edge := range phi.Edges {
		candidate, ok := ResolveReachingValue(walk.branch(), edge, leaf, key)
		if !ok || index > 0 && key(candidate) != agreed {
			return zero, false
		}
		resolved, agreed = candidate, key(candidate)
	}
	return resolved, true
}

// branch copies the visited set so sibling phi edges are judged independently.
func (walk ReachingWalk) branch() ReachingWalk {
	return ReachingWalk{forms: walk.forms, seen: maps.Clone(walk.seen)}
}
