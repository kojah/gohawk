package ssaflow

import "golang.org/x/tools/go/ssa"

// EmbeddedFieldPath is a bounded sequence of embedded field addresses rooted
// in one exact SSA value. It never equates a loaded pointer with its cell or
// proves that an address exists in another function. Unused Fields are zero.
type EmbeddedFieldPath struct {
	Root   ssa.Value
	Fields [8]int
	Depth  int
}

// ResolveEmbeddedFieldPath resolves an agreed embedded-field path through the
// walk's selected transparent forms. acceptRoot chooses eligible terminal values;
// accepting a load names that exact snapshot, never its underlying cell.
func ResolveEmbeddedFieldPath(walk ReachingWalk, value ssa.Value, acceptRoot func(ssa.Value) bool) (EmbeddedFieldPath, bool) {
	if acceptRoot == nil {
		return EmbeddedFieldPath{}, false
	}
	var leaf func(ReachingWalk, ssa.Value) (EmbeddedFieldPath, bool)
	leaf = func(walk ReachingWalk, value ssa.Value) (EmbeddedFieldPath, bool) {
		if field, ok := value.(*ssa.FieldAddr); ok {
			path, known := ResolveReachingValue(walk, field.X, leaf, func(path EmbeddedFieldPath) EmbeddedFieldPath { return path })
			if !known {
				return EmbeddedFieldPath{}, false
			}
			return path.Append(field.Field)
		}
		if acceptRoot(value) {
			return EmbeddedFieldPath{Root: value}, true
		}
		return EmbeddedFieldPath{}, false
	}
	return ResolveReachingValue(walk, value, leaf, func(path EmbeddedFieldPath) EmbeddedFieldPath { return path })
}

// Append extends a path without changing its root, or declines an invalid or
// over-budget path. It does not construct an SSA field address or validate type
// projections; callers append only field indexes established from their IR.
func (path EmbeddedFieldPath) Append(fields ...int) (EmbeddedFieldPath, bool) {
	if path.Root == nil || path.Depth < 0 || path.Depth > len(path.Fields) || len(fields) > len(path.Fields)-path.Depth {
		return EmbeddedFieldPath{}, false
	}
	for _, field := range fields {
		if field < 0 {
			return EmbeddedFieldPath{}, false
		}
	}
	copy(path.Fields[path.Depth:], fields)
	path.Depth += len(fields)
	return path, true
}
