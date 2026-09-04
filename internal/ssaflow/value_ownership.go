package ssaflow

import (
	"go/token"

	"golang.org/x/tools/go/ssa"
)

// Ownership provenance walks backward: given a value, which storage does it
// come out of, and does that storage outlive this call? It is the mirror of
// the forward question in store_escape.go, which asks where a value a function
// holds ends up. The two share a vocabulary and answer opposite questions, so
// they are kept apart: a proof that follows provenance must not accidentally
// pick up a rule about escape.

// ExternallyOwnedValue reports whether value comes from storage that outlives
// the current function invocation.
func ExternallyOwnedValue(value ssa.Value) bool {
	return externallyOwnedValue(value, map[ssa.Value]bool{})
}

func externallyOwnedValue(value ssa.Value, seen map[ssa.Value]bool) bool {
	if value == nil || seen[value] {
		return false
	}
	seen[value] = true
	if inner, ok := UnwrapTransparentValue(
		value,
		TransparentChangeInterface|TransparentChangeType|TransparentConvert|TransparentMakeInterface,
	); ok {
		return externallyOwnedValue(inner, seen)
	}
	if source, ok := ownershipSource(value); ok {
		return externallyOwnedValue(source, seen)
	}
	switch typed := value.(type) {
	case *ssa.Parameter, *ssa.FreeVar, *ssa.Global:
		return true
	case *ssa.Alloc:
		if typed.Referrers() != nil {
			for _, reference := range *typed.Referrers() {
				if store, ok := reference.(*ssa.Store); ok && store.Addr == typed && externallyOwnedValue(store.Val, seen) {
					return true
				}
			}
		}
	case *ssa.Phi:
		for _, edge := range typed.Edges {
			if externallyOwnedValue(edge, seen) {
				return true
			}
		}
	}
	return false
}

// ElementOfAggregate reports whether value was selected from an element of
// a slice, array, map, or range iteration, directly or through fields and
// loads. Such a value is shared with whatever else holds the aggregate, so a
// lifecycle obligation attached to it cannot be attributed to the current
// function alone; callers treat it as unknown rather than violated.
func ElementOfAggregate(value ssa.Value) bool {
	forms := TransparentChangeInterface | TransparentChangeType | TransparentConvert | TransparentMakeInterface
	return NewReachingWalk(forms).Any(value, aggregateElementLeaf)
}

func aggregateElementLeaf(walk ReachingWalk, value ssa.Value) bool {
	switch typed := value.(type) {
	case *ssa.IndexAddr, *ssa.Index, *ssa.Lookup:
		return true
	case *ssa.Extract:
		if _, ok := typed.Tuple.(*ssa.Next); ok {
			return true
		}
	}
	source, ok := ownershipSource(value)
	return ok && walk.Any(source, aggregateElementLeaf)
}

func ownershipSource(value ssa.Value) (ssa.Value, bool) {
	switch typed := value.(type) {
	case *ssa.FieldAddr:
		return typed.X, true
	case *ssa.Field:
		return typed.X, true
	case *ssa.IndexAddr:
		return typed.X, true
	case *ssa.Index:
		return typed.X, true
	case *ssa.UnOp:
		// A load and a receive both take a value out of something that holds
		// it, so the holder is a candidate owner. The arithmetic and logical
		// operators produce a new value from an operand and carry no such
		// claim: negating a counter does not make the counter its owner.
		return typed.X, typed.Op == token.MUL || typed.Op == token.ARROW
	case *ssa.Lookup:
		return typed.X, true
	case *ssa.Slice:
		return typed.X, true
	case *ssa.TypeAssert:
		// A type assertion denotes the same object as the interface value it
		// narrows, so a channel field read through an asserted event is owned
		// by whoever owns the event.
		return typed.X, true
	case *ssa.Extract:
		return typed.Tuple, true
	case *ssa.Next:
		// An element produced by ranging over a map or string belongs to the
		// aggregate being ranged, so a channel taken from a receiver's map is
		// owned by the receiver. policyserv closes such channels from a
		// goroutine in its pubsub Close:
		// https://github.com/matrix-org/policyserv/blob/16995e82a8518f66c5bdc5b6d6d3be04a7640f33/pubsub/psql.go#L62-L72
		return typed.Iter, true
	case *ssa.Range:
		return typed.X, true
	default:
		return nil, false
	}
}
