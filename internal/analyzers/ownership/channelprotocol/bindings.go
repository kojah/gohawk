package channelprotocol

import (
	"go/token"
	"go/types"

	"github.com/kojah/gohawk/internal/ssaflow"

	"golang.org/x/tools/go/ssa"
)

// Binding a summary preserves the time at which a channel was read. Captured
// cells additionally need stable contents because the worker may read them
// after the launch. A matching access path alone cannot justify this identity.
func (engine *summaryEngine) instantiate(instruction ssa.CallInstruction) summary {
	function, closure := ssaflow.DirectCallee(instruction.Common())
	if function == nil {
		return summary{reason: "protocol-body-unavailable"}
	}
	callee := engine.summarize(function)
	if callee.reason != "" {
		return callee
	}
	bindings := ssaflow.CallBindings(instruction.Common(), function, closure)
	result := summary{operations: make([]operation, 0, len(callee.operations))}
	for _, op := range callee.operations {
		if !engine.budget.Spend() {
			engine.memo.Cut()
			return summary{reason: "protocol-budget-exhausted"}
		}
		channel, ok := engine.bind(op.channel, bindings, instruction)
		if !ok {
			return summary{reason: "protocol-channel-binding-unknown"}
		}
		op.channel, op.site = channel, instruction.Pos()
		result.operations = append(result.operations, op)
	}
	return result
}

func (engine *summaryEngine) bind(
	reference channelReference, bindings []ssaflow.CallBinding, instruction ssa.Instruction,
) (channelReference, bool) {
	for _, binding := range bindings {
		if binding.Local != reference.value {
			continue
		}
		if !reference.indirect {
			return engine.reference(binding.Supplied)
		}
		if _, captured := binding.Supplied.(*ssa.FreeVar); captured {
			return channelReference{value: binding.Supplied, indirect: true}, true
		}
		content := engine.storage.StableContent(binding.Supplied, instruction)
		if content.Proven() {
			return engine.reference(content.Value)
		}
		return channelReference{}, false
	}
	return channelReference{}, false
}

func (engine *summaryEngine) reference(value ssa.Value) (channelReference, bool) {
	if !ssaflow.ChannelType(value) {
		return channelReference{}, false
	}
	return ssaflow.ResolveReachingValue(ssaflow.NewReachingWalk(ssaflow.TransparentChangeType), value,
		engine.referenceLeaf, func(reference channelReference) channelReference { return reference })
}

func (engine *summaryEngine) referenceLeaf(_ ssaflow.ReachingWalk, value ssa.Value) (channelReference, bool) {
	resolved := engine.storage.Resolve(value)
	if resolved.Proven() {
		switch resolved.Value.(type) {
		case *ssa.Parameter, *ssa.FreeVar, *ssa.MakeChan:
			return channelReference{value: resolved.Value}, true
		}
	}
	// A symbolic captured cell is resolved at its caller, where its local
	// allocation and all writes are visible to the shared storage query.
	load, ok := value.(*ssa.UnOp)
	if ok && load.Op == token.MUL {
		if capture, ok := load.X.(*ssa.FreeVar); ok && readableAddress(capture) {
			return channelReference{value: capture, indirect: true}, true
		}
	}
	return channelReference{}, false
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
	return channel
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
