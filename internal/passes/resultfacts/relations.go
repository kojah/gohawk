package resultfacts

import (
	"go/types"

	"github.com/kojah/gohawk/internal/lifecycle"
	"github.com/kojah/gohawk/internal/ssaflow"

	"golang.org/x/tools/go/ssa"
)

// Result cases tie one result's outcome to a condition a caller can check:
// a Boolean predicate that reports an error's nilness is false whenever that
// error is nil, and a value is present exactly when its paired error is
// absent. Each case is proven over every normal return under its condition,
// uses the shared ssaflow.CallCondition vocabulary, and is exported beside
// the unconditional guarantees. A missing case says nothing; it never
// establishes the opposite implication. A returned parameter is not a case:
// it holds on every return and relates identities, not outcomes.

// ResultCase is one proven implication: result Result has Outcome on every
// normal return where Condition holds.
type ResultCase struct {
	Condition ssaflow.CallCondition
	Result    int
	Outcome   ssaflow.Outcome
}

// ReturnedParameter records that result Result is parameter Parameter itself,
// under the same static type, on every normal return. A builder returning its
// receiver and a pass-through wrapper have this shape; a caller may then
// treat the result as the argument it passed.
type ReturnedParameter struct {
	Result    int
	Parameter int
}

// Implies reports whether some proven case answers query: result has
// outcome wherever the query's condition holds.
func (summary Summary) Implies(query ssaflow.CallCondition, result int, outcome ssaflow.Outcome) bool {
	for _, proven := range summary.cases {
		if proven.Result == result && proven.Outcome == outcome && proven.Condition.Matches(query) {
			return true
		}
	}
	return false
}

// Cases returns every proven result case.
func (summary Summary) Cases() []ResultCase {
	return summary.cases
}

// ReturnedParameter returns the parameter that result is proven to be.
func (summary Summary) ReturnedParameter(result int) (int, bool) {
	for _, returned := range summary.returned {
		if returned.Result == result {
			return returned.Parameter, true
		}
	}
	return 0, false
}

// errorOutcome is the condition that the error result at index has outcome.
func errorOutcome(index int, outcome ssaflow.Outcome) ssaflow.CallCondition {
	return ssaflow.CallCondition{Result: index, Outcome: outcome}
}

func (engine *Engine) relations(function *ssa.Function, budget *ssaflow.SearchBudget) ([]ResultCase, []ReturnedParameter) {
	var cases []ResultCase
	var returned []ReturnedParameter
	results := function.Signature.Results()
	for result := range results.Len() {
		resultType := results.At(result).Type()
		for index, parameter := range function.Params {
			if budget.Spend() && types.Identical(parameter.Type(), resultType) && lifecycle.ReturnsParameterUnchanged(function, parameter, result) {
				returned = append(returned, ReturnedParameter{Result: result, Parameter: index})
			}
		}
		if isBoolean(resultType) {
			for index, parameter := range function.Params {
				if index >= 64 || !ssaflow.Nilable(parameter.Type()) {
					continue
				}
				if engine.parameterRelation(function, result, parameter, true, AlwaysFalse, budget) {
					cases = append(cases, ResultCase{Condition: ssaflow.ParameterNil(index), Result: result, Outcome: ssaflow.OutcomeFalse})
				}
			}
		}
		if !ssaflow.Nilable(resultType) {
			continue
		}
		for operand := range results.Len() {
			if operand == result || !types.Identical(results.At(operand).Type(), types.Universe.Lookup("error").Type()) {
				continue
			}
			for _, paired := range pairedNilness {
				if engine.resultRelation(function, result, operand, paired, budget) {
					cases = append(cases, ResultCase{Condition: errorOutcome(operand, paired.errorOutcome), Result: result, Outcome: paired.resultOutcome})
				}
			}
		}
	}
	return cases, returned
}

// pairedCase is one implication between a nilable result and its paired
// error: where the error has errorOutcome, the result has resultOutcome.
type pairedCase struct {
	errorOutcome  ssaflow.Outcome
	resultOutcome ssaflow.Outcome
}

var pairedNilness = []pairedCase{
	{errorOutcome: ssaflow.OutcomeNil, resultOutcome: ssaflow.OutcomeNonNil},
	{errorOutcome: ssaflow.OutcomeNonNil, resultOutcome: ssaflow.OutcomeNil},
}

// parameterRelation walks the body under the assumption that parameter is
// nil (or non-nil) and requires the result to be the expected literal on
// every reachable normal return. Only a comparison of the exact formal prunes
// a branch: a wrapped error may be non-nil when its source is nil, and a
// spilled cell may have been reassigned.
// https://github.com/norwoodj/helm-docs/blob/a5573af096a4b526dcbc3c896c220b1714a0765b/pkg/helm/chart_info.go#L94-L106
func (engine *Engine) parameterRelation(
	function *ssa.Function, result int, parameter *ssa.Parameter, assumeNil bool, expected Guarantee, budget *ssaflow.SearchBudget,
) bool {
	assumed := ssaflow.FixedValues{parameter: ssaflow.OutcomeNonNil}
	if assumeNil {
		assumed[parameter] = ssaflow.OutcomeNil
	}
	valid, witness := true, false
	ssaflow.WalkStates([]*ssa.BasicBlock{function.Blocks[0]}, func(block *ssa.BasicBlock) *ssa.BasicBlock { return block },
		func(block *ssa.BasicBlock) ([]*ssa.BasicBlock, bool) {
			for _, instruction := range block.Instrs {
				if !budget.Spend() {
					valid = false
					return nil, false
				}
				returned, ok := instruction.(*ssa.Return)
				if !ok {
					continue
				}
				witness = true
				if result >= len(returned.Results) || engine.assumedValue(returned.Results[result], assumed, budget) != expected {
					valid = false
					return nil, false
				}
			}
			return assumed.Narrow(block.Succs, block), true
		})
	return valid && witness && !budget.Exhausted()
}

// assumedValue resolves a Boolean result under the assumed nilness of the
// exact parameter: a comparison of that parameter with nil is decided by the
// assumption, and anything else falls back to the unconditional guarantee.
func (engine *Engine) assumedValue(value ssa.Value, assumed ssaflow.FixedValues, budget *ssaflow.SearchBudget) Guarantee {
	result, ok := ssaflow.ResolveReachingValue(
		ssaflow.NewReachingWalk(ssaflow.TransparentChangeType), value,
		func(_ ssaflow.ReachingWalk, leaf ssa.Value) (Guarantee, bool) {
			if holds, decided := assumed.Holds(leaf); decided {
				if holds {
					return AlwaysTrue, true
				}
				return AlwaysFalse, true
			}
			guarantee := engine.leaf(leaf, budget)
			return guarantee, guarantee != Unknown
		},
		func(guarantee Guarantee) Guarantee { return guarantee },
	)
	if !ok || budget.Exhausted() {
		return Unknown
	}
	return result
}

// resultRelation checks every normal return: where the error operand is
// known nil the result must be known non-nil, and where it is known non-nil
// the result must be known nil, depending on kind. A return that forwards
// both positions from one call inherits that callee's relation. A return
// whose error nilness is unknown proves nothing, and at least one return
// must witness the assumed side.
func (engine *Engine) resultRelation(function *ssa.Function, result, operand int, kind pairedCase, budget *ssaflow.SearchBudget) bool {
	witness := false
	for _, block := range function.Blocks {
		for _, instruction := range block.Instrs {
			if !budget.Spend() {
				return false
			}
			returned, ok := instruction.(*ssa.Return)
			if !ok || result >= len(returned.Results) || operand >= len(returned.Results) {
				continue
			}
			holds, witnessed := engine.returnHolds(returned.Results[result], returned.Results[operand], kind, budget)
			if !holds {
				return false
			}
			witness = witness || witnessed
		}
	}
	return witness && !budget.Exhausted()
}

// returnHolds judges one return: whether it keeps the relation, and whether it
// witnesses it rather than holding only vacuously, as a return whose error is
// nil does for a claim about non-nil errors.
func (engine *Engine) returnHolds(resultValue, errorValue ssa.Value, kind pairedCase, budget *ssaflow.SearchBudget) (bool, bool) {
	switch engine.value(errorValue, budget) {
	case AlwaysNil:
		if kind.errorOutcome == ssaflow.OutcomeNil {
			return engine.value(resultValue, budget) == AlwaysNonNil, true
		}
		return true, false
	case AlwaysNonNil:
		if kind.errorOutcome == ssaflow.OutcomeNonNil {
			return engine.value(resultValue, budget) == AlwaysNil, true
		}
		return true, false
	case Unknown, AlwaysTrue, AlwaysFalse:
		// An error of unknown nilness is decided below by the result alone
		// or by a forwarded pair.
	}
	// A return whose result already has the implied nilness holds the
	// implication whatever its error: return nil, err after a failed call is
	// nil where the error is non-nil.
	if resultSatisfies(engine.value(resultValue, budget), kind) {
		return true, true
	}
	callee, resultIndex, operandIndex, forwarded := forwardedPair(resultValue, errorValue)
	if !forwarded || !engine.function(callee, budget).Implies(errorOutcome(operandIndex, kind.errorOutcome), resultIndex, kind.resultOutcome) {
		return false, false
	}
	return true, true
}

// resultSatisfies reports whether a result of this guarantee meets the
// relation's consequent on its own.
func resultSatisfies(guarantee Guarantee, kind pairedCase) bool {
	return kind.resultOutcome == ssaflow.OutcomeNil && guarantee == AlwaysNil || kind.resultOutcome == ssaflow.OutcomeNonNil && guarantee == AlwaysNonNil
}

// forwardedPair resolves a return that passes two results of one call
// through unchanged, so the callee's relation between them carries over.
func forwardedPair(resultValue, errorValue ssa.Value) (*ssa.Function, int, int, bool) {
	resultCall, resultIndex, ok := ssaflow.CallResultSource(resultValue)
	if !ok {
		return nil, 0, 0, false
	}
	errorCall, errorIndex, ok := ssaflow.CallResultSource(errorValue)
	if !ok || errorCall != resultCall {
		return nil, 0, 0, false
	}
	callee := ssaflow.ResolvedCallee(resultCall.Common())
	return callee, resultIndex, errorIndex, callee != nil
}

func isBoolean(value types.Type) bool {
	basic, ok := value.Underlying().(*types.Basic)
	return ok && basic.Kind() == types.Bool
}
