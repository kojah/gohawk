package ssaflow

import (
	"fmt"
	"go/constant"
	"go/types"
	"math/bits"
	"strings"

	"github.com/kojah/gohawk/internal/syntax"
	"golang.org/x/tools/go/ssa"
)

// A call condition is something a caller can check about one call: an
// outcome of one of its results, Boolean arguments it passes as constants,
// arguments known to be nil or non-nil, or any combination. It is the
// serializable, positional condition of a summary case, so a claim proven in
// one package is selected at a call in another;
// FixedValues is the same argument condition bound to one body's SSA
// values for a proof. Every summary that holds under a condition names it
// with this type, so there is one vocabulary to prove, export, and match.

// Outcome names a value a call's result can be tested for. OutcomeAny
// places no condition on any result.
type Outcome uint8

const (
	OutcomeAny Outcome = iota
	OutcomeTrue
	OutcomeFalse
	OutcomeNil
	OutcomeNonNil
)

// ArgumentConstants names parameters by position, receiver first, and one
// bit of what each holds: its Boolean value in a condition's Arguments, or
// whether it is nil in its Nilness. In a summarized case it is what the case
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

// SuppliedCondition reports what a call's arguments fix, as a condition a
// summarized case can be matched against: the Boolean constants it passes,
// and the arguments it passes as nil or as values that are never nil,
// including caller values that known already fixes.
func SuppliedCondition(common *ssa.CallCommon, known FixedValues) CallCondition {
	var supplied CallCondition
	if common == nil || common.IsInvoke() {
		return supplied
	}
	for index, argument := range common.Args {
		if index >= 64 {
			break
		}
		outcome, ok := fixedOutcome(argument, known)
		if !ok {
			continue
		}
		switch outcome {
		case OutcomeTrue, OutcomeFalse:
			supplied.Arguments.Bound |= 1 << index
			if outcome == OutcomeTrue {
				supplied.Arguments.Values |= 1 << index
			}
		case OutcomeNil, OutcomeNonNil:
			supplied.Nilness.Bound |= 1 << index
			if outcome == OutcomeNil {
				supplied.Nilness.Values |= 1 << index
			}
		case OutcomeAny:
		}
	}
	return supplied
}

// Bindings maps the condition's argument assumptions onto function's
// parameters: the Boolean constants onto Boolean parameters, the nilness onto
// nilable ones. It reports false when a position does not fit its kind.
func (condition CallCondition) Bindings(function *ssa.Function) (FixedValues, bool) {
	if condition.Arguments.Bound == 0 && condition.Nilness.Bound == 0 {
		return nil, true
	}
	fixed := FixedValues{}
	for index, parameter := range function.Params {
		if index >= 64 {
			break
		}
		bit := uint64(1) << index
		if condition.Arguments.Bound&bit != 0 {
			if basic, ok := parameter.Type().Underlying().(*types.Basic); !ok || basic.Kind() != types.Bool {
				return nil, false
			}
			fixed[parameter] = OutcomeFalse
			if condition.Arguments.Values&bit != 0 {
				fixed[parameter] = OutcomeTrue
			}
		}
		if condition.Nilness.Bound&bit != 0 {
			if !Nilable(parameter.Type()) || condition.Arguments.Bound&bit != 0 {
				return nil, false
			}
			fixed[parameter] = OutcomeNonNil
			if condition.Nilness.Values&bit != 0 {
				fixed[parameter] = OutcomeNil
			}
		}
	}
	return fixed, len(fixed) == bits.OnesCount64(condition.Arguments.Bound|condition.Nilness.Bound)
}

// CallCondition is one summary case's condition: result Result has Outcome,
// unless Outcome is OutcomeAny, the call supplies the Boolean Arguments, and
// the arguments Nilness names are nil where its value bit is set and non-nil
// where it is clear. The zero value is unconditional.
type CallCondition struct {
	Result    int
	Outcome   Outcome
	Arguments ArgumentConstants
	Nilness   ArgumentConstants
}

// ParameterNil is the condition that the parameter at index, receiver first,
// is nil.
func ParameterNil(index int) CallCondition {
	return CallCondition{Nilness: ArgumentConstants{Bound: 1 << index, Values: 1 << index}}
}

// Unconditional reports whether the condition constrains nothing.
func (condition CallCondition) Unconditional() bool {
	return condition.Outcome == OutcomeAny && condition.Arguments.Bound == 0 && condition.Nilness.Bound == 0
}

// Matches reports whether a summarized case answers query: constants the
// query's call supplies, and the same result condition. A case with no result
// condition holds on every normal return, so it answers any result condition
// too.
func (summarized CallCondition) Matches(query CallCondition) bool {
	if !query.Arguments.Satisfies(summarized.Arguments) || !query.Nilness.Satisfies(summarized.Nilness) {
		return false
	}
	return summarized.Outcome == OutcomeAny || summarized.Result == query.Result && summarized.Outcome == query.Outcome
}

// ValidFor reports whether the condition fits signature: a Boolean outcome on
// a Boolean result, a nil outcome on an error result, and nilness only of
// nilable parameters.
func (condition CallCondition) ValidFor(signature *types.Signature) bool {
	if signature == nil || condition.Result < 0 || !nilnessFits(condition.Nilness, signature) {
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
func OutcomeOf(outcome Outcome, value ssa.Value) (holds, known bool) {
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

// nilnessFits reports whether every position the nilness condition names is
// a nilable parameter, counting the receiver first as SSA does.
func nilnessFits(nilness ArgumentConstants, signature *types.Signature) bool {
	var parameters []types.Type
	if signature.Recv() != nil {
		parameters = append(parameters, signature.Recv().Type())
	}
	for parameter := range signature.Params().Variables() {
		parameters = append(parameters, parameter.Type())
	}
	for index := range 64 {
		if nilness.Bound&(1<<index) == 0 {
			continue
		}
		if index >= len(parameters) || !Nilable(parameters[index]) {
			return false
		}
	}
	return true
}

// Nilable reports whether a value of the type can be nil.
func Nilable(value types.Type) bool {
	switch value.Underlying().(type) {
	case *types.Pointer, *types.Interface, *types.Map, *types.Slice, *types.Chan, *types.Signature:
		return true
	}
	return false
}

// String renders the condition for fact dumps and traces: each result test
// and argument assumption by position, receiver first, or "always".
func (condition CallCondition) String() string {
	var parts []string
	if condition.Outcome != OutcomeAny {
		parts = append(parts, fmt.Sprintf("result %d is %s", condition.Result, condition.Outcome))
	}
	for index := range 64 {
		bit := uint64(1) << index
		if condition.Arguments.Bound&bit != 0 {
			parts = append(parts, fmt.Sprintf("argument %d is %t", index, condition.Arguments.Values&bit != 0))
		}
		if condition.Nilness.Bound&bit != 0 {
			state := "non-nil"
			if condition.Nilness.Values&bit != 0 {
				state = "nil"
			}
			parts = append(parts, fmt.Sprintf("argument %d is %s", index, state))
		}
	}
	if len(parts) == 0 {
		return "always"
	}
	return strings.Join(parts, " and ")
}

// String names the outcome.
func (outcome Outcome) String() string {
	switch outcome {
	case OutcomeTrue:
		return "true"
	case OutcomeFalse:
		return "false"
	case OutcomeNil:
		return "nil"
	case OutcomeNonNil:
		return "non-nil"
	case OutcomeAny:
	}
	return "any"
}
