package ssaflow

import (
	"testing"

	"github.com/kojah/gohawk/internal/ssaflow/ssaflowtest"
	"golang.org/x/tools/go/ssa"
)

func TestPossibleSpawnBindingBudget(t *testing.T) {
	pkg := ssaflowtest.BuildPackage(t, "binding", `package binding
func subject(first, second chan int, flag bool) { ch:=first; if flag {ch=second}; go func(){<-ch}() }
`)
	spawn := InstructionsOf[*ssa.Go](pkg.Func("subject"))[0]
	function, closure := DirectCallee(spawn.Common())
	value := function.FreeVars[0]
	cutoff := NewSearchBudget(0)
	if got := SpawnedValueAtCallWithin(spawn, function, closure, value, cutoff); got != nil || !cutoff.Exhausted() {
		t.Fatalf("cutoff=%v exhausted=%v", got, cutoff.Exhausted())
	}
	fresh := NewSearchBudget(QueryBudget)
	got := SpawnedValueAtCallWithin(spawn, function, closure, value, fresh)
	if got == nil || fresh.Exhausted() || got != SpawnedValueAtCall(spawn, function, closure, value) {
		t.Fatal("fresh possible binding must preserve first-candidate policy")
	}
	aliasCutoff := NewSearchBudget(0)
	if MayAliasThroughLoadsWithin(value, value, aliasCutoff) || !aliasCutoff.Exhausted() {
		t.Fatal("even a direct reaching leaf must charge its allowance")
	}
}

func TestInstructionMayFollowBudget(t *testing.T) {
	pkg := ssaflowtest.BuildPackage(t, "following", `package following
func marker() {}
func subject(flag bool) { marker(); if flag {marker()}; marker() }
`)
	calls := InstructionsOf[*ssa.Call](pkg.Func("subject"))
	cutoff := NewSearchBudget(0)
	if InstructionMayFollowWithin(calls[0], calls[1], cutoff) || !cutoff.Exhausted() {
		t.Fatal("unavailable reachability must not imply disconnection")
	}
	for _, before := range calls {
		for _, after := range calls {
			fresh := NewSearchBudget(QueryBudget)
			if got := InstructionMayFollowWithin(before, after, fresh); got != InstructionMayFollow(before, after) || fresh.Exhausted() {
				t.Fatal("fresh reachability must preserve order and direction")
			}
		}
	}
}
