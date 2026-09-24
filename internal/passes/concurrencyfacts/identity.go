package concurrencyfacts

import (
	"go/token"
	"go/types"

	"github.com/kojah/gohawk/internal/heapmodel"
	"github.com/kojah/gohawk/internal/ssaflow"
	"golang.org/x/tools/go/ssa"
)

// A mutex reached through a pointer field, as in s.conn.mu, has no exact root
// in embeddedPath: the load of s.conn could see a different object each time.
// When heapmodel proves the field write-once, every load of it through the
// same path sees the same object, in every function and goroutine, so the
// load is an exact root too. The function's first such load stands for all
// of them, and binding maps it through the caller's own load of the same
// path. Only mutex identities use this: a channel read from a field keeps the
// slot-stability proof in bindField, and fact export stays parameter-relative,
// so a summary that uses such a root is not published.

// fieldInventory holds the package's write-once fields and the canonical load
// chosen for each load, shared by every query of one engine.
type fieldInventory struct {
	fields    *heapmodel.WriteOnceFields
	pkg       *ssa.Package
	canonical map[*ssa.UnOp]*ssa.UnOp
}

func (inventory *fieldInventory) use(fields *heapmodel.WriteOnceFields) {
	inventory.fields = fields
}

// writeOnce returns the inventory for function's package. An engine built
// without an analysis pass indexes the package's declared functions on first
// use; an engine never mixes packages.
func (engine *Engine) writeOnce(function *ssa.Function) *heapmodel.WriteOnceFields {
	inventory := engine.fields
	if inventory == nil || function == nil || function.Pkg == nil {
		return nil
	}
	if inventory.pkg == nil {
		inventory.pkg = function.Pkg
		if inventory.fields == nil {
			inventory.fields = heapmodel.NewWriteOnceFields(function.Pkg.Pkg, ssaflow.DeclaredFunctions(function.Pkg))
		}
	}
	if inventory.pkg != function.Pkg {
		return nil
	}
	return inventory.fields
}

// identityPath names a mutex by its path, allowing write-once loads as roots.
func (engine *Engine) identityPath(value ssa.Value) (ssaflow.EmbeddedFieldPath, bool) {
	if !MutexPointer(value.Type()) {
		return embeddedPath(value)
	}
	return pathFrom(value, engine.fixedLoad)
}

// fixedLoad returns the canonical load for a load of a write-once field whose
// address has an exact path of its own, or for a read of a captured variable
// the closure never writes. A goroutine reaches its parent's s through such a
// cell; the launch binds the cell to its content, which the heap model must
// prove stable (see bindRoot).
func (engine *Engine) fixedLoad(load *ssa.UnOp) (*ssa.UnOp, bool) {
	if load.Op != token.MUL {
		return nil, false
	}
	if capture, ok := load.X.(*ssa.FreeVar); ok {
		return capturedLoad(load, capture)
	}
	address, ok := load.X.(*ssa.FieldAddr)
	if !ok || !engine.writeOnce(load.Parent()).Fixed(fieldOf(address)) {
		return nil, false
	}
	canonical, seen := engine.fields.canonical[load]
	if seen {
		return canonical, canonical != nil
	}
	engine.fields.canonical[load] = nil
	base, ok := pathFrom(address, engine.fixedLoad)
	if !ok {
		return nil, false
	}
	for _, candidate := range ssaflow.InstructionsOf[*ssa.UnOp](load.Parent()) {
		if !engine.budget.Spend() {
			return nil, false
		}
		other, ok := candidate.X.(*ssa.FieldAddr)
		if candidate.Op != token.MUL || !ok || fieldOf(other) != fieldOf(address) {
			continue
		}
		if path, ok := pathFrom(other, engine.fixedLoad); ok && path == base {
			engine.fields.canonical[load] = candidate
			return candidate, true
		}
	}
	return nil, false
}

// capturedLoad returns the closure's first read of capture when every use of
// the capture is a read.
func capturedLoad(load *ssa.UnOp, capture *ssa.FreeVar) (*ssa.UnOp, bool) {
	var first *ssa.UnOp
	for _, use := range *capture.Referrers() {
		read, ok := use.(*ssa.UnOp)
		if !ok || read.Op != token.MUL {
			return nil, false
		}
		if first == nil {
			first = read
		}
	}
	return first, first != nil && load.X == capture
}

// bindRoot maps a callee path root to the caller's path for the same object.
func (engine *Engine) bindRoot(root ssa.Value, bindings []ssaflow.CallBinding, instruction ssa.Instruction) (ssaflow.EmbeddedFieldPath, bool) {
	switch root := root.(type) {
	case *ssa.Global:
		return ssaflow.EmbeddedFieldPath{Root: root}, true
	case *ssa.UnOp:
		if capture, ok := root.X.(*ssa.FreeVar); ok {
			return engine.bindCapturedRoot(capture, bindings, instruction)
		}
		address, ok := root.X.(*ssa.FieldAddr)
		if !ok {
			return ssaflow.EmbeddedFieldPath{}, false
		}
		base, ok := pathFrom(address, engine.fixedLoad)
		if !ok {
			return ssaflow.EmbeddedFieldPath{}, false
		}
		inner, ok := engine.bindRoot(base.Root, bindings, instruction)
		if !ok {
			return ssaflow.EmbeddedFieldPath{}, false
		}
		inner, ok = inner.Append(base.Fields[:base.Depth]...)
		if !ok {
			return ssaflow.EmbeddedFieldPath{}, false
		}
		load, ok := engine.callerLoad(instruction.Parent(), inner, fieldOf(address))
		return ssaflow.EmbeddedFieldPath{Root: load}, ok
	}
	for _, binding := range bindings {
		if binding.Local == root {
			return pathFrom(binding.Supplied, engine.fixedLoad)
		}
	}
	return ssaflow.EmbeddedFieldPath{}, false
}

// bindCapturedRoot maps a read of a captured cell to what the caller's cell
// holds, which the heap model must prove stable from the launch on. A cell
// forwarded from the caller's own capture stays a read of that capture.
func (engine *Engine) bindCapturedRoot(
	capture *ssa.FreeVar, bindings []ssaflow.CallBinding, instruction ssa.Instruction,
) (ssaflow.EmbeddedFieldPath, bool) {
	for _, binding := range bindings {
		if binding.Local != capture {
			continue
		}
		if outer, ok := binding.Supplied.(*ssa.FreeVar); ok {
			for _, use := range *outer.Referrers() {
				if read, ok := use.(*ssa.UnOp); ok {
					if canonical, ok := capturedLoad(read, outer); ok {
						return ssaflow.EmbeddedFieldPath{Root: canonical}, true
					}
				}
			}
			return ssaflow.EmbeddedFieldPath{}, false
		}
		content := engine.storage.StableContent(binding.Supplied, instruction)
		if !content.Proven() {
			return ssaflow.EmbeddedFieldPath{}, false
		}
		return pathFrom(content.Value, engine.fixedLoad)
	}
	return ssaflow.EmbeddedFieldPath{}, false
}

// callerLoad finds the caller's canonical load of the field at address path.
func (engine *Engine) callerLoad(function *ssa.Function, address ssaflow.EmbeddedFieldPath, field *types.Var) (*ssa.UnOp, bool) {
	for _, candidate := range ssaflow.InstructionsOf[*ssa.UnOp](function) {
		if !engine.budget.Spend() {
			return nil, false
		}
		other, ok := candidate.X.(*ssa.FieldAddr)
		if candidate.Op != token.MUL || !ok || fieldOf(other) != field {
			continue
		}
		if path, ok := pathFrom(other, engine.fixedLoad); ok && path == address {
			return engine.fixedLoad(candidate)
		}
	}
	return nil, false
}

// fieldOf returns the field a field address selects.
func fieldOf(address *ssa.FieldAddr) *types.Var {
	pointer, ok := address.X.Type().Underlying().(*types.Pointer)
	if !ok {
		return nil
	}
	structure, ok := pointer.Elem().Underlying().(*types.Struct)
	if !ok {
		return nil
	}
	return structure.Field(address.Field)
}
