package ssaflow

import (
	"fmt"
	"go/constant"
	"go/token"
	"slices"
	"strings"

	"golang.org/x/tools/go/ssa"
)

// Constant Boolean arguments fix a callee's branches. A call that passes a
// literal true or false, or a value its own caller already fixed, decides
// every branch in the callee that tests that parameter, or the captured copy
// of it, directly or negated. A comparison, a phi, or any value derived from
// the parameter stays with the ordinary feasibility view: only the exact
// bound value is decided, so a binding can remove an impossible path but
// never invent a possible one.

// BooleanConstants fixes Boolean parameters and captured variables of the
// bodies being searched to constant values. A *ssa.Parameter key holds the
// value itself. A *ssa.FreeVar key is a captured cell, as Go captures every
// variable by reference, and holds the value every load of the cell reads;
// it is bound only when the cell is written once before capture and every
// closure only reads it.
type BooleanConstants map[ssa.Value]bool

// ConstantBooleanArguments binds the callee's parameters, and the captured
// variables of closure when the callee is its body, to the Boolean constants
// the call supplies: literals, or caller values that known already fixes.
// It returns nil when nothing is fixed.
func ConstantBooleanArguments(common *ssa.CallCommon, closure *ssa.MakeClosure, callee *ssa.Function, known BooleanConstants) BooleanConstants {
	if callee == nil || len(callee.Blocks) == 0 {
		return nil
	}
	var constants BooleanConstants
	bind := func(local, supplied ssa.Value) {
		if value, ok := constantBoolean(supplied, known); ok {
			if constants == nil {
				constants = BooleanConstants{}
			}
			constants[local] = value
		}
	}
	if common != nil && !common.IsInvoke() && len(common.Args) == len(callee.Params) {
		for index, argument := range common.Args {
			bind(callee.Params[index], argument)
		}
	}
	if closure != nil && closure.Fn == callee && len(closure.Bindings) == len(callee.FreeVars) {
		for index, binding := range closure.Bindings {
			if value, ok := capturedBoolean(binding, known); ok {
				if constants == nil {
					constants = BooleanConstants{}
				}
				constants[callee.FreeVars[index]] = value
			}
		}
	}
	return constants
}

// capturedBoolean reports the value every read of a captured Boolean cell
// yields: a cell written once with a fixed value, whose captures only read
// it, or a cell an enclosing closure already bound and passes on.
func capturedBoolean(binding ssa.Value, known BooleanConstants) (bool, bool) {
	switch cell := binding.(type) {
	case *ssa.FreeVar:
		value, ok := known[cell]
		return value, ok
	case *ssa.Alloc:
		stored, ok := WrittenOnceCell(cell)
		if !ok {
			return false, false
		}
		return constantBoolean(stored, known)
	}
	return false, false
}

// ConstantBooleanArgumentBits reports which of the call's arguments are
// Boolean constants and their values, as masks indexed by argument position
// with any receiver first. It serves summaries of bodies that are not
// available, whose parameters are known only by position.
func ConstantBooleanArgumentBits(common *ssa.CallCommon, known BooleanConstants) (bound, values uint64) {
	if common == nil || common.IsInvoke() {
		return 0, 0
	}
	for index, argument := range common.Args {
		if index >= 64 {
			break
		}
		if value, ok := constantBoolean(argument, known); ok {
			bound |= 1 << index
			if value {
				values |= 1 << index
			}
		}
	}
	return bound, values
}

func constantBoolean(value ssa.Value, known BooleanConstants) (bool, bool) {
	if literal, ok := value.(*ssa.Const); ok {
		if literal.Value != nil && literal.Value.Kind() == constant.Bool {
			return constant.BoolVal(literal.Value), true
		}
		return false, false
	}
	fixed, ok := known[value]
	return fixed, ok
}

// DecidedSuccessor returns the successor a block's branch takes when its
// condition is a bound value, possibly negated.
func (constants BooleanConstants) DecidedSuccessor(block *ssa.BasicBlock) (*ssa.BasicBlock, bool) {
	if len(constants) == 0 || len(block.Instrs) == 0 || len(block.Succs) != 2 {
		return nil, false
	}
	branch, ok := block.Instrs[len(block.Instrs)-1].(*ssa.If)
	if !ok {
		return nil, false
	}
	condition, negated := branch.Cond, false
	for {
		not, ok := condition.(*ssa.UnOp)
		if !ok || not.Op != token.NOT {
			break
		}
		condition, negated = not.X, !negated
	}
	// A load of a bound captured cell reads its one fixed value.
	if load, ok := condition.(*ssa.UnOp); ok && load.Op == token.MUL {
		if _, captured := load.X.(*ssa.FreeVar); captured {
			condition = load.X
		}
	}
	value, known := constants[condition]
	if !known {
		return nil, false
	}
	if value != negated {
		return block.Succs[0], true
	}
	return block.Succs[1], true
}

// Narrow keeps only the decided successor of block, if the bindings decide
// its branch and it is among successors.
func (constants BooleanConstants) Narrow(successors []*ssa.BasicBlock, block *ssa.BasicBlock) []*ssa.BasicBlock {
	taken, decided := constants.DecidedSuccessor(block)
	if !decided {
		return successors
	}
	if slices.Contains(successors, taken) {
		return []*ssa.BasicBlock{taken}
	}
	return nil
}

// Key renders the bindings of function's own parameters and captured
// variables in a stable order, so a memo can tell apart the same body
// searched under different constants.
func (constants BooleanConstants) Key(function *ssa.Function) string {
	if function == nil || len(constants) == 0 {
		return ""
	}
	var parts []string
	for index, parameter := range function.Params {
		if value, ok := constants[parameter]; ok {
			parts = append(parts, fmt.Sprintf("p%d=%t", index, value))
		}
	}
	for index, captured := range function.FreeVars {
		if value, ok := constants[captured]; ok {
			parts = append(parts, fmt.Sprintf("f%d=%t", index, value))
		}
	}
	return strings.Join(parts, ",")
}
