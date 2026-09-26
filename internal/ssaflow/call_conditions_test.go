package ssaflow_test

import (
	"go/types"
	"testing"

	"github.com/kojah/gohawk/internal/ssaflow"
)

func TestCallConditionMatchesArgumentsAndNilness(t *testing.T) {
	nilFirst := ssaflow.CallCondition{Nilness: ssaflow.ArgumentConstants{Bound: 1, Values: 1}}
	flagSecond := ssaflow.CallCondition{Arguments: ssaflow.ArgumentConstants{Bound: 2, Values: 2}}
	errorNil := ssaflow.CallCondition{Result: 1, Outcome: ssaflow.OutcomeNil}
	for _, test := range []struct {
		name       string
		summarized ssaflow.CallCondition
		query      ssaflow.CallCondition
		want       bool
	}{
		{"nil parameter supplied", nilFirst, nilFirst, true},
		{"non-nil parameter supplied", nilFirst, ssaflow.CallCondition{Nilness: ssaflow.ArgumentConstants{Bound: 1}}, false},
		{"nilness not known at the call", nilFirst, ssaflow.CallCondition{}, false},
		{"extra constants at the call", flagSecond, ssaflow.CallCondition{Arguments: ssaflow.ArgumentConstants{Bound: 3, Values: 2}}, true},
		{"same result test", errorNil, errorNil, true},
		{"other result test", errorNil, ssaflow.CallCondition{Result: 1, Outcome: ssaflow.OutcomeNonNil}, false},
		// A case without a result test holds on every return, so it answers one.
		{"unconditional result case", flagSecond, ssaflow.CallCondition{Result: 1, Outcome: ssaflow.OutcomeNil, Arguments: flagSecond.Arguments}, true},
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
	if !(ssaflow.CallCondition{Nilness: ssaflow.ArgumentConstants{Bound: 1, Values: 1}}).ValidFor(signature) {
		t.Error("nilness of a pointer parameter should be valid")
	}
	if (ssaflow.CallCondition{Nilness: ssaflow.ArgumentConstants{Bound: 2}}).ValidFor(signature) {
		t.Error("nilness of an int parameter should be invalid")
	}
}

func TestCallConditionString(t *testing.T) {
	for _, test := range []struct {
		condition ssaflow.CallCondition
		want      string
	}{
		{ssaflow.CallCondition{}, "always"},
		{ssaflow.CallCondition{Result: 1, Outcome: ssaflow.OutcomeNonNil}, "result 1 is non-nil"},
		{ssaflow.ParameterNil(2), "argument 2 is nil"},
		{
			ssaflow.CallCondition{Arguments: ssaflow.ArgumentConstants{Bound: 0b110, Values: 0b010}},
			"argument 1 is true and argument 2 is false",
		},
	} {
		if got := test.condition.String(); got != test.want {
			t.Errorf("String() = %q, want %q", got, test.want)
		}
	}
}
