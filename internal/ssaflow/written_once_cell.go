package ssaflow

import (
	"go/token"

	"golang.org/x/tools/go/ssa"
)

// WrittenOnceCell returns the value stored in cell when that store is the
// cell's only write anywhere: the function only reads the cell, and every
// closure that captures it, nested ones included, only reads it too. Every
// read then yields that value, in any goroutine and at any time after the
// store, which is how a variable captured by several goroutines names one
// object. It answers identity only; whether the value itself is stable is the
// caller's question.
func WrittenOnceCell(cell *ssa.Alloc) (ssa.Value, bool) {
	if cell.Referrers() == nil {
		return nil, false
	}
	var stored ssa.Value
	for _, use := range *cell.Referrers() {
		switch use := use.(type) {
		case *ssa.Store:
			if use.Addr != cell || stored != nil {
				return nil, false
			}
			stored = use.Val
		case *ssa.UnOp:
			if use.Op != token.MUL {
				return nil, false
			}
		case *ssa.MakeClosure:
			if !capturedReadOnly(use, cell) {
				return nil, false
			}
		case *ssa.DebugRef:
		default:
			return nil, false
		}
	}
	return stored, stored != nil
}

// capturedReadOnly reports whether closure, and every closure nested in it
// that captures the same cell, only reads cell.
func capturedReadOnly(closure *ssa.MakeClosure, cell ssa.Value) bool {
	function, ok := closure.Fn.(*ssa.Function)
	if !ok {
		return false
	}
	for index, binding := range closure.Bindings {
		if binding != cell || index >= len(function.FreeVars) {
			continue
		}
		capture := function.FreeVars[index]
		if capture.Referrers() == nil {
			continue
		}
		for _, use := range *capture.Referrers() {
			switch use := use.(type) {
			case *ssa.UnOp:
				if use.Op != token.MUL {
					return false
				}
			case *ssa.MakeClosure:
				if !capturedReadOnly(use, capture) {
					return false
				}
			case *ssa.DebugRef:
			default:
				return false
			}
		}
	}
	return true
}
