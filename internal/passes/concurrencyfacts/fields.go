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
type mutexPath struct {
	root   ssa.Value
	fields [8]int
	depth  int
}

func embeddedPath(value ssa.Value) (mutexPath, bool) {
	return ssaflow.ResolveReachingValue(ssaflow.NewReachingWalk(ssaflow.TransparentNone), value, embeddedPathLeaf,
		func(path mutexPath) mutexPath { return path })
}

func embeddedPathLeaf(walk ssaflow.ReachingWalk, value ssa.Value) (mutexPath, bool) {
	switch value := value.(type) {
	case *ssa.Alloc, *ssa.Parameter, *ssa.FreeVar:
		return mutexPath{root: value}, true
	case *ssa.FieldAddr:
		path, ok := ssaflow.ResolveReachingValue(walk, value.X, embeddedPathLeaf, func(path mutexPath) mutexPath { return path })
		if !ok || path.depth == len(path.fields) {
			return mutexPath{}, false
		}
		path.fields[path.depth] = value.Field
		path.depth++
		return path, true
	default:
		return mutexPath{}, false
	}
}

func (engine *Engine) fieldAddress(function *ssa.Function, path mutexPath) (ssa.Value, bool) {
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
	if reference.Indirect || !ok || path.depth == 0 || !MutexPointer(reference.Value.Type()) {
		return Reference{}, false
	}
	for _, binding := range bindings {
		if binding.Local != path.root {
			continue
		}
		root, valid := embeddedPath(binding.Supplied)
		if !valid || root.depth+path.depth > len(path.fields) {
			return Reference{}, false
		}
		copy(root.fields[root.depth:], path.fields[:path.depth])
		root.depth += path.depth
		value, found := engine.fieldAddress(instruction.Parent(), root)
		return Reference{Value: value}, found
	}
	return Reference{}, false
}

func (engine *Engine) importedField(call ssa.CallInstruction, value ssa.Value, fields []int) (ssa.Value, bool) {
	path, ok := embeddedPath(value)
	if !ok || path.depth+len(fields) > len(path.fields) {
		return nil, false
	}
	copy(path.fields[path.depth:], fields)
	path.depth += len(fields)
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
	allocation, local := path.root.(*ssa.Alloc)
	return ok && local && allocation.Parent() == function
}
