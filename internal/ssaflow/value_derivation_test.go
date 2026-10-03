package ssaflow_test

import (
	"testing"

	"github.com/kojah/gohawk/internal/ssaflow"
	"github.com/kojah/gohawk/internal/ssaflow/ssaflowtest"
	"golang.org/x/tools/go/ssa"
)

// Derivation includes computations and opaque call operands. It supplies
// possible provenance, not an exact identity or ownership guarantee.
func TestDerivationBoundaries(t *testing.T) {
	pkg := ssaflowtest.BuildPackage(t, "derivation", `package derivation
func opaque(*int) *int
func called(p *int) *int { return opaque(p) }
func boxed(p *int) any { return p }
func arithmetic(n int) int { return n + 1 }
func unrelated(p *int) *int { return new(int) }
func stored(p *int) *int {
	x := new(*int)
	*x = p
	return *x
}
func cyclic(p *int, n int) *int {
	x := p
	for i := 0; i < n; i++ { if i%2 == 0 { x = new(int) } }
	return x
}
func unrelatedCycle(p *int, n int) *int {
	x := new(int)
	for i := 0; i < n; i++ { if i%2 == 0 { x = new(int) } }
	return x
}
`)
	for _, test := range []struct {
		name string
		want bool
	}{
		{"called", true},
		{"boxed", true},
		{"arithmetic", true},
		{"stored", true},
		{"unrelated", false},
		{"cyclic", true},
		{"unrelatedCycle", false},
	} {
		t.Run(test.name, func(t *testing.T) {
			function := pkg.Func(test.name)
			returns := ssaflow.InstructionsOf[*ssa.Return](function)
			if len(returns) != 1 || len(returns[0].Results) != 1 {
				t.Fatal("expected one return with one result")
			}
			got := ssaflow.DerivesFromWithin(returns[0].Results[0], function.Params[0], ssaflow.StructurallySame, nil)
			if got != test.want {
				t.Errorf("DerivesFromWithin() = %t, want %t", got, test.want)
			}
		})
	}
}

func TestDerivationIdentityBeforeOperands(t *testing.T) {
	same := func(left, right ssa.Value) bool { return left == right }
	source := &ssa.Alloc{}
	empty := &ssa.Phi{}
	cycle := &ssa.Phi{}
	cycle.Edges = []ssa.Value{cycle, source}
	unrelated := &ssa.Phi{}
	unrelated.Edges = []ssa.Value{unrelated}
	for _, test := range []struct {
		name   string
		value  ssa.Value
		source ssa.Value
		want   bool
	}{
		{"empty phi identity", empty, empty, true},
		{"cyclic phi identity", unrelated, unrelated, true},
		{"source beside cycle", cycle, source, true},
		{"cycle without source", unrelated, source, false},
		{"missing value", nil, source, false},
		{"missing source", source, nil, false},
	} {
		t.Run(test.name, func(t *testing.T) {
			if got := ssaflow.DerivesFromWithin(test.value, test.source, same, nil); got != test.want {
				t.Errorf("DerivesFromWithin() = %t, want %t", got, test.want)
			}
		})
	}
}
