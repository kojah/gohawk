package ssaflow

import (
	"go/constant"
	"go/token"
	"strconv"
	"strings"

	"golang.org/x/tools/go/ssa"
)

// Static access paths name a value by the field and constant-index steps that
// select it from a root, rendered as strings such as "field:0/index:1" so a
// summary can carry them across packages. EmbeddedFieldPath is a different
// vocabulary for a different question: the numeric embedded-field indexes a
// promoted method selects from one exact root, bounded and never rendered.
// The two are kept apart because a summary step must survive serialization
// and a promotion path must not be confused with an arbitrary field selection.

// JoinAccessPath renders a static field/index path for a summary or key.
func JoinAccessPath(path []string) string { return strings.Join(path, "/") }

// SplitAccessPath reverses JoinAccessPath.
func SplitAccessPath(joined string) []string {
	if joined == "" {
		return nil
	}
	return strings.Split(joined, "/")
}

// AccessPath identifies one SSA value relative to the aggregate root from
// which its fields and indexes are selected.
type AccessPath struct {
	Value ssa.Value
	Root  ssa.Value
}

// ValueIsAccessPathFrom reports whether value is root itself or a statically
// identifiable field or constant-index projection beneath root.
func ValueIsAccessPathFrom(value, root ssa.Value) bool {
	_, ok := AccessPathSteps(value, root)
	return ok
}

// SameAccessPath reports whether left and right select the same sequence of
// fields and constant indexes from their respective roots. It maps a closure's
// free-variable access back to the captured binding without equating either
// selected field with the aggregate that contains it.
func SameAccessPath(left, right AccessPath) bool {
	leftPath, leftOK := AccessPathSteps(left.Value, left.Root)
	rightPath, rightOK := AccessPathSteps(right.Value, right.Root)
	if !leftOK || !rightOK || len(leftPath) != len(rightPath) {
		return false
	}
	for index := range leftPath {
		if leftPath[index] != rightPath[index] {
			return false
		}
	}
	return true
}

func AccessPathSteps(value, root ssa.Value) ([]string, bool) {
	return accessPathSteps(value, root, map[ssa.Value]bool{})
}

func accessPathSteps(value, root ssa.Value, seen map[ssa.Value]bool) ([]string, bool) {
	if value == nil || root == nil || seen[value] {
		return nil, false
	}
	if StructurallyIdentical(value, root) {
		return nil, true
	}
	seen[value] = true
	if inner, ok := UnwrapTransparentValue(
		value,
		TransparentChangeInterface|TransparentChangeType|TransparentConvert|TransparentMakeInterface,
	); ok {
		return accessPathSteps(inner, root, seen)
	}
	switch typed := value.(type) {
	case *ssa.FieldAddr:
		path, ok := accessPathSteps(typed.X, root, seen)
		return appendAccess(path, "field:"+strconv.Itoa(typed.Field), ok)
	case *ssa.IndexAddr:
		index, ok := ConstantIndex(typed.Index)
		if !ok {
			return nil, false
		}
		path, baseOK := accessPathSteps(typed.X, root, seen)
		return appendAccess(path, "index:"+index, baseOK)
	case *ssa.UnOp:
		if typed.Op == token.MUL && StructurallyIdentical(typed.X, root) {
			return nil, true
		}
		return accessPathSteps(typed.X, root, seen)
	}
	return nil, false
}

func appendAccess(path []string, component string, ok bool) ([]string, bool) {
	if !ok {
		return nil, false
	}
	return append(path, component), true
}

func ConstantIndex(value ssa.Value) (string, bool) {
	literal, ok := value.(*ssa.Const)
	if !ok || literal.Value == nil || literal.Value.Kind() != constant.Int {
		return "", false
	}
	return literal.Value.ExactString(), true
}
