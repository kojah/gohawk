package concurrencyfacts

import (
	"go/types"

	"github.com/kojah/gohawk/internal/ssaflow"
	"golang.org/x/tools/go/ssa"
)

// Embedded mutex paths name addresses; channel paths name the contents of a
// slot. The latter require the shared heap proof that the slot remains stable
// through every helper and worker. A matching field name alone is not identity.
// Bindings require an existing caller address; no SSA values are invented.
func embeddedPath(value ssa.Value) (ssaflow.EmbeddedFieldPath, bool) {
	return ssaflow.ResolveEmbeddedFieldPath(ssaflow.NewReachingWalk(ssaflow.TransparentNone), value, func(root ssa.Value) bool {
		switch root.(type) {
		case *ssa.Alloc, *ssa.Parameter, *ssa.FreeVar:
			return true
		default:
			return false
		}
	})
}

func (engine *Engine) fieldAddress(function *ssa.Function, path ssaflow.EmbeddedFieldPath) (ssa.Value, bool) {
	if function == nil {
		return nil, false
	}
	for _, block := range function.Blocks {
		for _, instruction := range block.Instrs {
			if !engine.budget.Spend() {
				return nil, false
			}
			field, ok := instruction.(*ssa.FieldAddr)
			if !ok {
				continue
			}
			if candidate, ok := embeddedPath(field); ok && candidate == path {
				return field, true
			}
		}
	}
	return nil, false
}

func (engine *Engine) bindField(reference Reference, bindings []ssaflow.CallBinding, instruction ssa.Instruction) (Reference, bool) {
	path, ok := embeddedPath(reference.Value)
	if reference.Projection.Depth > 0 {
		path, ok = reference.Projection, true
	}
	channelContent := reference.Indirect && reference.Projection.Depth > 0 && ssaflow.ChannelType(reference.Value)
	if !ok || path.Depth == 0 || !channelContent && (reference.Indirect || !MutexPointer(reference.Value.Type())) {
		return Reference{}, false
	}
	for _, binding := range bindings {
		if binding.Local != path.Root {
			continue
		}
		root, valid := embeddedPath(binding.Supplied)
		if !valid {
			return Reference{}, false
		}
		root, valid = root.Append(path.Fields[:path.Depth]...)
		if !valid {
			return Reference{}, false
		}
		value, found := engine.fieldAddress(instruction.Parent(), root)
		if found {
			if channelContent {
				content := engine.storage.StableFieldContent(value, instruction)
				if !content.Proven() {
					return Reference{}, false
				}
				return engine.reference(content.Value)
			}
			return Reference{Value: value}, true
		}
		if engine.budget.Exhausted() {
			return Reference{}, false
		}
		return Reference{Value: reference.Value, Projection: root, Indirect: reference.Indirect}, true
	}
	return Reference{}, false
}

func (engine *Engine) importedField(call ssa.CallInstruction, value ssa.Value, fields []int) (ssa.Value, bool) {
	path, ok := embeddedPath(value)
	if !ok {
		return nil, false
	}
	path, ok = path.Append(fields...)
	if !ok {
		return nil, false
	}
	return engine.fieldAddress(call.Parent(), path)
}

// Whole-owner stores can reset embedded synchronization state just as a store
// directly to a Mutex or WaitGroup can. Pointer fields do not embed that state.
func containsSynchronization(value types.Type) bool {
	if synchronizationPointer(types.NewPointer(value)) {
		return true
	}
	switch value := value.Underlying().(type) {
	case *types.Struct:
		for field := range value.Fields() {
			if containsSynchronization(field.Type()) {
				return true
			}
		}
	case *types.Array:
		return containsSynchronization(value.Elem())
	}
	return false
}
