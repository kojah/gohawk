package ssautil

import (
	"go/token"

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
	switch typed := aggregate.(type) {
	case *ssa.Call:
		common := typed.Common()
		if CallName(common) == "append" {
			for _, argument := range common.Args {
				if aggregateStoresValue(argument, value, seen) {
					return true
				}
			}
		}
		callee := common.StaticCallee()
		if callee != nil {
			for index, argument := range common.Args {
				if index >= len(callee.Params) || !aggregateStoresValue(argument, value, seen) {
					continue
				}
				for _, block := range callee.Blocks {
					for _, candidate := range block.Instrs {
						if returned, ok := candidate.(*ssa.Return); ok && returnedValueOwnsValue(returned, callee.Params[index], seen) {
							return true
						}
					}
				}
			}
		}
	case *ssa.ChangeInterface:
		return aggregateStoresValue(typed.X, value, seen)
	case *ssa.ChangeType:
		return aggregateStoresValue(typed.X, value, seen)
	case *ssa.Convert:
		return aggregateStoresValue(typed.X, value, seen)
	case *ssa.MakeInterface:
		return aggregateStoresValue(typed.X, value, seen)
	case *ssa.Phi:
		for _, edge := range typed.Edges {
			if aggregateStoresValue(edge, value, seen) {
				return true
			}
		}
	case *ssa.Slice:
		return aggregateStoresValue(typed.X, value, seen)
	}
	if _, ok := aggregate.(*ssa.Alloc); ok && addressStoresValue(aggregate, value, seen) {
		return true
	}
	if aggregate.Referrers() == nil {
		return false
	}
	for _, reference := range *aggregate.Referrers() {
		switch typed := reference.(type) {
		case *ssa.FieldAddr:
			if addressStoresValue(typed, value, seen) || aggregateStoresValue(typed, value, seen) {
				return true
			}
		case *ssa.IndexAddr:
			if addressStoresValue(typed, value, seen) || aggregateStoresValue(typed, value, seen) {
				return true
			}
		}
	}
	return false
}

func addressStoresValue(address ssa.Value, value ssa.Value, seen map[ownershipPair]bool) bool {
	if address.Referrers() == nil {
		return false
	}
	for _, reference := range *address.Referrers() {
		switch typed := reference.(type) {
		case *ssa.Store:
			if typed.Addr == address && (SameValue(typed.Val, value) || aggregateStoresValue(typed.Val, value, seen)) {
				return true
			}
		case *ssa.UnOp:
			// A returned owner may contain a pointer to a callback slot rather
			// than the callback directly. Follow the load so `*owner.cancel =
			// cancel` transfers the same obligation as `owner.cancel = cancel`.
			if typed.Op == token.MUL && addressStoresValue(typed, value, seen) {
				return true
			}
		}
	}
	return false
}
