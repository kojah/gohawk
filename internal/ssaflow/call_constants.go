package ssaflow

import (
	"fmt"
	"go/constant"
	"go/token"
	"slices"
	"strings"

	"golang.org/x/tools/go/ssa"
)

// Fixed arguments decide a callee's branches. A call that passes a Boolean
// literal, nil, a value that cannot be nil, or a value its own caller already
// fixed, decides every branch in the callee that tests that parameter, or
// the captured copy of it: the value itself or its negation for a Boolean,
// a comparison with nil for a nilable value. Any other comparison, a phi, or
// a value derived from the parameter stays with the ordinary feasibility
// view: only the exact bound value is decided, so a binding can remove an
// impossible path but never invent a possible one.

// FixedValues fixes parameters and captured variables of the bodies being
// searched to an outcome: true or false for a Boolean, nil or non-nil for a
// nilable value. A *ssa.Parameter key holds the value itself. A *ssa.FreeVar
// key is a captured cell, as Go captures every variable by reference, and
// holds the outcome of every load of the cell; it is bound only when the
// cell is written once before capture and every closure only reads it.
type FixedValues map[ssa.Value]Outcome

// FixedArguments binds the callee's parameters, and the captured variables
// of closure when the callee is its body, to the outcomes the call's
// arguments fix: literals, values that cannot be nil, and caller values that
// known already fixes. It returns nil when nothing is fixed.
func FixedArguments(common *ssa.CallCommon, closure *ssa.MakeClosure, callee *ssa.Function, known FixedValues) FixedValues {
	if callee == nil || len(callee.Blocks) == 0 {
		return nil
	}
	var fixed FixedValues
	bind := func(local ssa.Value, outcome Outcome) {
		if fixed == nil {
			fixed = FixedValues{}
		}
		fixed[local] = outcome
	}
	if common != nil && !common.IsInvoke() && len(common.Args) == len(callee.Params) {
		for index, argument := range common.Args {
			if outcome, ok := fixedOutcome(argument, known); ok && decidable(callee.Params[index], outcome) {
				bind(callee.Params[index], outcome)
			}
		}
	}
	if closure != nil && closure.Fn == callee && len(closure.Bindings) == len(callee.FreeVars) {
		for index, binding := range closure.Bindings {
			if outcome, ok := capturedOutcome(binding, known); ok && decidableCell(callee.FreeVars[index], outcome) {
				bind(callee.FreeVars[index], outcome)
			}
		}
	}
	return fixed
}

// decidable reports whether binding the parameter to outcome can decide
// anything. A Boolean always can. Nilness is bound only for a parameter the
// body compares with nil or captures, so a pointer argument that is never
// tested adds no binding, and the memo keys that name bindings stay few.
func decidable(parameter *ssa.Parameter, outcome Outcome) bool {
	if outcome == OutcomeTrue || outcome == OutcomeFalse {
		return true
	}
	return slices.ContainsFunc(*parameter.Referrers(), ComparesWithNil)
}

// decidableCell is decidable for a captured cell: nilness is bound only when
// a load of the cell is compared with nil.
func decidableCell(cell *ssa.FreeVar, outcome Outcome) bool {
	if outcome == OutcomeTrue || outcome == OutcomeFalse {
		return true
	}
	return slices.ContainsFunc(*cell.Referrers(), func(user ssa.Instruction) bool {
		load, ok := user.(*ssa.UnOp)
		return ok && load.Op == token.MUL && slices.ContainsFunc(*load.Referrers(), ComparesWithNil)
	})
}

// ComparesWithNil reports whether a use of a nilable value can decide a
// branch on its nilness: a comparison with nil, or a store into the cell a
// closure captures it by.
func ComparesWithNil(user ssa.Instruction) bool {
	switch typed := user.(type) {
	case *ssa.BinOp:
		return (typed.Op == token.EQL || typed.Op == token.NEQ) && (DefinitelyNil(typed.X) || DefinitelyNil(typed.Y))
	case *ssa.Store:
		_, cell := typed.Addr.(*ssa.Alloc)
		return cell
	}
	return false
}

// fixedOutcome reports what a value passed as an argument is known to be:
// a Boolean literal, nil, a value that is never nil, or a caller value that
// known fixes. An interface holding a typed nil pointer is not nil.
func fixedOutcome(value ssa.Value, known FixedValues) (Outcome, bool) {
	if outcome, ok := known[value]; ok {
		return outcome, true
	}
	if literal, ok := value.(*ssa.Const); ok {
		switch {
		case literal.Value != nil && literal.Value.Kind() == constant.Bool:
			if constant.BoolVal(literal.Value) {
				return OutcomeTrue, true
			}
			return OutcomeFalse, true
		case literal.IsNil():
			return OutcomeNil, true
		}
		return OutcomeAny, false
	}
	if neverNil(value) {
		return OutcomeNonNil, true
	}
	return OutcomeAny, false
}

// neverNil reports whether a value is non-nil by construction: a fresh
// allocation, a made map, slice, channel, or closure, a function, or an
// interface box, which has a dynamic type even around a nil pointer.
func neverNil(value ssa.Value) bool {
	switch value.(type) {
	case *ssa.Alloc, *ssa.MakeMap, *ssa.MakeSlice, *ssa.MakeChan, *ssa.MakeClosure, *ssa.MakeInterface, *ssa.Function:
		return Nilable(value.Type())
	}
	return false
}

// capturedOutcome reports the outcome every read of a captured cell yields:
// a cell written once with a fixed value, whose captures only read it, or a
// cell an enclosing closure already bound and passes on.
func capturedOutcome(binding ssa.Value, known FixedValues) (Outcome, bool) {
	switch cell := binding.(type) {
	case *ssa.FreeVar:
		outcome, ok := known[cell]
		return outcome, ok
	case *ssa.Alloc:
		stored, ok := WrittenOnceCell(cell)
		if !ok {
			return OutcomeAny, false
		}
		return fixedOutcome(stored, known)
	}
	return OutcomeAny, false
}

// DecidedSuccessor returns the successor a block's branch takes when its
// condition is a bound Boolean value, possibly negated, or a comparison of a
// bound nilable value with nil.
func (fixed FixedValues) DecidedSuccessor(block *ssa.BasicBlock) (*ssa.BasicBlock, bool) {
	if len(fixed) == 0 || len(block.Instrs) == 0 || len(block.Succs) != 2 {
		return nil, false
	}
	branch, ok := block.Instrs[len(block.Instrs)-1].(*ssa.If)
	if !ok {
		return nil, false
	}
	holds, decided := fixed.Holds(branch.Cond)
	if !decided {
		return nil, false
	}
	if holds {
		return block.Succs[0], true
	}
	return block.Succs[1], true
}

// Holds decides a Boolean value from the bindings: a bound Boolean, possibly
// negated, or a bound nilable value compared with nil.
func (fixed FixedValues) Holds(condition ssa.Value) (holds, decided bool) {
	negated := false
	for {
		not, ok := condition.(*ssa.UnOp)
		if !ok || not.Op != token.NOT {
			break
		}
		condition, negated = not.X, !negated
	}
	if comparison, ok := condition.(*ssa.BinOp); ok && (comparison.Op == token.EQL || comparison.Op == token.NEQ) {
		operand := comparison.X
		if !DefinitelyNil(comparison.Y) {
			if !DefinitelyNil(comparison.X) {
				return false, false
			}
			operand = comparison.Y
		}
		outcome, known := fixed[boundKey(operand)]
		if !known || outcome != OutcomeNil && outcome != OutcomeNonNil {
			return false, false
		}
		equal := outcome == OutcomeNil
		return equal == (comparison.Op == token.EQL) != negated, true
	}
	outcome, known := fixed[boundKey(condition)]
	if !known || outcome != OutcomeTrue && outcome != OutcomeFalse {
		return false, false
	}
	return (outcome == OutcomeTrue) != negated, true
}

// boundKey names the binding a value reads: the value itself, or the
// captured cell a load reads.
func boundKey(value ssa.Value) ssa.Value {
	if load, ok := value.(*ssa.UnOp); ok && load.Op == token.MUL {
		if _, captured := load.X.(*ssa.FreeVar); captured {
			return load.X
		}
	}
	return value
}

// Narrow keeps only the decided successor of block, if the bindings decide
// its branch and it is among successors.
func (fixed FixedValues) Narrow(successors []*ssa.BasicBlock, block *ssa.BasicBlock) []*ssa.BasicBlock {
	taken, decided := fixed.DecidedSuccessor(block)
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
// searched under different bindings.
func (fixed FixedValues) Key(function *ssa.Function) string {
	if function == nil || len(fixed) == 0 {
		return ""
	}
	var parts []string
	for index, parameter := range function.Params {
		if outcome, ok := fixed[parameter]; ok {
			parts = append(parts, fmt.Sprintf("p%d=%d", index, outcome))
		}
	}
	for index, captured := range function.FreeVars {
		if outcome, ok := fixed[captured]; ok {
			parts = append(parts, fmt.Sprintf("f%d=%d", index, outcome))
		}
	}
	return strings.Join(parts, ",")
}
