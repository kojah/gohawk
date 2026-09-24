package heapmodel

import (
	"github.com/kojah/gohawk/internal/ssaflow"
	"golang.org/x/tools/go/ssa"
)

// Possible identity, answered by the points-to graph where one is available
// and by the structural value walk otherwise. Both keep the same contract:
// two objects are the same only when the function's own flow connects
// them.

// MayAlias reports a possible identity through conversions, any phi edge,
// and local storage. It is not a must-alias proof: use DefinitelySameValue
// when a diagnostic or guaranteed action requires exact identity. It does
// not equate a field or index with its containing aggregate; use
// ValueDerivesFrom or MayContainValue for containment instead. It is the
// Boolean of ProveMayAlias, which is the one decision path.
func MayAlias(value, target ssa.Value) bool {
	return ProveMayAlias(value, target).Aliases
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

// DefinitelySameValue proves value identity: the values name one object
// on every path. The value walk proves it for one SSA value seen through
// wrappers, a phi whose alternatives all agree, and equal address
// selections; the points-to graph adds a cell resolved through a copy of
// its pointee, a join where every path stored one object, and two reads of
// one slot with no write between them. A false result means unproved, not
// necessarily different.
func DefinitelySameValue(left, right ssa.Value) bool {
	if ssaflow.StructurallyIdentical(left, right) {
		return true
	}
	return DefinitelySame(left, right)
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
