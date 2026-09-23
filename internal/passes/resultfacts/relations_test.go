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
	want := map[string][]Relation{
		"Failed":               {{0, FalseWhenParameterNil, 0}, {0, TrueWhenParameterNonNil, 0}},
		"Succeeded":            nil,
		"FailedWithSideEffect": {{0, FalseWhenParameterNil, 0}, {0, TrueWhenParameterNonNil, 0}},
		"Other":                {{0, FalseWhenParameterNil, 1}, {0, TrueWhenParameterNonNil, 1}},
		"Wrapped":              nil,
		"Forwarded":            nil,
		"Sometimes":            {{0, TrueWhenParameterNonNil, 0}},
		"Open":                 {{0, NonNilWhenResultNil, 1}, {0, NilWhenResultNonNil, 1}},
		"OpenForwarded":        {{0, NonNilWhenResultNil, 1}, {0, NilWhenResultNonNil, 1}},
		"OpenMaybeNil":         {{0, NilWhenResultNonNil, 1}},
		"OpenLeaky":            {{0, NonNilWhenResultNil, 1}},
		"OpenUnknown":          {{0, ReturnsParameter, 0}, {1, ReturnsParameter, 1}},
		"Same":                 {{0, ReturnsParameter, 0}},
		"Chosen":               nil,
		"Erased":               nil,
		"Rewrapped":            {{0, ReturnsParameter, 0}},
	}
	self := pkg.Prog.LookupMethod(types.NewPointer(pkg.Type("box").Type()), pkg.Pkg, "Self")
	if got := NewEngine().Function(self, ssaflow.NewSearchBudget(4000)); !got.Holds(ReturnsParameter, 0, 0) {
		t.Errorf("Self: a method returning its receiver: %+v", got.Relations())
	}
	for name, expected := range want {
		t.Run(name, func(t *testing.T) {
			got := NewEngine().Function(pkg.Func(name), ssaflow.NewSearchBudget(4000))
			if len(got.Relations()) != len(expected) {
				t.Fatalf("relations = %+v, want %+v", got.Relations(), expected)
			}
			for _, relation := range expected {
				if !got.Holds(relation.Kind, relation.Result, relation.Operand) {
					t.Errorf("missing %+v in %+v", relation, got.Relations())
				}
			}
		})
	}
}
