package concurrencyfacts

import (
	"go/token"
	"go/types"

	"github.com/kojah/gohawk/internal/heapmodel"
	proofs "github.com/kojah/gohawk/internal/proof"
	"github.com/kojah/gohawk/internal/ssaflow"
	ssacall "github.com/kojah/gohawk/internal/ssaflow/calls"
	"github.com/kojah/gohawk/internal/syntax"
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
		return embeddedPathWithin(value, engine.budget)
	}
	return pathFromWithin(value, engine.fixedLoad, engine.budget)
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
		return capturedLoadWithin(load, capture, engine.budget)
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
	defer func() {
		// A recursive sentinel or failed walk is reusable only after a completed
		// search. Shared field metadata must not poison a later fresh allowance.
		if engine.budget.Exhausted() || engine.budget.PoolExhausted() {
			delete(engine.fields.canonical, load)
		}
	}()
	base, ok := pathFromWithin(address, engine.fixedLoad, engine.budget)
	if !ok {
		return nil, false
	}
	candidate, ok := engine.firstFieldLoad(load.Parent(), base, fieldOf(address))
	if !ok {
		return nil, false
	}
	engine.fields.canonical[load] = candidate
	return candidate, true
}

// capturedLoad returns the closure's first read of capture when every use of
// the capture is a read.
func capturedLoadWithin(load *ssa.UnOp, capture *ssa.FreeVar, budget *proofs.SearchBudget) (*ssa.UnOp, bool) {
	var first *ssa.UnOp
	for use := range ssaflow.ReferrersWithin(capture, budget) {
		read, ok := use.(*ssa.UnOp)
		if !ok || read.Op != token.MUL {
			return nil, false
		}
		if first == nil {
			first = read
		}
	}
	if budget.Exhausted() || budget.PoolExhausted() {
		return nil, false
	}
	return first, first != nil && load.X == capture
}

// bindRoot maps a callee path root to the caller's path for the same object.
func (engine *Engine) bindRoot(root ssa.Value, bindings []ssacall.CallBinding, instruction ssa.Instruction) (ssaflow.EmbeddedFieldPath, bool) {
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
		base, ok := pathFromWithin(address, engine.fixedLoad, engine.budget)
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
			return pathFromWithin(binding.Supplied, engine.fixedLoad, engine.budget)
		}
	}
	return ssaflow.EmbeddedFieldPath{}, false
}

// bindCapturedRoot maps a read of a captured cell to what the caller's cell
// holds, which the heap model must prove stable from the launch on. A cell
// forwarded from the caller's own capture stays a read of that capture.
func (engine *Engine) bindCapturedRoot(
	capture *ssa.FreeVar, bindings []ssacall.CallBinding, instruction ssa.Instruction,
) (ssaflow.EmbeddedFieldPath, bool) {
	for _, binding := range bindings {
		if binding.Local != capture {
			continue
		}
		if outer, ok := binding.Supplied.(*ssa.FreeVar); ok {
			for use := range ssaflow.ReferrersWithin(outer, engine.budget) {
				if read, ok := use.(*ssa.UnOp); ok {
					if canonical, ok := capturedLoadWithin(read, outer, engine.budget); ok {
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
		return pathFromWithin(content.Value, engine.fixedLoad, engine.budget)
	}
	return ssaflow.EmbeddedFieldPath{}, false
}

// callerLoad finds the caller's canonical load of the field at address path.
func (engine *Engine) callerLoad(function *ssa.Function, address ssaflow.EmbeddedFieldPath, field *types.Var) (*ssa.UnOp, bool) {
	candidate, ok := engine.firstFieldLoad(function, address, field)
	if !ok {
		return nil, false
	}
	return engine.fixedLoad(candidate)
}

// firstFieldLoad owns the bounded, block-order census and exact field/path
// match. The caller decides whether to cache that load or bind its identity;
// this search does not establish write-once or captured-storage stability.
func (engine *Engine) firstFieldLoad(function *ssa.Function, address ssaflow.EmbeddedFieldPath, field *types.Var) (*ssa.UnOp, bool) {
	for instruction := range ssaflow.InstructionsWithin(function, engine.budget) {
		candidate, isLoad := instruction.(*ssa.UnOp)
		if !isLoad {
			continue
		}
		if !engine.budget.Spend() {
			return nil, false
		}
		other, ok := candidate.X.(*ssa.FieldAddr)
		if candidate.Op != token.MUL || !ok || fieldOf(other) != field {
			continue
		}
		if path, ok := pathFromWithin(other, engine.fixedLoad, engine.budget); ok && path == address {
			return candidate, true
		}
	}
	return nil, false
}

// fieldOf returns the field a field address selects.
func fieldOf(address *ssa.FieldAddr) *types.Var {
	structure := syntax.PointerStruct(address.X.Type())
	if structure == nil {
		return nil
	}
	return structure.Field(address.Field)
}
