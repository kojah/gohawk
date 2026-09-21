package ssaflow

import (
	"go/token"

	"golang.org/x/tools/go/ssa"
)

// Walk outward from a supplied value, not backwards through possible origins.
// Every alias use must be accounted for before the absence of an effect is a
// proof. Local spills deliberately count as retention until their aliases can
// be modeled; unsupported instructions never silently become read-only.
func (query *CallEffects) uses(value ssa.Value, seen map[ssa.Value]bool) CallEffect {
	if seen[value] {
		return 0
	}
	if value.Referrers() == nil {
		return effectUnknown
	}
	seen[value] = true
	var effects CallEffect
	for _, use := range *value.Referrers() {
		if !query.budget.Spend() {
			query.memo.Cut()
			return effects | effectUnknown
		}
		effects |= query.use(value, use, seen)
	}
	return effects
}

func (query *CallEffects) use(value ssa.Value, use ssa.Instruction, seen map[ssa.Value]bool) CallEffect {
	switch use := use.(type) {
	case *ssa.DebugRef:
		return 0
	case *ssa.FieldAddr, *ssa.IndexAddr, *ssa.Slice, *ssa.ChangeType, *ssa.MakeInterface, *ssa.ChangeInterface, *ssa.Phi:
		return query.uses(use.(ssa.Value), seen)
	case *ssa.UnOp:
		if use.Op == token.MUL {
			return EffectRead
		}
		return effectUnknown
	case *ssa.BinOp:
		if use.Op == token.EQL || use.Op == token.NEQ {
			return EffectRead
		}
		return effectUnknown
	case *ssa.Store:
		var effects CallEffect
		if use.Addr == value {
			effects |= EffectMutate
		}
		if use.Val == value {
			// The new alias may later be loaded and mutated. Retention is
			// observed, but absence of other effects cannot be established.
			effects |= EffectRetain | effectUnknown
		}
		return effects
	case *ssa.Return:
		return EffectRetain
	case *ssa.Send:
		if use.X == value {
			return EffectRetain | EffectAsync | effectUnknown
		}
		return EffectMutate
	case *ssa.MapUpdate:
		if use.Value == value || use.Key == value {
			return EffectRetain | effectUnknown
		}
		return EffectMutate
	case *ssa.Call, *ssa.Defer, *ssa.Go:
		return query.call(use, value)
	case *ssa.MakeClosure:
		return query.closure(use, value)
	default:
		return effectUnknown
	}
}

func (query *CallEffects) closure(closure *ssa.MakeClosure, value ssa.Value) CallEffect {
	function, ok := closure.Fn.(*ssa.Function)
	if !ok || closure.Referrers() == nil {
		return effectUnknown
	}
	var effects CallEffect
	for _, binding := range CallBindings(nil, function, closure) {
		if binding.Supplied == value {
			effects |= query.value(binding.Local)
		}
	}
	for _, use := range *closure.Referrers() {
		if !query.budget.Spend() {
			query.memo.Cut()
			return effects | effectUnknown
		}
		common := InstructionCall(use)
		if common == nil || common.Value != closure {
			effects |= EffectRetain | effectUnknown
			continue
		}
		if _, launched := use.(*ssa.Go); launched {
			effects |= EffectAsync
		}
	}
	return effects
}
