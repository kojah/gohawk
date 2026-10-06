package ssaflow

import (
	"testing"

	"github.com/kojah/gohawk/internal/ssaflow/ssaflowtest"
	"golang.org/x/tools/go/ssa"
)

func TestFlowLocationKeepsDistinctPathsAndPositions(t *testing.T) {
	pkg := ssaflowtest.BuildPackage(t, "locations", `package locations
 func subject(flag bool) int { value:=1; if flag {value=2}; return value }
 `)
	fn := pkg.Func("subject")
	branch := InstructionsOf[*ssa.If](fn)[0]
	block := InstructionsOf[*ssa.Return](fn)[0].Block()
	if len(block.Preds) != 2 || block.Preds[0].Index != 0 {
		t.Fatal("fixture lost entry predecessor and merge")
	}
	identity, _, stable, known := GuardCondition(branch.Cond)
	if !known || !stable {
		t.Fatal("fixture lost stable guard")
	}
	trueGuards := PathGuards{{Identity: identity, Value: true, Stable: stable}}
	falseGuards := PathGuards{{Identity: identity, Value: false, Stable: stable}}
	keys := []FlowLocationKey{
		FlowLocationKeyWithin(block, nil, 0, nil, nil),
		FlowLocationKeyWithin(block, block.Preds[0], 0, nil, nil),
		FlowLocationKeyWithin(block, block.Preds[1], 0, nil, nil),
		FlowLocationKeyWithin(block, nil, 1, nil, nil),
		FlowLocationKeyWithin(block, nil, 0, trueGuards, nil),
		FlowLocationKeyWithin(block, nil, 0, falseGuards, nil),
	}
	keys = append(keys, keys[0])
	expanded := 0
	WalkStates(keys, func(key FlowLocationKey) FlowLocationKey { return key }, func(FlowLocationKey) ([]FlowLocationKey, bool) {
		expanded++
		return nil, true
	})
	if expanded != 6 {
		t.Fatalf("distinct predecessor, position or guard lost: expanded=%d", expanded)
	}
	// The queue visit fits, but the guarded key does not. Its empty rendering
	// must not be admitted as an unguarded location or expanded by the walk.
	cut := NewSearchBudget(1)
	expanded = 0
	WalkStatesWithin([]int{0}, func(index int) FlowLocationKey {
		return FlowLocationKeyWithin(block, nil, index, trueGuards, cut)
	}, func(int) ([]int, bool) { expanded++; return nil, true }, cut)
	if expanded != 0 || !cut.Exhausted() {
		t.Fatal("partial guarded location admitted")
	}
}

func TestObligationKeyKeepsCoverageAtSameLocation(t *testing.T) {
	pkg := ssaflowtest.BuildPackage(t, "coverage", `package coverage
 func subject(){}
 `)
	block := pkg.Func("subject").Blocks[0]
	states := []obligationState{
		{block: block, covered: ObligationNone},
		{block: block, covered: ObligationUnknown},
		{block: block, covered: ObligationExact},
		{block: block, predecessor: block},
		{block: block, index: 1},
		{block: block, guards: PathGuards{{Identity: "x", Value: true}}},
		{block: block, guards: PathGuards{{Identity: "x"}}},
		{block: &ssa.BasicBlock{Index: block.Index + 1}},
	}
	expanded := 0
	var ids guardIDs
	WalkStates(states, func(state obligationState) obligationKey { return state.keyWithin(nil, &ids) },
		func(obligationState) ([]obligationState, bool) { expanded++; return nil, true })
	if expanded != len(states) {
		t.Fatalf("coverage collapsed at one guarded position: expanded=%d", expanded)
	}
}
