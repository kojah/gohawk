package path_test

import (
	"slices"
	"testing"

	proofs "github.com/kojah/gohawk/internal/engine/proof"
	cfg "github.com/kojah/gohawk/internal/engine/ssaflow/cfg"
	"github.com/kojah/gohawk/internal/engine/ssaflow/ssaflowtest"
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
	if !cfg.BlockReachable(entry, entry) || cfg.BlockInCycle(entry) {
		t.Error("acyclic entry must reach itself without being in a cycle")
	}
	if !cfg.InstructionMayFollow(before, after) || cfg.InstructionMayFollow(after, before) {
		t.Error("instructions in one block must retain source order")
	}
	if !cfg.InstructionMayFollow(before, left.Instrs[0]) || cfg.InstructionMayFollow(left.Instrs[0], right.Instrs[0]) {
		t.Error("entry reaches each arm; sibling arms cannot reach each other")
	}
	other := pkg.Func("other").Blocks[0]
	if cfg.BlockReachable(entry, other) || cfg.BlockReachable(nil, entry) || cfg.BlockReachable(entry, nil) ||
		cfg.InstructionMayFollow(before, other.Instrs[0]) || cfg.InstructionMayFollow(nil, after) {
		t.Error("missing or cross-function starts must not establish reachability")
	}
}

func TestCycleReachabilityPreservesInstructionsAndSuccessors(t *testing.T) {
	t.Parallel()
	pkg := ssaflowtest.BuildPackage(t, "reachability", reachabilityFixture)
	cycle := pkg.Func("cycle")
	if cfg.BlockInCycle(cycle.Blocks[0]) {
		t.Error("entering a loop does not put its acyclic entry in the cycle")
	}
	foundCycle := false
	for _, block := range cycle.Blocks {
		seeds := slices.Clone(block.Succs)
		inCycle := cfg.BlockInCycle(block)
		foundCycle = foundCycle || inCycle
		if inCycle && len(block.Instrs) > 1 {
			first, last := block.Instrs[0], block.Instrs[len(block.Instrs)-1]
			if !cfg.InstructionMayFollow(first, last) || cfg.InstructionMayFollow(last, first) {
				t.Errorf("cycle reversed instruction order in block %d", block.Index)
			}
		}
		if !cfg.BlockReachable(cycle.Blocks[0], block) {
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

func TestBlockReachableWithinAllowance(t *testing.T) {
	t.Parallel()
	pkg := ssaflowtest.BuildPackage(t, "reachability", reachabilityFixture)
	entry := pkg.Func("branch").Blocks[0]
	left, right := entry.Succs[0], entry.Succs[1]
	for _, test := range []struct {
		name         string
		from, target int
		want         bool
	}{
		{"reachable", entry.Index, left.Index, true},
		{"sibling", left.Index, right.Index, false},
	} {
		t.Run(test.name, func(t *testing.T) {
			fn := pkg.Func("branch")
			from, target := fn.Blocks[test.from], fn.Blocks[test.target]
			pool := proofs.NewSearchBudget(100)
			cut := pool.Within(0)
			if cfg.BlockReachableWithin(from, target, cut) || !cut.Exhausted() || pool.Exhausted() {
				t.Fatal("child cutoff must leave reachability unavailable and parent available")
			}
			fresh := pool.Within(100)
			if got := cfg.BlockReachableWithin(from, target, fresh); got != test.want || fresh.Exhausted() {
				t.Fatalf("fresh reachability=%v, want %v; exhausted %v", got, test.want, fresh.Exhausted())
			}
		})
	}
}
