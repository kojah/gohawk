package resultfacts

import (
	"testing"

	"github.com/kojah/gohawk/internal/ssaflow"
	"github.com/kojah/gohawk/internal/ssaflow/ssaflowtest"
)

const resultFixture = `package results
type failure struct{}
func (*failure) Error() string { return "failure" }
var sentinel error = &failure{}
func Nil() error { return nil }
func TypedNil() error { return (*failure)(nil) }
func Fresh() *int { return new(int) }
func Yes() bool { return true }
func No() bool { return false }
func Forward() error { return Nil() }
func Tuple() (bool, error, *int) { return true, (*failure)(nil), nil }
func ForwardTuple() (bool, error, *int) { return Tuple() }
func Mixed(yes bool) error { if yes { return nil }; return &failure{} }
func Global() error { return sentinel }
func Parameter(value error) error { return value }
func Opaque() error
func Unresolved() error { return Opaque() }
func Recursive() error { return Recursive() }
func Deferred() (err error) { defer func() { err = &failure{} }(); return nil }
func Recovered() (yes bool) { defer func() { recover() }(); panic("failure") }
func PanicOnly() error { panic("failure") }
func Forever() error { for {} }
func Boxed(value *failure) error { return value }
func Generic[T any](value T) any { return value }
func GenericNil() any { return Generic[any](nil) }
`

func TestResultGuarantees(t *testing.T) {
	pkg := ssaflowtest.BuildPackage(t, "results", resultFixture)
	for name, want := range map[string][]Guarantee{
		"Nil": {AlwaysNil}, "TypedNil": {AlwaysNonNil}, "Fresh": {AlwaysNonNil},
		"Yes": {AlwaysTrue}, "No": {AlwaysFalse}, "Forward": {AlwaysNil},
		"Tuple": {AlwaysTrue, AlwaysNonNil, AlwaysNil}, "ForwardTuple": {AlwaysTrue, AlwaysNonNil, AlwaysNil},
		"Mixed": {Unknown}, "Global": {Unknown}, "Parameter": {Unknown}, "Unresolved": {Unknown},
		"Recursive": {Unknown}, "Deferred": {Unknown}, "Recovered": {Unknown}, "PanicOnly": {Unknown}, "Forever": {Unknown},
		"Boxed":      {AlwaysNonNil},
		"GenericNil": {Unknown},
	} {
		t.Run(name, func(t *testing.T) {
			got := NewEngine().Function(pkg.Func(name), ssaflow.NewSearchBudget(2000))
			for index, expected := range want {
				if got.Result(index) != expected {
					t.Errorf("result %d: got %v, want %v (%+v)", index, got.Result(index), expected, got)
				}
			}
		})
	}
}

func TestResultBudgetDoesNotPoisonSummary(t *testing.T) {
	pkg := ssaflowtest.BuildPackage(t, "results", resultFixture)
	engine := NewEngine()
	if got := engine.Function(pkg.Func("Forward"), ssaflow.NewSearchBudget(1)); got.Result(0) != Unknown {
		t.Fatalf("shortened proof: %+v", got)
	}
	if got := engine.Function(pkg.Func("Forward"), ssaflow.NewSearchBudget(2000)); got.Result(0) != AlwaysNil {
		t.Fatalf("fresh budget did not recover: %+v", got)
	}
}
