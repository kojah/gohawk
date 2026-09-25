package concurrencyfacts

import (
	"go/token"
	"go/types"

	"github.com/kojah/gohawk/internal/heapmodel"
	"github.com/kojah/gohawk/internal/ssaflow"
	"golang.org/x/tools/go/ssa"
)

// Embedded mutex paths name addresses; channel paths name the contents of a
// slot. The latter require the shared heap proof that the slot remains stable
// through every helper and worker. A matching field name alone is not identity.
// Bindings require an existing caller address; no SSA values are invented.
// embeddedPath names value as fields selected from an exact root. Besides a
// local allocation, a parameter, and a capture, two roots are exact. A package
// variable is one object in every call, so a mutex embedded in it needs no
// binding; a channel or pointer field of it is content another goroutine can
// replace, and is not named here. And when a closure captures a parameter, the
// builder spills it to a cell and reads it back: such a read is the parameter
// itself while the heap model proves the cell still holds it, which a write by
// the closure or a later reassignment breaks.
func embeddedPath(value ssa.Value) (ssaflow.EmbeddedFieldPath, bool) {
	return pathFrom(value, nil)
}

// pathFrom is embeddedPath with an optional rule that accepts a load as a
// root and returns the load that stands for it; see identity.go.
func pathFrom(value ssa.Value, loaded func(*ssa.UnOp) (*ssa.UnOp, bool)) (ssaflow.EmbeddedFieldPath, bool) {
	mutex := MutexPointer(value.Type())
	path, ok := ssaflow.ResolveEmbeddedFieldPath(ssaflow.NewReachingWalk(ssaflow.TransparentNone), value, func(root ssa.Value) bool {
		switch root := root.(type) {
		case *ssa.Alloc, *ssa.Parameter, *ssa.FreeVar:
			return true
		case *ssa.Global:
			return mutex
		case *ssa.UnOp:
			if _, spilled := spilledParameter(root); spilled {
				return true
			}
			if loaded != nil {
				_, fixed := loaded(root)
				return fixed
			}
		}
		return false
	})
	if !ok {
		return path, false
	}
	if parameter, spilled := spilledParameter(path.Root); spilled {
		path.Root = parameter
	} else if load, isLoad := path.Root.(*ssa.UnOp); isLoad && loaded != nil {
		canonical, _ := loaded(load)
		path.Root = canonical
	}
	return path, true
}

// spilledParameter returns the parameter a load reads back from its spill cell.
func spilledParameter(value ssa.Value) (*ssa.Parameter, bool) {
	load, ok := value.(*ssa.UnOp)
	if !ok || load.Op != token.MUL {
		return nil, false
	}
	cell, ok := load.X.(*ssa.Alloc)
	if !ok || cell.Referrers() == nil {
		return nil, false
	}
	if parameter, ok := onlyStoredParameter(cell); ok {
		return parameter, true
	}
	for _, use := range *cell.Referrers() {
		store, ok := use.(*ssa.Store)
		if !ok || store.Addr != cell {
			continue
		}
		if parameter, ok := store.Val.(*ssa.Parameter); ok && heapmodel.DefinitelySameValue(load, parameter) {
			return parameter, true
		}
	}
	return nil, false
}

// onlyStoredParameter returns the parameter when the spill is the cell's only
// write anywhere, which ssaflow.WrittenOnceCell proves structurally. Then every
// read yields the parameter, even in a goroutine and whatever the timing,
// which the heap model's per-point proof cannot show once the cell reaches an
// asynchronous participant.
func onlyStoredParameter(cell *ssa.Alloc) (*ssa.Parameter, bool) {
	stored, ok := ssaflow.WrittenOnceCell(cell)
	parameter, isParameter := stored.(*ssa.Parameter)
	return parameter, ok && isParameter
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
			if candidate, ok := engine.identityPath(field); ok && candidate == path {
				return field, true
			}
		}
	}
	return nil, false
}

func (engine *Engine) bindField(reference Reference, bindings []ssaflow.CallBinding, instruction ssa.Instruction) (Reference, bool) {
	path, ok := engine.identityPath(reference.Value)
	if reference.Projection.Depth > 0 {
		path, ok = reference.Projection, true
	}
	channelContent := reference.Indirect && reference.Projection.Depth > 0 && ssaflow.ChannelType(reference.Value)
	if !ok || path.Depth == 0 || !channelContent && (reference.Indirect || !MutexPointer(reference.Value.Type())) {
		return Reference{}, false
	}
	if !channelContent {
		return engine.bindMutexPath(reference, path, bindings, instruction)
	}
	// A channel read from a field is the slot's content, not an address, so
	// it binds only when the caller's slot is proven stable through the call;
	// a write-once root is not enough for content another call could store.
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

// bindMutexPath maps a mutex path's root into the caller and selects the same
// fields there. A package mutex needs no binding: when the caller has no
// address of its own for it, the callee's address still names it.
func (engine *Engine) bindMutexPath(
	reference Reference, path ssaflow.EmbeddedFieldPath, bindings []ssaflow.CallBinding, instruction ssa.Instruction,
) (Reference, bool) {
	root, valid := engine.bindRoot(path.Root, bindings, instruction)
	if !valid {
		return Reference{}, false
	}
	root, valid = root.Append(path.Fields[:path.Depth]...)
	if !valid {
		return Reference{}, false
	}
	if value, found := engine.fieldAddress(instruction.Parent(), root); found {
		return Reference{Value: value}, true
	}
	if engine.budget.Exhausted() {
		return Reference{}, false
	}
	if _, global := path.Root.(*ssa.Global); global {
		return Reference{Value: reference.Value}, true
	}
	return Reference{Value: reference.Value, Projection: root, Indirect: reference.Indirect}, true
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
