package ssaflow

import (
	"go/constant"

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
