package ssaflow

import (
	"go/token"
	"strings"

	"golang.org/x/tools/go/ssa"
)

// Access paths name the part of an aggregate a value is, so that a claim
// about "the file in field out of this parameter" can be made and matched
// exactly, rather than collapsing to "something derived from the parameter".
// A path is the static sequence of field and constant-index selections from
// a root; a runtime index has no path. These helpers extract a path beneath
// a parameter, including through the cell a by-value parameter is spilled
// into, and resolve the value a caller stored at a path beneath an argument.

// AccessPathOf returns the field and constant-index steps by which value is
// selected beneath root, empty for root itself. A load through an address
// beneath root has the address's path.
func AccessPathOf(value, root ssa.Value) ([]string, bool) {
	return accessPath(value, root, map[ssa.Value]bool{})
}

// AccessPathFromParameter is AccessPathOf with the parameter's spill cells as
// alternative roots: a struct or array parameter is copied into a local
// cell before a field is selected, and a cell that is only ever written
// whole from the parameter holds exactly the parameter's contents.
func AccessPathFromParameter(value, parameter ssa.Value) ([]string, bool) {
	if path, ok := AccessPathOf(value, parameter); ok {
		return path, true
	}
	if parameter.Referrers() == nil {
		return nil, false
	}
	for _, reference := range *parameter.Referrers() {
		store, ok := reference.(*ssa.Store)
		if !ok || store.Val != parameter {
			continue
		}
		cell, ok := store.Addr.(*ssa.Alloc)
		if !ok || !WholeWrittenCell(cell) {
			continue
		}
		if path, ok := AccessPathOf(value, cell); ok {
			return path, true
		}
	}
	return nil, false
}

// JoinAccessPath renders a path for a fact or a key; the empty path is "".
func JoinAccessPath(path []string) string {
	return strings.Join(path, "/")
}

// SplitAccessPath is the inverse of JoinAccessPath.
func SplitAccessPath(joined string) []string {
	if joined == "" {
		return nil
	}
	return strings.Split(joined, "/")
}

// ValueAtPath resolves the value stored at path beneath root as observed at
// observation. The root may be an address, such as a local aggregate or a
// pointer, or a load of a whole aggregate, in which case the loaded cell is
// the root. An empty path is the root itself. The address is found by
// following the selections the function actually made, so a path nobody
// selected resolves to nothing.
func ValueAtPath(root ssa.Value, path []string, observation ssa.Instruction) (ssa.Value, bool) { //nolint:ireturn // SSA values keep their concrete forms.
	if len(path) == 0 {
		return root, true
	}
	// The graph knows the slot whether or not the function selected it by
	// that path; the selection walk below is kept for the paths the graph
	// cannot resolve to one object.
	if value, ok := regionsOf(root).valueAtPath(root, path, observation); ok {
		return value, true
	}
	if load, ok := root.(*ssa.UnOp); ok && load.Op == token.MUL {
		root = load.X
	}
	for _, address := range selectionsOf(root, path) {
		if content := NewStorage(nil).Content(address, observation); content.Proven() {
			return content.Value, true
		}
	}
	return nil, false
}

// ContentIsNilAt reports whether the value at path beneath root is certainly
// nil when observation runs: the root refers to one object, the slot holds
// one non-stale entry, and it is nil. An empty path asks whether the root
// itself is nil. A must-answer, so a value that is nil on one branch only
// is not nil here.
func ContentIsNilAt(root ssa.Value, path []string, observation ssa.Instruction) bool {
	// The graph is the observing function's: a constant root belongs to no
	// function, and a nil constant is nil in every one.
	if observation == nil || observation.Parent() == nil {
		return false
	}
	return regionsOfFunction(observation.Parent()).contentIsNil(root, path, observation)
}

// selectionsOf returns every address the function selected beneath root by
// exactly path.
func selectionsOf(root ssa.Value, path []string) []ssa.Value {
	frontier := []ssa.Value{root}
	for _, step := range path {
		var next []ssa.Value
		for _, address := range frontier {
			if address.Referrers() == nil {
				continue
			}
			for _, reference := range *address.Referrers() {
				selected, ok := reference.(ssa.Value)
				if !ok {
					continue
				}
				if selection, ok := accessPath(selected, address, map[ssa.Value]bool{}); ok && len(selection) == 1 && selection[0] == step {
					next = append(next, selected)
				}
			}
		}
		frontier = next
	}
	return frontier
}

// StoredPath returns the access path beneath root at which target is stored,
// as observed at observation: the field or constant-index selection whose
// content is the target. It looks one and two selections deep, which covers
// a field of a struct and an element of an array held in a field.
func StoredPath(root, target ssa.Value, observation ssa.Instruction) ([]string, bool) {
	if path, ok := regionsOf(root).storedPath(root, target, observation); ok {
		return path, true
	}
	if load, ok := root.(*ssa.UnOp); ok && load.Op == token.MUL {
		root = load.X
	}
	storage := NewStorage(nil)
	var walk func(address ssa.Value, prefix []string, depth int) ([]string, bool)
	walk = func(address ssa.Value, prefix []string, depth int) ([]string, bool) {
		if depth == 0 || address.Referrers() == nil {
			return nil, false
		}
		for _, reference := range *address.Referrers() {
			selected, ok := reference.(ssa.Value)
			if !ok {
				continue
			}
			step, ok := accessPath(selected, address, map[ssa.Value]bool{})
			if !ok || len(step) != 1 {
				continue
			}
			path := append(append([]string(nil), prefix...), step[0])
			if content := storage.Content(selected, observation); content.Proven() && storage.Same(content.Value, target).Proven() {
				return path, true
			}
			if found, ok := walk(selected, path, depth-1); ok {
				return found, true
			}
		}
		return nil, false
	}
	return walk(root, nil, 2)
}
