package lifecycle

import (
	"testing"

	"github.com/kojah/gohawk/internal/ssaflow"
	ssapath "github.com/kojah/gohawk/internal/ssaflow/path"
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
	guards, contradiction := ssapath.PathGuards(nil).ExtendWithin(branches[0], branches[0].Succs[0], nil, nil)
	if contradiction != ssapath.GuardConsistent || len(guards) != 1 || !guards[0].Stable || !guards[0].Value {
		t.Fatalf("first branch: %+v %v", guards, contradiction)
	}
	last := branches[len(branches)-1]
	if _, contradiction := guards.ExtendWithin(last, last.Succs[1], nil, nil); contradiction != ssapath.GuardStableContradiction {
		t.Errorf("the other arm of a stable guard should contradict, got %v", contradiction)
	}
	if _, contradiction := guards.ExtendWithin(last, last.Succs[0], nil, nil); contradiction != ssapath.GuardConsistent {
		t.Errorf("the same arm of a stable guard is consistent, got %v", contradiction)
	}
	if _, contradiction := guards.ExtendWithin(branches[1], branches[1].Succs[1], nil, nil); contradiction != ssapath.GuardConsistent {
		t.Errorf("an unrelated branch is consistent, got %v", contradiction)
	}
	keepLoaded := func(guard ssapath.PathGuard) bool { return !guard.Stable }
	if kept, _ := ssapath.PathGuards(nil).ExtendWithin(branches[0], branches[0].Succs[0], keepLoaded, nil); len(kept) != 0 {
		t.Errorf("a filtered-out guard must not be remembered: %+v", kept)
	}

	loadedFn := pkg.Func("loaded")
	branches = branchBlocks(loadedFn)
	guards, _ = ssapath.PathGuards(nil).ExtendWithin(branches[0], branches[0].Succs[0], nil, nil)
	if len(guards) != 1 || guards[0].Stable {
		t.Fatalf("a field load is a loaded guard: %+v", guards)
	}
	if _, contradiction := guards.ExtendWithin(branches[1], branches[1].Succs[1], nil, nil); contradiction != ssapath.GuardLoadedContradiction {
		t.Errorf("the other arm of a loaded guard is uncertain, got %v", contradiction)
	}
	for _, store := range ssaflow.InstructionsOf[*ssa.Store](loadedFn) {
		guards = guards.AfterWithin(store, nil)
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
