package ssaflow

import (
	"go/constant"
	"go/types"
	"math/bits"

	"github.com/kojah/gohawk/internal/syntax"
	"golang.org/x/tools/go/ssa"
)

// A call condition is something a caller can check about one call: an
// outcome of one of its results, Boolean arguments it passes as constants,
// or both. It is the serializable, positional condition of a summary case,
// so a claim proven in one package is selected at a call in another;
// BooleanConstants is the same argument condition bound to one body's SSA
// values for a proof. Every summary that holds under a condition names it
// with this type, so there is one vocabulary to prove, export, and match.

// ResultOutcome names a value a call's result can be tested for. OutcomeAny
// places no condition on any result.
type ResultOutcome uint8

const (
	OutcomeAny ResultOutcome = iota
	OutcomeTrue
	OutcomeFalse
	OutcomeNil
	OutcomeNonNil
)

// ArgumentConstants names Boolean parameters by position, receiver first,
// and the constant each holds. In a summarized case it is what the case
// assumes; in a query it is what the call supplies. Positions past 63 are
// never bound.
type ArgumentConstants struct {
	Bound  uint64
	Values uint64
}

// Satisfies reports whether the supplied constants fix every argument the
// assumed constants name, to the same value.
func (supplied ArgumentConstants) Satisfies(assumed ArgumentConstants) bool {
	return assumed.Bound&^supplied.Bound == 0 && (assumed.Values^supplied.Values)&assumed.Bound == 0
}

// SuppliedConstants reports the Boolean constants a call passes: literals,
// and caller values that known already fixes.
func SuppliedConstants(common *ssa.CallCommon, known BooleanConstants) ArgumentConstants {
	var supplied ArgumentConstants
	if common == nil || common.IsInvoke() {
		return supplied
	}
	for index, argument := range common.Args {
		if index >= 64 {
			break
		}
		if value, ok := constantBoolean(argument, known); ok {
			supplied.Bound |= 1 << index
			if value {
				supplied.Values |= 1 << index
			}
		}
	}
	return supplied
}

// Bindings maps the assumed constants onto function's Boolean parameters. It
// reports false when a bound position is not a Boolean parameter.
func (assumed ArgumentConstants) Bindings(function *ssa.Function) (BooleanConstants, bool) {
	if assumed.Bound == 0 {
		return nil, true
	}
	constants := BooleanConstants{}
	for index, parameter := range function.Params {
		if index >= 64 || assumed.Bound&(1<<index) == 0 {
			continue
		}
		if basic, ok := parameter.Type().Underlying().(*types.Basic); !ok || basic.Kind() != types.Bool {
			return nil, false
		}
		constants[parameter] = assumed.Values&(1<<index) != 0
	}
	return constants, len(constants) == bits.OnesCount64(assumed.Bound)
}

// CallCondition is one summary case's condition: result Result has Outcome,
// unless Outcome is OutcomeAny, and the call supplies Arguments. The zero
// value is unconditional.
type CallCondition struct {
	Result    int
	Outcome   ResultOutcome
	Arguments ArgumentConstants
}

// Unconditional reports whether the condition constrains nothing.
func (condition CallCondition) Unconditional() bool {
	return condition.Outcome == OutcomeAny && condition.Arguments.Bound == 0
}

// Matches reports whether a summarized case answers query: constants the
// query's call supplies, and the same result condition. A case with no result
// condition holds on every normal return, so it answers any result condition
// too.
func (summarized CallCondition) Matches(query CallCondition) bool {
	if !query.Arguments.Satisfies(summarized.Arguments) {
		return false
	}
	return summarized.Outcome == OutcomeAny || summarized.Result == query.Result && summarized.Outcome == query.Outcome
}

// ValidFor reports whether the condition's result test fits signature: a
// Boolean outcome on a Boolean result, a nil outcome on an error result.
func (condition CallCondition) ValidFor(signature *types.Signature) bool {
	if signature == nil || condition.Result < 0 {
		return false
	}
	if condition.Outcome == OutcomeAny {
		return condition.Result == 0
	}
	if condition.Result >= signature.Results().Len() {
		return false
	}
	result := signature.Results().At(condition.Result).Type()
	switch condition.Outcome {
	case OutcomeTrue, OutcomeFalse:
		basic, ok := result.Underlying().(*types.Basic)
		return ok && basic.Kind() == types.Bool
	case OutcomeNil, OutcomeNonNil:
		return syntax.IsErrorType(result)
	case OutcomeAny:
	}
	return false
}

// OutcomeOf decides whether value, returned in a result slot, has outcome.
// Only constants and interface boxes decide it: even a boxed nil pointer has
// a dynamic type and is not a nil interface.
func OutcomeOf(outcome ResultOutcome, value ssa.Value) (holds, known bool) {
	if _, boxed := value.(*ssa.MakeInterface); boxed && (outcome == OutcomeNil || outcome == OutcomeNonNil) {
		return outcome == OutcomeNonNil, true
	}
	literal, ok := value.(*ssa.Const)
	if !ok {
		return false, false
	}
	switch outcome {
	case OutcomeTrue, OutcomeFalse:
		if literal.Value != nil && literal.Value.Kind() == constant.Bool {
			return constant.BoolVal(literal.Value) == (outcome == OutcomeTrue), true
		}
	case OutcomeNil, OutcomeNonNil:
		if literal.IsNil() {
			return outcome == OutcomeNil, true
		}
	case OutcomeAny:
	}
	return false, false
}
