package lockorder

import (
	"fmt"
	"go/token"
	"go/types"
	"maps"
	"slices"
	"strings"

	"golang.org/x/tools/go/ssa"
)

func lockStateKey(state lockFlowState) string {
	guards := make([]string, 0, len(state.guards))
	for identity, guard := range state.guards {
		guards = append(guards, fmt.Sprintf("%s:%s=%t", identity, guard.condition, guard.value))
	}
	slices.Sort(guards)
	return fmt.Sprintf(
		"%d:%s:%s:%s:%s=%t",
		state.block.Index,
		strings.Join(state.held, ","),
		strings.Join(state.deferred, ","),
		strings.Join(guards, ","),
		state.condition,
		state.conditionValue,
	)
}

func cloneLockGuards(source map[string]lockGuard) map[string]lockGuard {
	result := make(map[string]lockGuard, len(source))
	maps.Copy(result, source)
	return result
}

func guardConflicts(held []string, guards map[string]lockGuard, condition string, value bool) bool {
	for _, identity := range held {
		guard, ok := guards[identity]
		if ok && guard.condition == condition && guard.value != value {
			return true
		}
	}
	return false
}

func blockCondition(block *ssa.BasicBlock) (string, bool) {
	if len(block.Instrs) == 0 {
		return "", false
	}
	branch, ok := block.Instrs[len(block.Instrs)-1].(*ssa.If)
	if !ok {
		return "", false
	}
	return conditionIdentity(branch.Cond)
}

func conditionIdentity(value ssa.Value) (string, bool) {
	if parameter, ok := value.(*ssa.Parameter); ok {
		basic, boolean := parameter.Type().Underlying().(*types.Basic)
		if boolean && basic.Info()&types.IsBoolean != 0 {
			// A bare Boolean parameter is stable for the function invocation, so
			// repeated branches on that exact SSA value cannot disagree. Keep this
			// narrower than general derivation: loads and phis may change between
			// the acquisition and release checks.
			return "boolean:" + conditionOperandIdentity(parameter), true
		}
	}
	comparison, ok := value.(*ssa.BinOp)
	if !ok || (comparison.Op != token.EQL && comparison.Op != token.NEQ) {
		return "", false
	}
	left := conditionOperandIdentity(comparison.X)
	right := conditionOperandIdentity(comparison.Y)
	if right < left {
		left, right = right, left
	}
	return comparison.Op.String() + ":" + left + ":" + right, true
}

func conditionOperandIdentity(value ssa.Value) string {
	switch typed := value.(type) {
	case *ssa.Parameter:
		return fmt.Sprintf("parameter:%p", typed)
	case *ssa.Const:
		if typed.Value == nil {
			return "constant:nil:" + types.TypeString(typed.Type(), nil)
		}
		return "constant:" + typed.Value.ExactString()
	default:
		return fmt.Sprintf("%T:%p", value, value)
	}
}
