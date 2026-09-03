package goroutineownership

import (
	"go/token"
	"slices"

	"github.com/kojah/gohawk/internal/ssaflow"

	"golang.org/x/tools/go/ssa"
)

// Containment evidence decides whether an SSA value is, or carries, one of the
// worker's tracked values. It is what lets a return, store, send, or opaque
// call consume a completion channel hidden inside a closure, a struct literal,
// or a wrapping call result. The walk is deliberately one-directional: it
// only ever widens what counts as consumption, so an over-approximation makes
// an instruction opaque and suppresses a diagnostic rather than inventing one.

// carryForms are the wrappers through which a carried value keeps its
// identity for containment evidence.
const carryForms = ssaflow.TransparentChangeInterface | ssaflow.TransparentChangeType | ssaflow.TransparentConvert | ssaflow.TransparentMakeInterface

// consumes reports whether value is a tracked value or something that carries
// one: a closure capturing it, an aggregate holding it, a loaded composite, or
// the result of a call that received it.
func (analysis *spawnAnalysis) consumes(value ssa.Value) bool {
	return slices.ContainsFunc(analysis.tracked, func(tracked trackedValue) bool {
		return carries(ssaflow.NewReachingWalk(carryForms), value, tracked.value)
	})
}

func carries(walk ssaflow.ReachingWalk, value, target ssa.Value) bool {
	if value != nil && ssaflow.SameValue(value, target) {
		return true
	}
	return walk.Any(value, func(walk ssaflow.ReachingWalk, value ssa.Value) bool {
		if ssaflow.SameValue(value, target) {
			return true
		}
		switch typed := value.(type) {
		case *ssa.MakeClosure:
			// A closure carries whatever it captured, including an addressable
			// local that held the target at any point.
			return slices.ContainsFunc(typed.Bindings, func(binding ssa.Value) bool {
				return ssaflow.CapturedBindingMatches(binding, target) || carries(walk, binding, target)
			})
		case *ssa.UnOp:
			// A struct passed or returned by value is loaded from the local that
			// assembled it, so the load carries what the local's fields hold.
			return typed.Op == token.MUL && carries(walk, typed.X, target)
		case *ssa.Alloc, *ssa.FieldAddr, *ssa.IndexAddr:
			return storedCarries(walk, value, target)
		case *ssa.Slice:
			return carries(walk, typed.X, target)
		case *ssa.Extract:
			return carries(walk, typed.Tuple, target)
		case *ssa.Call:
			return callResultCarries(walk, typed, target)
		}
		return false
	})
}

// callResultCarries assumes a non-builtin call retains its arguments, because
// wrapSignal(done) may return the owner that later escapes. Builtins other
// than append only observe their arguments.
func callResultCarries(walk ssaflow.ReachingWalk, call *ssa.Call, target ssa.Value) bool {
	if builtin, ok := call.Common().Value.(*ssa.Builtin); ok && builtin.Name() != "append" {
		return false
	}
	return slices.ContainsFunc(call.Common().Args, func(argument ssa.Value) bool {
		return carries(walk, argument, target)
	})
}

// storedCarries follows stores into an addressable local and into the fields
// and elements selected from it. Every `results[i]` expression is its own
// IndexAddr, and every `w.field` expression its own FieldAddr, so a read
// through one is matched against stores made through any other address of the
// same element or field; that over-approximation only ever makes an
// instruction opaque or a signal buffered.
func storedCarries(walk ssaflow.ReachingWalk, address, target ssa.Value) bool {
	if siblingSelectionCarries(walk, address, target) {
		return true
	}
	if address.Referrers() == nil {
		return false
	}
	for _, reference := range *address.Referrers() {
		switch typed := reference.(type) {
		case *ssa.Store:
			if typed.Addr == address && carries(walk, typed.Val, target) {
				return true
			}
		case *ssa.FieldAddr:
			if storedCarries(walk, typed, target) {
				return true
			}
		case *ssa.IndexAddr:
			if storedCarries(walk, typed, target) {
				return true
			}
		}
	}
	return false
}

// siblingSelectionCarries matches a read address against stores made through
// any other address selecting the same element or field of the same aggregate.
// A composite literal stores through one field address while a later read
// selects that field through another, so the store is only visible from the
// sibling.
func siblingSelectionCarries(walk ssaflow.ReachingWalk, address, target ssa.Value) bool {
	base, selectsSame := siblingSelection(address)
	if base == nil || base.Referrers() == nil {
		return false
	}
	for _, sibling := range *base.Referrers() {
		other, ok := sibling.(ssa.Value)
		if !ok || other == address || !selectsSame(sibling) || !walk.Mark(other) {
			continue
		}
		if storedCarries(walk, other, target) {
			return true
		}
	}
	return false
}

// siblingSelection returns the aggregate an address selects from and a
// predicate matching other addresses that select the same element or field.
func siblingSelection(address ssa.Value) (ssa.Value, func(ssa.Instruction) bool) {
	switch typed := address.(type) {
	case *ssa.IndexAddr:
		return typed.X, func(instruction ssa.Instruction) bool {
			_, ok := instruction.(*ssa.IndexAddr)
			return ok
		}
	case *ssa.FieldAddr:
		return typed.X, func(instruction ssa.Instruction) bool {
			other, ok := instruction.(*ssa.FieldAddr)
			return ok && other.Field == typed.Field
		}
	}
	return nil, nil
}

// bindingCarries matches a closure binding or call argument against a tracked
// value, including an addressable local that has contained it.
func bindingCarries(binding, target ssa.Value) bool {
	return ssaflow.CapturedBindingMatches(binding, target) || carries(ssaflow.NewReachingWalk(carryForms), binding, target)
}
