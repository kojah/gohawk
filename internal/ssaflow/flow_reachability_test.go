package ssaflow_test

import (
	"slices"
	"testing"

	"github.com/kojah/gohawk/internal/ssaflow"
	"github.com/kojah/gohawk/internal/ssaflow/ssaflowtest"
)

const reachabilityFixture = `package reachability
func mark(value int) {}
func branch(yes bool) {
	mark(1)
	mark(2)
	if yes { mark(3) } else { mark(4) }
	mark(5)
}
func cycle(yes bool) { for yes { mark(6) }; mark(7) }
func other() { mark(8) }
`

func TestReachabilitySeedAndInstructionOrder(t *testing.T) {
	t.Parallel()
	pkg := ssaflowtest.BuildPackage(t, "reachability", reachabilityFixture)
	branch := pkg.Func("branch")
	entry := branch.Blocks[0]
	left, right := entry.Succs[0], entry.Succs[1]
	before, after := entry.Instrs[0], entry.Instrs[1]
	if !ssaflow.BlockReachable(entry, entry) || ssaflow.BlockInCycle(entry) {
		t.Error("acyclic entry must reach itself without being in a cycle")
	}
	if !ssaflow.InstructionMayFollow(before, after) || ssaflow.InstructionMayFollow(after, before) {
		t.Error("instructions in one block must retain source order")
	}
	if !ssaflow.InstructionMayFollow(before, left.Instrs[0]) || ssaflow.InstructionMayFollow(left.Instrs[0], right.Instrs[0]) {
		t.Error("entry reaches each arm; sibling arms cannot reach each other")
	}
	other := pkg.Func("other").Blocks[0]
	if ssaflow.BlockReachable(entry, other) || ssaflow.BlockReachable(nil, entry) || ssaflow.BlockReachable(entry, nil) ||
		ssaflow.InstructionMayFollow(before, other.Instrs[0]) || ssaflow.InstructionMayFollow(nil, after) {
		t.Error("missing or cross-function starts must not establish reachability")
	}
}

func TestCycleReachabilityPreservesInstructionsAndSuccessors(t *testing.T) {
	t.Parallel()
	pkg := ssaflowtest.BuildPackage(t, "reachability", reachabilityFixture)
	cycle := pkg.Func("cycle")
	if ssaflow.BlockInCycle(cycle.Blocks[0]) {
		t.Error("entering a loop does not put its acyclic entry in the cycle")
	}
	foundCycle := false
	for _, block := range cycle.Blocks {
		seeds := slices.Clone(block.Succs)
		inCycle := ssaflow.BlockInCycle(block)
		foundCycle = foundCycle || inCycle
		if inCycle && len(block.Instrs) > 1 {
			first, last := block.Instrs[0], block.Instrs[len(block.Instrs)-1]
			if !ssaflow.InstructionMayFollow(first, last) || ssaflow.InstructionMayFollow(last, first) {
				t.Errorf("cycle reversed instruction order in block %d", block.Index)
			}
		}
		if !ssaflow.BlockReachable(cycle.Blocks[0], block) {
			t.Errorf("entry cannot reach fixture block %d", block.Index)
		}
		if !slices.Equal(seeds, block.Succs) {
			t.Errorf("reachability changed successors of block %d", block.Index)
		}
	}
	if !foundCycle {
		t.Fatal("loop fixture produced no cycle")
	}
}
