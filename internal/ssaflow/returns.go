package ssaflow

import (
	"go/token"
	"iter"

	"github.com/kojah/gohawk/internal/syntax"

	"golang.org/x/tools/go/ssa"
)

func ReturnedValueOwnsValue(returned *ssa.Return, value ssa.Value) bool {
	return returnedValueOwnsValue(returned, value, map[ownershipPair]bool{})
}

type ownershipPair struct {
	aggregate ssa.Value
	value     ssa.Value
}

func returnedValueOwnsValue(returned *ssa.Return, value ssa.Value, seen map[ownershipPair]bool) bool {
	for _, result := range returned.Results {
		if SameValue(result, value) || aggregateStoresValue(result, value, seen) {
			return true
		}
	}
	return false
}

func aggregateStoresValue(aggregate, value ssa.Value, seen map[ownershipPair]bool) bool {
	pair := ownershipPair{aggregate: aggregate, value: value}
	if aggregate == nil || seen[pair] {
		return false
	}
	if SameValue(aggregate, value) {
		return true
	}
	seen[pair] = true
	if inner, ok := UnwrapTransparentValue(
		aggregate,
		TransparentChangeInterface|TransparentChangeType|TransparentConvert|TransparentMakeInterface,
	); ok {
		return aggregateStoresValue(inner, value, seen)
	}
	switch typed := aggregate.(type) {
	case *ssa.Call:
		if callAggregateStoresValue(typed, value, seen) {
			return true
		}
	case *ssa.Phi:
		if anyAggregateStoresValue(typed.Edges, value, seen) {
			return true
		}
	case *ssa.Slice:
		return aggregateStoresValue(typed.X, value, seen)
	case *ssa.MakeClosure:
		// A returned callback that captured the value keeps it alive and is the
		// only thing that can still release it, so the caller receives the
		// obligation with the callback.
		for _, binding := range typed.Bindings {
			if CapturedBindingMatches(binding, value) || aggregateStoresValue(CapturedBindingValue(binding), value, seen) {
				return true
			}
		}
	case *ssa.UnOp:
		// Copying the owner by value, as in ephemeral(*cmd), carries the same
		// process or handle state, so returning the copy transfers it. A struct
		// literal returned by value is likewise loaded from the local that
		// assembled it, so the load carries whatever that local's fields hold.
		if typed.Op == token.MUL && (SameValue(typed.X, value) || aggregateStoresValue(typed.X, value, seen)) {
			return true
		}
	}
	if _, ok := aggregate.(*ssa.Alloc); ok && addressStoresValue(aggregate, value, seen) {
		return true
	}
	return aggregateReferrersStoreValue(aggregate, value, seen)
}

func callAggregateStoresValue(call *ssa.Call, value ssa.Value, seen map[ownershipPair]bool) bool {
	common := call.Common()
	if CallMatchesSymbol(common, syntax.Builtin("append")) && anyAggregateStoresValue(common.Args, value, seen) {
		return true
	}
	callee := common.StaticCallee()
	if callee == nil {
		return false
	}
	for index, argument := range common.Args {
		if index < len(callee.Params) &&
			aggregateStoresValue(argument, value, seen) &&
			functionReturnsOwner(callee, callee.Params[index], seen) {
			return true
		}
	}
	return false
}

func anyAggregateStoresValue(aggregates []ssa.Value, value ssa.Value, seen map[ownershipPair]bool) bool {
	for _, aggregate := range aggregates {
		if aggregateStoresValue(aggregate, value, seen) {
			return true
		}
	}
	return false
}

func functionReturnsOwner(function *ssa.Function, value ssa.Value, seen map[ownershipPair]bool) bool {
	for _, block := range function.Blocks {
		for _, instruction := range block.Instrs {
			returned, ok := instruction.(*ssa.Return)
			if ok && returnedValueOwnsValue(returned, value, seen) {
				return true
			}
		}
	}
	return false
}

func aggregateReferrersStoreValue(aggregate, value ssa.Value, seen map[ownershipPair]bool) bool {
	if aggregate.Referrers() == nil {
		return false
	}
	for _, reference := range *aggregate.Referrers() {
		address, ok := reference.(ssa.Value)
		if !ok {
			continue
		}
		switch address.(type) {
		case *ssa.FieldAddr, *ssa.IndexAddr:
			if addressStoresValue(address, value, seen) || aggregateStoresValue(address, value, seen) {
				return true
			}
		}
	}
	return false
}

func addressStoresValue(address ssa.Value, value ssa.Value, seen map[ownershipPair]bool) bool {
	for stored := range StoredInto(address) {
		if SameValue(stored, value) || aggregateStoresValue(stored, value, seen) {
			return true
		}
	}
	return false
}

// StoredInto yields every value stored into address, into a field or element
// selected from it, or through a pointer loaded from it. It is the one walk
// for asking what an aggregate holds; callers supply the question about each
// stored value.
func StoredInto(address ssa.Value) iter.Seq[ssa.Value] {
	return func(yield func(ssa.Value) bool) {
		storedInto(address, yield, map[ssa.Value]bool{})
	}
}

func storedInto(address ssa.Value, yield func(ssa.Value) bool, seen map[ssa.Value]bool) bool {
	if address == nil || seen[address] || address.Referrers() == nil {
		return true
	}
	seen[address] = true
	for _, reference := range *address.Referrers() {
		switch typed := reference.(type) {
		case *ssa.Store:
			if typed.Addr == address && !yield(typed.Val) {
				return false
			}
		case *ssa.FieldAddr, *ssa.IndexAddr:
			if !storedInto(typed.(ssa.Value), yield, seen) {
				return false
			}
		case *ssa.UnOp:
			// A returned owner may contain a pointer to a callback slot rather
			// than the callback directly. Follow the load so `*owner.cancel =
			// cancel` transfers the same obligation as `owner.cancel = cancel`.
			if typed.Op == token.MUL && !storedInto(typed, yield, seen) {
				return false
			}
		}
	}
	return true
}
