package ssaflow

import (
	"slices"
	"testing"

	proofs "github.com/kojah/gohawk/internal/engine/proof"
	"github.com/kojah/gohawk/internal/engine/ssaflow/ssaflowtest"
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

func TestInstructionCensusAvailabilityAndEarlyStop(t *testing.T) {
	pkg := ssaflowtest.BuildPackage(t, "census", `package census
func marker() {}
func subject(flag bool) { marker(); if flag {marker()}; marker() }
`)
	function := pkg.Func("subject")
	var expected []ssa.Instruction
	for _, block := range function.Blocks {
		expected = append(expected, block.Instrs...)
	}
	zero := proofs.NewSearchBudget(0)
	if got := slices.Collect(InstructionsWithin(function, zero)); len(got) != 0 || !zero.Exhausted() {
		t.Fatalf("zero census=%v exhausted=%v", got, zero.Exhausted())
	}
	partial := proofs.NewSearchBudget(2)
	if got := slices.Collect(InstructionsWithin(function, partial)); !slices.Equal(got, expected[:2]) || !partial.Exhausted() {
		t.Fatalf("partial census=%v exhausted=%v", got, partial.Exhausted())
	}
	// Exactly enough allowance is still available until a further step is
	// requested. Stopping on a witness must not probe the next instruction.
	early := proofs.NewSearchBudget(1)
	for got := range InstructionsWithin(function, early) {
		if got != expected[0] {
			t.Fatal("census order changed")
		}
		break
	}
	if early.Exhausted() || early.Spend() || !early.Exhausted() {
		t.Fatal("early break must spend exactly the yielded instruction")
	}
	fresh := proofs.NewSearchBudget(len(expected))
	if got := slices.Collect(InstructionsWithin(function, fresh)); !slices.Equal(got, expected) || fresh.Exhausted() {
		t.Fatalf("fresh census=%v exhausted=%v", got, fresh.Exhausted())
	}
	var calls []*ssa.Call
	for _, instruction := range expected {
		if call, ok := instruction.(*ssa.Call); ok {
			calls = append(calls, call)
		}
	}
	if !slices.Equal(calls, InstructionsOf[*ssa.Call](function)) {
		t.Fatal("typed collection must share the same instruction order")
	}
}
