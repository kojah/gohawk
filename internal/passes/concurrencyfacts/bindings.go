package concurrencyfacts

import (
	"go/token"
	"go/types"

	"github.com/kojah/gohawk/internal/ssaflow"

	"golang.org/x/tools/go/ssa"
)

// Binding a Summary preserves the time at which a resource was read. Captured
// cells additionally need stable contents because the worker may read them
// after the launch. A matching access path alone cannot justify this identity.
func (engine *Engine) instantiate(instruction ssa.CallInstruction) Summary {
	if function := instruction.Common().StaticCallee(); function != nil && len(function.Blocks) == 0 {
		return engine.importedCall(instruction, function)
	}
	return engine.summaries.AtCall(instruction, engine.budget, func(callee Summary, bindings []ssaflow.CallBinding) Summary {
		return engine.bindSummary(callee, bindings, instruction)
	})
}

func (engine *Engine) bindSummary(callee Summary, bindings []ssaflow.CallBinding, instruction ssa.CallInstruction) Summary {
	if !callee.Complete() && callee.Reason != "protocol-select-alternatives" {
		return callee
	}
	result := Summary{
		Operations: make([]Operation, 0, len(callee.Operations)), Reason: callee.Reason,
		AlternativesComplete: callee.AlternativesComplete,
	}
	for _, op := range callee.Operations {
		if !engine.budget.Spend() {
			return Summary{Reason: "protocol-budget-exhausted"}
		}
		resource, ok := engine.bind(op.Resource, bindings, instruction)
		if !ok {
			return Summary{Reason: "protocol-channel-binding-unknown"}
		}
		op.Resource, op.Site = resource, instruction.Pos()
		result.Operations = append(result.Operations, op)
	}
	for _, choice := range callee.Choices {
		// A select's arm sequence is a separate complete path, not another
		// unconditional effect. Bind every resource on every arm before the
		// caller may use any variant as a graph proof.
		bound := SelectChoice{Prefix: choice.Prefix, Site: choice.Site}
		for _, arm := range choice.Arms {
			if !engine.budget.Spend() {
				return Summary{Reason: "protocol-budget-exhausted"}
			}
			if !arm.Default {
				resource, ok := engine.bind(arm.Operation.Resource, bindings, instruction)
				if !ok {
					return Summary{Reason: "protocol-channel-binding-unknown"}
				}
				arm.Operation.Resource, arm.Operation.Site = resource, instruction.Pos()
			}
			if arm.Complete {
				// Keep the callee's source location but attribute a bound action
				// to this call site, as with the linear effect sequence above.
				sequence := make([]Operation, 0, len(arm.Sequence))
				for _, op := range arm.Sequence {
					if !engine.budget.Spend() {
						return Summary{Reason: "protocol-budget-exhausted"}
					}
					resource, ok := engine.bind(op.Resource, bindings, instruction)
					if !ok {
						return Summary{Reason: "protocol-channel-binding-unknown"}
					}
					op.Resource, op.Site = resource, instruction.Pos()
					sequence = append(sequence, op)
				}
				arm.Sequence = sequence
			}
			bound.Arms = append(bound.Arms, arm)
		}
		result.Choices = append(result.Choices, bound)
	}
	return result
}

func (engine *Engine) bind(
	reference Reference, bindings []ssaflow.CallBinding, instruction ssa.Instruction,
) (Reference, bool) {
	for _, binding := range bindings {
		if binding.Local != reference.Value {
			continue
		}
		if !reference.Indirect {
			return engine.reference(binding.Supplied)
		}
		if _, captured := binding.Supplied.(*ssa.FreeVar); captured {
			return Reference{Value: binding.Supplied, Indirect: true}, true
		}
		content := engine.storage.StableContent(binding.Supplied, instruction)
		if content.Proven() {
			return engine.reference(content.Value)
		}
		return Reference{}, false
	}
	return engine.bindField(reference, bindings, instruction)
}

func (engine *Engine) reference(value ssa.Value) (Reference, bool) {
	if value == nil || !ssaflow.ChannelType(value) && !synchronizationPointer(value.Type()) {
		return Reference{}, false
	}
	return ssaflow.ResolveReachingValue(ssaflow.NewReachingWalk(ssaflow.TransparentChangeType), value,
		engine.referenceLeaf, func(reference Reference) Reference { return reference })
}

func (engine *Engine) referenceLeaf(_ ssaflow.ReachingWalk, value ssa.Value) (Reference, bool) {
	resolved := engine.storage.Resolve(value)
	if resolved.Proven() {
		switch value := resolved.Value.(type) {
		case *ssa.Call:
			return Reference{Value: value}, ssaflow.CallMatchesSymbol(value.Common(), newCond)
		case *ssa.Parameter, *ssa.FreeVar, *ssa.MakeChan:
			return Reference{Value: resolved.Value}, true
		case *ssa.Alloc:
			if synchronizationPointer(resolved.Value.Type()) {
				return Reference{Value: resolved.Value}, true
			}
		case *ssa.Global, *ssa.FieldAddr:
			// Mutex addresses identify cells, not mutable contents. Such values
			// can bind formal mutex parameters but cannot be exported as formals.
			if MutexPointer(resolved.Value.Type()) {
				if path, ok := embeddedPath(resolved.Value); ok && path.Depth > 0 {
					value, found := engine.fieldAddress(resolved.Value.Parent(), path)
					return Reference{Value: value}, found
				}
				return Reference{Value: resolved.Value}, true
			}
		}
	}
	// A symbolic captured cell is resolved at its caller, where its local
	// allocation and all writes are visible to the shared storage query.
	load, ok := value.(*ssa.UnOp)
	if ok && load.Op == token.MUL {
		if capture, ok := load.X.(*ssa.FreeVar); ok && readableAddress(capture) {
			return Reference{Value: capture, Indirect: true}, true
		}
	}
	return Reference{}, false
}

func readableAddress(value ssa.Value) bool {
	if localAddress(value) {
		return true
	}
	if _, ok := value.(*ssa.FreeVar); !ok {
		return false
	}
	pointer, ok := value.Type().Underlying().(*types.Pointer)
	if !ok {
		return false
	}
	_, channel := pointer.Elem().Underlying().(*types.Chan)
	return channel || synchronizationPointer(pointer.Elem())
}

func localAddress(value ssa.Value) bool {
	return ssaflow.NewReachingWalk(ssaflow.TransparentNone).Every(value, localAddressLeaf)
}

func localAddressLeaf(walk ssaflow.ReachingWalk, value ssa.Value) bool {
	switch value := value.(type) {
	case *ssa.Alloc:
		return true
	case *ssa.FieldAddr:
		return walk.Every(value.X, localAddressLeaf)
	default:
		return false
	}
}
