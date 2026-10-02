package ssaflow

import (
	"fmt"
	"go/constant"
	"go/token"
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
// cell is written once before capture and every closure only reads it. A
// caller may also key the *ssa.Alloc cell it passes to a closure, to say
// what every load of the captured copy reads when the closure runs, as a
// named result does once a return has set it before the deferred calls; the
// binding holds only when the closure never writes the cell.
type FixedValues map[ssa.Value]Outcome

// FixedArgumentsProof publishes only a complete census of modeled argument
// and capture outcomes. Proven does not establish any callee behavior.
type FixedArgumentsProof struct {
	Proof
	Values FixedValues
}

// ProveFixedArgumentsWithin binds parameters and captured cells to literal or
// caller-fixed outcomes. Capture identity, read-only and nil-test searches
// share budget. Cutoff publishes no map; nil budget retains default binding
// policy. Missing bodies yield a completed empty metadata census.
func ProveFixedArgumentsWithin(
	common *ssa.CallCommon, closure *ssa.MakeClosure, callee *ssa.Function, known FixedValues, budget *SearchBudget,
) FixedArgumentsProof {
	if callee == nil || len(callee.Blocks) == 0 {
		return FixedArgumentsProof{Proof: Proof{State: EvidenceProven, Reason: EvidenceStructuralWalk}}
	}
	unknown := FixedArgumentsProof{Proof: Proof{Reason: EvidenceBudgetExhausted}}
	if !budget.Spend() {
		return unknown
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
			if !budget.Spend() {
				return unknown
			}
			if outcome, ok := fixedOutcome(argument, known); ok && decidableWithin(callee.Params[index], outcome, budget) {
				bind(callee.Params[index], outcome)
			}
		}
	}
	if closure != nil && closure.Fn == callee && len(closure.Bindings) == len(callee.FreeVars) {
		for pair := range ClosureBindingPairsWithin(callee, closure, budget) {
			if outcome, ok := capturedArgumentOutcomeWithin(pair, known, budget); ok {
				bind(pair.Free, outcome)
			}
		}
	}
	if budget.Exhausted() || budget.PoolExhausted() {
		return unknown
	}
	return FixedArgumentsProof{Proof: Proof{State: EvidenceProven, Reason: EvidenceStructuralWalk, Provenance: EvidenceFromLocalSSA}, Values: fixed}
}

// A caller-fixed current cell remains valid only in a directly read-only
// body. Otherwise retain the once-stored fallback, including nested lexical
// captures, and its existing nilness relevance rule.
func capturedArgumentOutcomeWithin(pair CapturedBinding, known FixedValues, budget *SearchBudget) (Outcome, bool) {
	if outcome, ok := known[pair.Binding]; ok && onlyReadWithin(pair.Free, budget) {
		return outcome, true
	}
	outcome, ok := capturedOutcomeWithin(pair.Binding, known, budget)
	if !ok || !decidableCellWithin(pair.Free, outcome, budget) {
		return OutcomeAny, false
	}
	return outcome, true
}

// Boolean bindings are always useful; nilness is retained only for an exact
// nil comparison or capture store, keeping unrelated pointer states out of
// callee contexts. These predicates select relevance, not outcome truth.
func decidableWithin(parameter *ssa.Parameter, outcome Outcome, budget *SearchBudget) bool {
	if outcome == OutcomeTrue || outcome == OutcomeFalse {
		return true
	}
	for user := range ReferrersWithin(parameter, budget) {
		if comparesWithNilWithin(user, budget) {
			return true
		}
	}
	return false
}

func decidableCellWithin(cell *ssa.FreeVar, outcome Outcome, budget *SearchBudget) bool {
	if outcome == OutcomeTrue || outcome == OutcomeFalse {
		return true
	}
	for user := range ReferrersWithin(cell, budget) {
		if load, ok := user.(*ssa.UnOp); ok && load.Op == token.MUL {
			for use := range ReferrersWithin(load, budget) {
				if comparesWithNilWithin(use, budget) {
					return true
				}
			}
		}
	}
	return false
}

// Only direct loads preserve a caller-fixed captured cell's outcome inside
// this body. Nested captures retain the ordinary once-stored fallback.
func onlyReadWithin(cell *ssa.FreeVar, budget *SearchBudget) bool {
	for user := range ReferrersWithin(cell, budget) {
		if load, ok := user.(*ssa.UnOp); !ok || load.Op != token.MUL {
			return false
		}
	}
	return !budget.Exhausted() && !budget.PoolExhausted()
}

// ValueOutcome reports what a value is known to be on its own: a Boolean
// literal, nil, or a value that is never nil. An interface holding a typed
// nil pointer is not nil.
func ValueOutcome(value ssa.Value) (Outcome, bool) {
	return fixedOutcome(value, nil)
}

// ComparesWithNil reports whether a use is a nil comparison or a store into
// a captured cell. It retains the default relevance query for summary setup.
func ComparesWithNil(user ssa.Instruction) bool {
	return comparesWithNilWithin(user, nil)
}

func comparesWithNilWithin(user ssa.Instruction, budget *SearchBudget) bool {
	switch typed := user.(type) {
	case *ssa.BinOp:
		return (typed.Op == token.EQL || typed.Op == token.NEQ) && (DefinitelyNilWithin(typed.X, budget) || DefinitelyNilWithin(typed.Y, budget))
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

// capturedOutcomeWithin reports the outcome every read of a captured cell yields:
// a cell written once with a fixed value, whose captures only read it, or a
// cell an enclosing closure already bound and passes on.
func capturedOutcomeWithin(binding ssa.Value, known FixedValues, budget *SearchBudget) (Outcome, bool) {
	switch cell := binding.(type) {
	case *ssa.FreeVar:
		outcome, ok := known[cell]
		return outcome, ok
	case *ssa.Alloc:
		stored, ok := WrittenOnceCellWithin(cell, budget)
		if !ok {
			return OutcomeAny, false
		}
		return fixedOutcome(stored, known)
	}
	return OutcomeAny, false
}

// DecidedSuccessorWithin returns the successor a block's branch takes when its
// condition is a bound Boolean value, possibly negated, or a comparison of a
// bound nilable value with nil. Cutoff supplies no decided successor.
func (fixed FixedValues) DecidedSuccessorWithin(block *ssa.BasicBlock, budget *SearchBudget) (*ssa.BasicBlock, bool) {
	if len(fixed) == 0 || len(block.Instrs) == 0 || len(block.Succs) != 2 {
		return nil, false
	}
	branch, ok := block.Instrs[len(block.Instrs)-1].(*ssa.If)
	if !ok {
		return nil, false
	}
	holds, decided := fixed.HoldsWithin(branch.Cond, budget)
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
	return fixed.HoldsWithin(condition, nil)
}

// HoldsWithin shares negation and nil-fold visits with the caller allowance.
// Exhaustion cannot decide a bound condition; a nil budget retains defaults.
func (fixed FixedValues) HoldsWithin(condition ssa.Value, budget *SearchBudget) (holds, decided bool) {
	condition, negated := booleanNegationSourceWithin(condition, budget)
	if budget.Exhausted() {
		return false, false
	}
	if comparison, ok := condition.(*ssa.BinOp); ok && (comparison.Op == token.EQL || comparison.Op == token.NEQ) {
		return fixed.nilComparisonWithin(comparison, negated, budget)
	}
	outcome, known := fixed[boundKey(condition)]
	if !known || outcome != OutcomeTrue && outcome != OutcomeFalse {
		return false, false
	}
	return (outcome == OutcomeTrue) != negated, true
}

func (fixed FixedValues) nilComparisonWithin(comparison *ssa.BinOp, negated bool, budget *SearchBudget) (bool, bool) {
	operand := comparison.X
	rightNil := DefinitelyNilWithin(comparison.Y, budget)
	if budget.Exhausted() {
		return false, false
	}
	if !rightNil {
		if !DefinitelyNilWithin(comparison.X, budget) {
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

// BooleanNegationSource returns the operand behind a chain of SSA Boolean NOT
// instructions and whether an odd number of negations reverses its truth. It
// stops at every other form, including loads, conversions, comparisons and phi
// merges; it neither evaluates the operand nor establishes its stability.
func BooleanNegationSource(value ssa.Value) (ssa.Value, bool) {
	return booleanNegationSourceWithin(value, nil)
}

func booleanNegationSourceWithin(value ssa.Value, budget *SearchBudget) (ssa.Value, bool) {
	negated := false
	for {
		if !budget.Spend() {
			return nil, false
		}
		not, ok := value.(*ssa.UnOp)
		if !ok || not.Op != token.NOT {
			return value, negated
		}
		value, negated = not.X, !negated
	}
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
	return fixed.NarrowWithin(successors, block, nil)
}

// NarrowWithin shares bound-condition and successor-filter visits. Cutoff
// keeps the primitive's input edges; callers retain availability before use.
func (fixed FixedValues) NarrowWithin(successors []*ssa.BasicBlock, block *ssa.BasicBlock, budget *SearchBudget) []*ssa.BasicBlock {
	taken, decided := fixed.DecidedSuccessorWithin(block, budget)
	if !decided {
		return successors
	}
	kept := keepSuccessorWithin(successors, taken, budget)
	if budget.Exhausted() {
		return successors
	}
	return kept
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
