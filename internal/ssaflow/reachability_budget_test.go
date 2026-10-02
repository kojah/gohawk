package ssaflow

import (
	"slices"
	"testing"

	"github.com/kojah/gohawk/internal/ssaflow/ssaflowtest"
	"golang.org/x/tools/go/ssa"
)

func TestReachabilityBudgetAvailability(t *testing.T) {
	pkg := ssaflowtest.BuildPackage(t, "reachability", `package reachability
func marker() {}
func subject(branch bool) { marker(); if branch { marker() }; marker() }
`)
	start := InstructionsOf[*ssa.Call](pkg.Func("subject"))[0]
	cutoff := NewSearchBudget(0)
	if got := InstructionsReachableAfterWithin(start, cutoff); len(got) != 0 || !cutoff.Exhausted() {
		t.Fatalf("cutoff = %v, exhausted=%v", got, cutoff.Exhausted())
	}
	partial := NewSearchBudget(2)
	if got := InstructionsReachableAfterWithin(start, partial); len(got) == 0 || !partial.Exhausted() {
		t.Fatalf("expected partial census, got %v exhausted=%v", got, partial.Exhausted())
	}
	fresh := NewSearchBudget(QueryBudget)
	got := InstructionsReachableAfterWithin(start, fresh)
	if fresh.Exhausted() || !slices.Equal(got, InstructionsReachableAfter(start)) {
		t.Fatal("fresh census must share the default reachability policy")
	}
}
