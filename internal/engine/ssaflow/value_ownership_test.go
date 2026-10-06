package ssaflow_test

import (
	"testing"

	"github.com/kojah/gohawk/internal/engine/ssaflow"
	cfg "github.com/kojah/gohawk/internal/engine/ssaflow/cfg"
	"github.com/kojah/gohawk/internal/engine/ssaflow/ssaflowtest"
	"golang.org/x/tools/go/ssa"
)

// External ownership is possible provenance, not a claim that every path
// transfers ownership. Cycles and opaque calls cannot create that evidence.
func TestExternalOwnershipProvenanceBoundaries(t *testing.T) {
	pkg := ssaflowtest.BuildPackage(t, "owners", `package owners
func opaque(*int) *int
func localMerge(choose bool) *int {
	x := new(int)
	if choose { x = new(int) }
	return x
}
func mixedMerge(p *int, choose bool) *int {
	x := new(int)
	if choose { x = p }
	return x
}
func localCycle(n int) *int {
	x := new(int)
	for i := 0; i < n; i++ { if i%2 == 0 { x = new(int) } }
	return x
}
func externalCycle(p *int, n int) *int {
	x := p
	for i := 0; i < n; i++ { if i%2 == 0 { x = new(int) } }
	return x
}
func localCellCycle() **int {
	x := new(*int)
	*x = *x
	return x
}
func externalCellCycle(p *int) **int {
	x := new(*int)
	*x = *x
	*x = p
	return x
}
func opaqueResult(p *int) *int { return opaque(p) }
func received(ch <-chan *int) *int { return <-ch }
func asserted(p any) *int { return p.(*int) }
func arithmetic(n int) int { return -n }
`)
	for _, test := range []struct {
		name  string
		owned bool
	}{
		{"localMerge", false},
		{"mixedMerge", true},
		{"localCycle", false},
		{"externalCycle", true},
		{"localCellCycle", false},
		{"externalCellCycle", true},
		{"opaqueResult", false},
		{"received", true},
		{"asserted", true},
		{"arithmetic", false},
	} {
		t.Run(test.name, func(t *testing.T) {
			function := pkg.Func(test.name)
			returns := ssaflow.InstructionsOf[*ssa.Return](function)
			if len(returns) != 1 || len(returns[0].Results) != 1 {
				t.Fatal("expected one return with one result")
			}
			if got := ssaflow.ExternallyOwnedValue(returns[0].Results[0]); got != test.owned {
				t.Errorf("external ownership = %t, want %t", got, test.owned)
			}
		})
	}
	for _, name := range []string{"localCycle", "externalCycle"} {
		phis := ssaflow.InstructionsOf[*ssa.Phi](pkg.Func(name))
		if len(phis) == 0 || !cfg.BlockInCycle(phis[0].Block()) {
			t.Fatalf("%s did not exercise a cyclic phi", name)
		}
	}
	if ssaflow.ExternallyOwnedValue(nil) {
		t.Error("missing value supplied external ownership evidence")
	}
}
