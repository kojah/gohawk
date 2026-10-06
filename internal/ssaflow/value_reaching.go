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
// A revisited alternative contributes no evidence: Any and Every treat it as
// false and Resolve treats it as unresolved. AnyIncludingOrigin can accept an
// independent direct witness before consulting alternatives. That keeps the folds
// conservative in the direction its callers rely on, because a proof that
// depends on a value already under proof would be circular.

// ReachingWalk carries the transparent forms and the visited set of one fold.
type ReachingWalk struct {
	forms      TransparentValueForm
	opaquePhis bool
	seen       map[ssa.Value]bool
	budget     *SearchBudget
	onRevisit  func()
}

// NewReachingWalk starts a fold that looks through forms.
func NewReachingWalk(forms TransparentValueForm) ReachingWalk {
	return ReachingWalk{forms: forms, seen: map[ssa.Value]bool{}}
}

// OpaquePhis keeps phi merges as leaves instead of examining their incoming
// alternatives. Callers whose identity proof admits only a single wrapper/load
// chain can use the shared cycle guard without widening that proof at merges.
func (walk ReachingWalk) OpaquePhis() ReachingWalk {
	walk.opaquePhis = true
	return walk
}

// Within attaches a shared allowance to value visits, including transparent
// wrappers, phi edges and revisits. Branches inherit it. A cutoff contributes
// no evidence; callers inspect availability before interpreting a false or
// unresolved result. A nil budget retains the unbounded default policy.
func (walk ReachingWalk) Within(budget *SearchBudget) ReachingWalk {
	walk.budget = budget
	return walk
}

// OnRevisit observes an origin rejected by the shared cycle guard. Callers
// composing a fold inside a memoized proof may invalidate the enclosing answer
// because a revisited origin is not a completed absence proof. The callback
// changes no fold result and propagates through recursive and sibling walks.
func (walk ReachingWalk) OnRevisit(observe func()) ReachingWalk {
	walk.onRevisit = observe
	return walk
}

func (walk ReachingWalk) revisited(value ssa.Value) bool {
	if !walk.seen[value] {
		return false
	}
	if walk.onRevisit != nil {
		walk.onRevisit()
	}
	return true
}

// Any reports whether some value reaching value satisfies leaf.
func (walk ReachingWalk) Any(value ssa.Value, leaf func(ReachingWalk, ssa.Value) bool) bool {
	return walk.any(value, nil, leaf)
}

// AnyIncludingOrigin also asks origin before expanding wrappers or phi edges,
// including before the cycle guard. A direct identity witness can therefore
// match a phi itself without relying on its alternatives. False from origin
// supplies no evidence; the ordinary reaching fold continues. The allowance is
// checked first, so an exhausted request cannot accept even a direct witness.
func (walk ReachingWalk) AnyIncludingOrigin(
	value ssa.Value, origin func(ssa.Value) bool, leaf func(ReachingWalk, ssa.Value) bool,
) bool {
	return walk.any(value, origin, leaf)
}

func (walk ReachingWalk) any(value ssa.Value, origin func(ssa.Value) bool, leaf func(ReachingWalk, ssa.Value) bool) bool {
	if !walk.budget.Spend() || value == nil {
		return false
	}
	if origin != nil && origin(value) {
		return !walk.budget.Exhausted()
	}
	if walk.revisited(value) {
		return false
	}
	walk.seen[value] = true
	if inner, ok := UnwrapTransparentValue(value, walk.forms); ok {
		return walk.any(inner, origin, leaf)
	}
	if phi, ok := value.(*ssa.Phi); ok && !walk.opaquePhis {
		for _, edge := range phi.Edges {
			if walk.any(edge, origin, leaf) {
				return true
			}
			if walk.budget.Exhausted() {
				return false
			}
		}
		return false
	}
	matched := leaf(walk, value)
	return matched && !walk.budget.Exhausted()
}

// Every reports whether every value reaching value satisfies leaf. A phi with
// no edges proves nothing, and each edge is judged with its own visited set so
// one edge's walk cannot hide evidence from a sibling.
func (walk ReachingWalk) Every(value ssa.Value, leaf func(ReachingWalk, ssa.Value) bool) bool {
	if !walk.budget.Spend() || value == nil || walk.revisited(value) {
		return false
	}
	walk.seen[value] = true
	if inner, ok := UnwrapTransparentValue(value, walk.forms); ok {
		return walk.Every(inner, leaf)
	}
	if phi, ok := value.(*ssa.Phi); ok && !walk.opaquePhis {
		return walk.EveryOf(phi.Edges, leaf)
	}
	matched := leaf(walk, value)
	return matched && !walk.budget.Exhausted()
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
	if !walk.budget.Spend() || walk.revisited(value) {
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
	if !walk.budget.Spend() || value == nil || walk.revisited(value) {
		return zero, false
	}
	walk.seen[value] = true
	if inner, ok := UnwrapTransparentValue(value, walk.forms); ok {
		return ResolveReachingValue(walk, inner, leaf, key)
	}
	phi, ok := value.(*ssa.Phi)
	if !ok || walk.opaquePhis {
		resolved, found := leaf(walk, value)
		if walk.budget.Exhausted() {
			return zero, false
		}
		return resolved, found
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
	return ReachingWalk{
		forms: walk.forms, opaquePhis: walk.opaquePhis, seen: maps.Clone(walk.seen), budget: walk.budget, onRevisit: walk.onRevisit,
	}
}
