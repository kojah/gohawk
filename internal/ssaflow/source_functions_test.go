package ssaflow

import (
	"slices"
	"testing"

	"github.com/kojah/gohawk/internal/ssaflow/ssaflowtest"
	"golang.org/x/tools/go/analysis/passes/buildssa"
	"golang.org/x/tools/go/ssa"
)

// A closure written in a package-level variable initializer belongs to the
// synthetic package initializer, which buildssa's SrcFuncs does not walk.
// Source selection adds it, nested closures included, and never the
// synthetic initializer itself.
func TestSourceFunctionsIncludeInitializerClosures(t *testing.T) {
	pkg := ssaflowtest.BuildPackage(t, "initializers", `package initializers
type command struct{ run func() int }
var cmd = &command{run: func() int {
	inner := func() int { return 1 }
	return inner()
}}
func declared() int { return cmd.run() }
`)
	names := func(functions []*ssa.Function) []string {
		var result []string
		for _, function := range functions {
			result = append(result, function.Name())
		}
		return result
	}
	selected := names(sourceFunctions(&buildssa.SSA{Pkg: pkg, SrcFuncs: []*ssa.Function{pkg.Func("declared")}}))
	for _, want := range []string{"declared", "init$1", "init$1$1"} {
		if !slices.Contains(selected, want) {
			t.Errorf("sourceFunctions = %v, missing %s", selected, want)
		}
	}
	if slices.Contains(selected, "init") {
		t.Errorf("sourceFunctions = %v, want no synthetic initializer", selected)
	}
	if declared := names(DeclaredFunctions(pkg)); !slices.Contains(declared, "init$1$1") {
		t.Errorf("DeclaredFunctions = %v, missing the nested initializer closure", declared)
	}
}
