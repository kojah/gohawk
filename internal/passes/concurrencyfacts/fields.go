package concurrencyfacts

import (
	"go/types"

	"github.com/kojah/gohawk/internal/ssaflow"
	"golang.org/x/tools/go/ssa"
)

// Embedded mutex fields are addresses, not pointer-valued contents. A bounded
// path can therefore be rebound without following mutable pointer fields.
// Bindings require an existing matching caller address; we never invent SSA
// values or treat an enclosing owner as the mutex itself.
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
			if !ok || !MutexPointer(field.Type()) {
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
	if reference.Indirect || !ok || path.Depth == 0 || !MutexPointer(reference.Value.Type()) {
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
		return Reference{Value: value}, found
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

// FreshMutex reports whether an exact embedded mutex address belongs to a
// fresh local allocation. It does not prove scope completeness or lock order.
func FreshMutex(function *ssa.Function, reference Reference) bool {
	if reference.Indirect || reference.Value == nil || !MutexPointer(reference.Value.Type()) {
		return false
	}
	path, ok := embeddedPath(reference.Value)
	allocation, local := path.Root.(*ssa.Alloc)
	return ok && local && allocation.Parent() == function
}
