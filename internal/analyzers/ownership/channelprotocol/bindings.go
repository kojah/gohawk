package channelprotocol

import (
	"go/token"
	"go/types"

	"github.com/kojah/gohawk/internal/ssaflow"

	"golang.org/x/tools/go/ssa"
)

// Binding a summary preserves the time at which a resource was read. Captured
// cells additionally need stable contents because the worker may read them
// after the launch. A matching access path alone cannot justify this identity.
func (engine *summaryEngine) instantiate(instruction ssa.CallInstruction) summary {
	return engine.summaries.AtCall(instruction, engine.budget, func(callee summary, bindings []ssaflow.CallBinding) summary {
		return engine.bindSummary(callee, bindings, instruction)
	})
}

func (engine *summaryEngine) bindSummary(callee summary, bindings []ssaflow.CallBinding, instruction ssa.CallInstruction) summary {
	if callee.reason != "" {
		return callee
	}
	result := summary{operations: make([]operation, 0, len(callee.operations))}
	for _, op := range callee.operations {
		if !engine.budget.Spend() {
			return summary{reason: "protocol-budget-exhausted"}
		}
		resource, ok := engine.bind(op.resource, bindings, instruction)
		if !ok {
			return summary{reason: "protocol-channel-binding-unknown"}
		}
		op.resource, op.site = resource, instruction.Pos()
		result.operations = append(result.operations, op)
	}
	return result
}

func (engine *summaryEngine) bind(
	reference resourceReference, bindings []ssaflow.CallBinding, instruction ssa.Instruction,
) (resourceReference, bool) {
	for _, binding := range bindings {
		if binding.Local != reference.value {
			continue
		}
		if !reference.indirect {
			return engine.reference(binding.Supplied)
		}
		if _, captured := binding.Supplied.(*ssa.FreeVar); captured {
			return resourceReference{value: binding.Supplied, indirect: true}, true
		}
		content := engine.storage.StableContent(binding.Supplied, instruction)
		if content.Proven() {
			return engine.reference(content.Value)
		}
		return resourceReference{}, false
	}
	return resourceReference{}, false
}

func (engine *summaryEngine) reference(value ssa.Value) (resourceReference, bool) {
	if value == nil || !ssaflow.ChannelType(value) && !waitGroupPointer(value.Type()) {
		return resourceReference{}, false
	}
	return ssaflow.ResolveReachingValue(ssaflow.NewReachingWalk(ssaflow.TransparentChangeType), value,
		engine.referenceLeaf, func(reference resourceReference) resourceReference { return reference })
}

func (engine *summaryEngine) referenceLeaf(_ ssaflow.ReachingWalk, value ssa.Value) (resourceReference, bool) {
	resolved := engine.storage.Resolve(value)
	if resolved.Proven() {
		switch resolved.Value.(type) {
		case *ssa.Parameter, *ssa.FreeVar, *ssa.MakeChan:
			return resourceReference{value: resolved.Value}, true
		case *ssa.Alloc:
			if waitGroupPointer(resolved.Value.Type()) {
				return resourceReference{value: resolved.Value}, true
			}
		}
	}
	// A symbolic captured cell is resolved at its caller, where its local
	// allocation and all writes are visible to the shared storage query.
	load, ok := value.(*ssa.UnOp)
	if ok && load.Op == token.MUL {
		if capture, ok := load.X.(*ssa.FreeVar); ok && readableAddress(capture) {
			return resourceReference{value: capture, indirect: true}, true
		}
	}
	return resourceReference{}, false
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
	return channel || waitGroupPointer(pointer.Elem())
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
