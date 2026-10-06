package calls_test

import (
	"go/types"
	"testing"

	ssacall "github.com/kojah/gohawk/internal/ssaflow/calls"
)

func TestCallConditionMatchesArgumentsAndNilness(t *testing.T) {
	nilFirst := ssacall.CallCondition{Nilness: ssacall.ArgumentConstants{Bound: 1, Values: 1}}
	flagSecond := ssacall.CallCondition{Arguments: ssacall.ArgumentConstants{Bound: 2, Values: 2}}
	errorNil := ssacall.CallCondition{Result: 1, Outcome: ssacall.OutcomeNil}
	for _, test := range []struct {
		name       string
		summarized ssacall.CallCondition
		query      ssacall.CallCondition
		want       bool
	}{
		{"nil parameter supplied", nilFirst, nilFirst, true},
		{"non-nil parameter supplied", nilFirst, ssacall.CallCondition{Nilness: ssacall.ArgumentConstants{Bound: 1}}, false},
		{"nilness not known at the call", nilFirst, ssacall.CallCondition{}, false},
		{"extra constants at the call", flagSecond, ssacall.CallCondition{Arguments: ssacall.ArgumentConstants{Bound: 3, Values: 2}}, true},
		{"same result test", errorNil, errorNil, true},
		{"other result test", errorNil, ssacall.CallCondition{Result: 1, Outcome: ssacall.OutcomeNonNil}, false},
		// A case without a result test holds on every return, so it answers one.
		{"unconditional result case", flagSecond, ssacall.CallCondition{Result: 1, Outcome: ssacall.OutcomeNil, Arguments: flagSecond.Arguments}, true},
	} {
		if got := test.summarized.Matches(test.query); got != test.want {
			t.Errorf("%s: Matches = %t, want %t", test.name, got, test.want)
		}
	}
}

func TestCallConditionNilnessNeedsANilableParameter(t *testing.T) {
	pointer := types.NewPointer(types.Typ[types.Int])
	parameters := types.NewTuple(
		types.NewParam(0, nil, "p", pointer),
		types.NewParam(0, nil, "n", types.Typ[types.Int]),
	)
	signature := types.NewSignatureType(nil, nil, nil, parameters, types.NewTuple(), false)
	if !(ssacall.CallCondition{Nilness: ssacall.ArgumentConstants{Bound: 1, Values: 1}}).ValidFor(signature) {
		t.Error("nilness of a pointer parameter should be valid")
	}
	if (ssacall.CallCondition{Nilness: ssacall.ArgumentConstants{Bound: 2}}).ValidFor(signature) {
		t.Error("nilness of an int parameter should be invalid")
	}
}

func TestCallConditionString(t *testing.T) {
	for _, test := range []struct {
		condition ssacall.CallCondition
		want      string
	}{
		{ssacall.CallCondition{}, "always"},
		{ssacall.CallCondition{Result: 1, Outcome: ssacall.OutcomeNonNil}, "result 1 is non-nil"},
		{ssacall.ParameterNil(2), "argument 2 is nil"},
		{
			ssacall.CallCondition{Arguments: ssacall.ArgumentConstants{Bound: 0b110, Values: 0b010}},
			"argument 1 is true and argument 2 is false",
		},
	} {
		if got := test.condition.String(); got != test.want {
			t.Errorf("String() = %q, want %q", got, test.want)
		}
	}
}
