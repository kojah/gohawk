package ssaflow

import "golang.org/x/tools/go/ssa"

// Possible identity, answered by the points-to graph where one is available
// and by the structural value walk otherwise. Both keep the same contract:
// two objects are the same only when the function's own flow connects
// them.

// MayAlias reports a possible identity through conversions, any phi edge,
// and local storage. It is not a must-alias proof: use DefinitelySameValue
// when a diagnostic or guaranteed action requires exact identity. It does
// not equate a field or index with its containing aggregate; use
// ValueDerivesFrom or MayContainValue for containment instead.
//
// The points-to graph answers with disjointness the value walk cannot: two
// fields of one object, a cell after it was overwritten, or an unescaped
// local and anything it was never stored into, are not aliases. A function
// the graph could not model keeps the walk, which never rules an alias out.
func MayAlias(value, target ssa.Value) bool {
	if graph := regionsOf(value); graph.available && valueFunction(target) == graph.function {
		return graph.mayAlias(value, target)
	}
	return structurallySame(value, target)
}

// CapturedBindingMatches reports whether a closure binding directly contains
// target or refers to an addressable local that has contained target. Unlike
// CapturedBindingValue, it handles variables reassigned before a callback is
// installed without depending on referrer iteration order.
func CapturedBindingMatches(binding, target ssa.Value) bool {
	if MayAlias(binding, target) {
		return true
	}
	if binding == nil || binding.Referrers() == nil {
		return false
	}
	for _, reference := range *binding.Referrers() {
		store, ok := reference.(*ssa.Store)
		if ok && store.Addr == binding && MayAlias(store.Val, target) {
			return true
		}
	}
	return false
}

// MayAliasAny reports whether value may alias any candidate; see MayAlias.
func MayAliasAny(value ssa.Value, candidates []ssa.Value) bool {
	for _, candidate := range candidates {
		if MayAlias(value, candidate) {
			return true
		}
	}
	return false
}

// ReturnedMayAliasAny reports whether a return may transfer any candidate value.
func ReturnedMayAliasAny(returned *ssa.Return, candidates []ssa.Value) bool {
	for _, result := range returned.Results {
		if MayAliasAny(result, candidates) {
			return true
		}
	}
	return false
}
