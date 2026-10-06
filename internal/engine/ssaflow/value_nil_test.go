package ssaflow_test

import (
	"go/types"
	"testing"

	"github.com/kojah/gohawk/internal/engine/ssaflow"
	"github.com/kojah/gohawk/internal/engine/ssaflow/ssaflowtest"
	"golang.org/x/tools/go/ssa"
)

func TestDefinitelyNilPreservesBoxing(t *testing.T) {
	pkg := ssaflowtest.BuildPackage(t, "nilvalues", `package nilvalues
type errPtr struct{}
func (*errPtr) Error() string { return "error" }
type pointer *int
func nilPointer() *int { return nil }
func convertedPointer() pointer { return pointer((*int)(nil)) }
func nilInterface() any { return nil }
func convertedInterface() any { var e error; return e }
func boxedPointer() any { return (*int)(nil) }
func boxedError() error { return (*errPtr)(nil) }
func changedBox() any { var e error = (*errPtr)(nil); return e }
func mixed(choose bool) *int {
	var p *int
	if choose { p = new(int) }
	return p
}
`)
	for _, test := range []struct {
		name string
		nil  bool
	}{
		{"nilPointer", true},
		{"convertedPointer", true},
		{"nilInterface", true},
		{"convertedInterface", true},
		{"boxedPointer", false},
		{"boxedError", false},
		{"changedBox", false},
		{"mixed", false},
	} {
		t.Run(test.name, func(t *testing.T) {
			returns := ssaflow.InstructionsOf[*ssa.Return](pkg.Func(test.name))
			if len(returns) != 1 || len(returns[0].Results) != 1 {
				t.Fatal("expected one return with one result")
			}
			if got := ssaflow.DefinitelyNil(returns[0].Results[0]); got != test.nil {
				t.Errorf("definitely nil = %t, want %t", got, test.nil)
			}
		})
	}
}

// Shared phi alternatives are independent paths; a repeated nil leaf is
// evidence on each path. A cycle alone cannot establish that evidence.
func TestDefinitelyNilPhiAlternatives(t *testing.T) {
	nilValue := ssa.NewConst(nil, types.NewPointer(types.Typ[types.Int]))
	shared := &ssa.Phi{Edges: []ssa.Value{nilValue, nilValue}}
	nested := &ssa.Phi{Edges: []ssa.Value{shared, shared}}
	cycle := &ssa.Phi{}
	cycle.Edges = []ssa.Value{nilValue, cycle}
	for _, test := range []struct {
		name  string
		value ssa.Value
		nil   bool
	}{
		{"shared", shared, true},
		{"nested", nested, true},
		{"cycle", cycle, false},
		{"empty", &ssa.Phi{}, false},
		{"missing", nil, false},
	} {
		if got := ssaflow.DefinitelyNil(test.value); got != test.nil {
			t.Errorf("%s: definitely nil = %t, want %t", test.name, got, test.nil)
		}
	}
}
