package ssainfer

import (
	"testing"

	"github.com/kojah/gohawk/internal/ssaflow"
	"golang.org/x/tools/go/ssa"
)

// A stable guard contradicts on its other arm; a loaded guard's other arm is
// only uncertain; a store to the loaded cell forgets it; an unrelated branch
// is consistent.
func TestPathGuards(t *testing.T) {
	pkg := buildTestSSA(t, `
package ssaflowtest

type options struct{ enabled bool }

func stable(enabled bool, a *int) int {
	if enabled {
		if a != nil {
			return 1
		}
	}
	if enabled {
		return 2
	}
	return 3
}

func loaded(o *options) int {
	if o.enabled {
		o.enabled = false
	}
	if o.enabled {
		return 1
	}
	return 0
}
`)
	stableFn := pkg.Func("stable")
	branches := branchBlocks(stableFn)
	guards, contradiction := ssaflow.PathGuards(nil).Extend(branches[0], branches[0].Succs[0], nil)
	if contradiction != ssaflow.GuardConsistent || len(guards) != 1 || !guards[0].Stable || !guards[0].Value {
		t.Fatalf("first branch: %+v %v", guards, contradiction)
	}
	last := branches[len(branches)-1]
	if _, contradiction := guards.Extend(last, last.Succs[1], nil); contradiction != ssaflow.GuardStableContradiction {
		t.Errorf("the other arm of a stable guard should contradict, got %v", contradiction)
	}
	if _, contradiction := guards.Extend(last, last.Succs[0], nil); contradiction != ssaflow.GuardConsistent {
		t.Errorf("the same arm of a stable guard is consistent, got %v", contradiction)
	}
	if _, contradiction := guards.Extend(branches[1], branches[1].Succs[1], nil); contradiction != ssaflow.GuardConsistent {
		t.Errorf("an unrelated branch is consistent, got %v", contradiction)
	}
	keepLoaded := func(guard ssaflow.PathGuard) bool { return !guard.Stable }
	if kept, _ := ssaflow.PathGuards(nil).Extend(branches[0], branches[0].Succs[0], keepLoaded); len(kept) != 0 {
		t.Errorf("a filtered-out guard must not be remembered: %+v", kept)
	}

	loadedFn := pkg.Func("loaded")
	branches = branchBlocks(loadedFn)
	guards, _ = ssaflow.PathGuards(nil).Extend(branches[0], branches[0].Succs[0], nil)
	if len(guards) != 1 || guards[0].Stable {
		t.Fatalf("a field load is a loaded guard: %+v", guards)
	}
	if _, contradiction := guards.Extend(branches[1], branches[1].Succs[1], nil); contradiction != ssaflow.GuardLoadedContradiction {
		t.Errorf("the other arm of a loaded guard is uncertain, got %v", contradiction)
	}
	for _, store := range ssaflow.InstructionsOf[*ssa.Store](loadedFn) {
		guards = guards.Forget(store)
	}
	if len(guards) != 0 {
		t.Errorf("a store to the guarded cell forgets the guard: %+v", guards)
	}
}

func branchBlocks(function *ssa.Function) []*ssa.BasicBlock {
	var blocks []*ssa.BasicBlock
	for _, block := range function.Blocks {
		if len(block.Instrs) > 0 {
			if _, ok := block.Instrs[len(block.Instrs)-1].(*ssa.If); ok {
				blocks = append(blocks, block)
			}
		}
	}
	return blocks
}
