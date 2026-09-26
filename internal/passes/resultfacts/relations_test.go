package resultfacts

import (
	"go/types"
	"testing"

	"github.com/kojah/gohawk/internal/ssaflow"
	"github.com/kojah/gohawk/internal/ssaflow/ssaflowtest"
)

const relationFixture = `package results

type box struct{}

// Exact predicates on the parameter.
func Failed(err error) bool { return err != nil }
func Succeeded(err error) bool { return err == nil }
func FailedWithSideEffect(err error, sink chan error) bool {
	if err != nil {
		select {
		case sink <- err:
		default:
		}
		return true
	}
	return false
}
func Other(_ error, other error) bool { return other != nil }
func Wrapped(err error) bool {
	wrapped := error(nil)
	if err != nil {
		wrapped = err
	}
	return wrapped != nil
}
func Forwarded(err error) bool { return Failed(err) }
func Sometimes(err error, flag bool) bool {
	if flag {
		return true
	}
	return err != nil
}

// Result pairs.
func Open(ok bool) (*box, error) {
	if !ok {
		return nil, sentinelError()
	}
	return &box{}, nil
}
func OpenForwarded(ok bool) (*box, error) { return Open(ok) }
func OpenMaybeNil(ok bool) (*box, error) {
	if !ok {
		return nil, sentinelError()
	}
	return nil, nil
}
func OpenLeaky(ok bool) (*box, error) {
	if !ok {
		return &box{}, sentinelError()
	}
	return &box{}, nil
}
func OpenUnknown(value *box, err error) (*box, error) { return value, err }
func OpenAfterCall(produce func() (*box, error)) (*box, error) {
	value, err := produce()
	if err != nil {
		return nil, err
	}
	_ = value
	return &box{}, nil
}

// Identity.
func Same(value *box) *box { return value }
func (value *box) Self() *box { return value }
func Chosen(value, other *box, pick bool) *box {
	if pick {
		return other
	}
	return value
}
func Erased(value *box) any { return value }
func Rewrapped(value *box) *box { return &(*value) }

type failure struct{}

func (*failure) Error() string { return "failure" }
func sentinelError() error     { return &failure{} }
`

func TestResultRelations(t *testing.T) {
	pkg := ssaflowtest.BuildPackage(t, "results", relationFixture)
	falseWhenNil := func(parameter int) ResultCase {
		return ResultCase{Condition: ssaflow.ParameterNil(parameter), Outcome: ssaflow.OutcomeFalse}
	}
	nonNilWhenErrorNil := ResultCase{Condition: errorOutcome(1, ssaflow.OutcomeNil), Outcome: ssaflow.OutcomeNonNil}
	nilWhenErrorNonNil := ResultCase{Condition: errorOutcome(1, ssaflow.OutcomeNonNil), Outcome: ssaflow.OutcomeNil}
	want := map[string]struct {
		cases    []ResultCase
		returned []ReturnedParameter
	}{
		"Failed":               {cases: []ResultCase{falseWhenNil(0)}},
		"Succeeded":            {},
		"FailedWithSideEffect": {cases: []ResultCase{falseWhenNil(0)}},
		"Other":                {cases: []ResultCase{falseWhenNil(1)}},
		"Wrapped":              {},
		"Forwarded":            {},
		"Sometimes":            {},
		"Open":                 {cases: []ResultCase{nonNilWhenErrorNil, nilWhenErrorNonNil}},
		"OpenForwarded":        {cases: []ResultCase{nonNilWhenErrorNil, nilWhenErrorNonNil}},
		"OpenMaybeNil":         {cases: []ResultCase{nilWhenErrorNonNil}},
		"OpenLeaky":            {cases: []ResultCase{nonNilWhenErrorNil}},
		"OpenUnknown":          {returned: []ReturnedParameter{{0, 0}, {1, 1}}},
		"OpenAfterCall":        {cases: []ResultCase{nilWhenErrorNonNil}},
		"Same":                 {returned: []ReturnedParameter{{0, 0}}},
		"Chosen":               {},
		"Erased":               {},
		"Rewrapped":            {returned: []ReturnedParameter{{0, 0}}},
	}
	self := pkg.Prog.LookupMethod(types.NewPointer(pkg.Type("box").Type()), pkg.Pkg, "Self")
	if parameter, ok := NewEngine().Function(self, ssaflow.NewSearchBudget(4000)).ReturnedParameter(0); !ok || parameter != 0 {
		t.Errorf("Self: a method returning its receiver: parameter %d, %t", parameter, ok)
	}
	for name, expected := range want {
		t.Run(name, func(t *testing.T) {
			got := NewEngine().Function(pkg.Func(name), ssaflow.NewSearchBudget(4000))
			if len(got.Cases()) != len(expected.cases) || len(got.returned) != len(expected.returned) {
				t.Fatalf("cases = %+v, returned = %+v; want %+v, %+v", got.Cases(), got.returned, expected.cases, expected.returned)
			}
			for _, proven := range expected.cases {
				if !got.Implies(proven.Condition, proven.Result, proven.Outcome) {
					t.Errorf("missing %+v in %+v", proven, got.Cases())
				}
			}
			for _, returned := range expected.returned {
				if parameter, ok := got.ReturnedParameter(returned.Result); !ok || parameter != returned.Parameter {
					t.Errorf("result %d returns parameter %d, %t; want %d", returned.Result, parameter, ok, returned.Parameter)
				}
			}
		})
	}
}
