package ssaflow

import (
	"go/constant"
	"go/token"

	"golang.org/x/tools/go/ssa"
)

// StorageInteger resolves a constant SSA index or the supplied default for an omitted bound.
func StorageInteger(value ssa.Value, fallback int64) (int64, bool) {
	if value == nil {
		return fallback, true
	}
	literal, ok := value.(*ssa.Const)
	if !ok || literal.Value == nil {
		return 0, false
	}
	return constant.Int64Val(literal.Value)
}

// IntegerLiteralEquals reports whether value is an integer constant equal to want.
// It does not evaluate arithmetic, conversions or merged values.
func IntegerLiteralEquals(value ssa.Value, want int64) bool {
	literal, ok := value.(*ssa.Const)
	return ok && literal.Value != nil && literal.Value.Kind() == constant.Int &&
		constant.Compare(literal.Value, token.EQL, constant.MakeInt64(want))
}
