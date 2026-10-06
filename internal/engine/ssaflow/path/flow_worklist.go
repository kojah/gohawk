package path

import (
	proofs "github.com/kojah/gohawk/internal/engine/proof"
	"golang.org/x/tools/go/ssa"
)

// FlowLocationKey identifies one guarded instruction position within a
// function. It does not include the caller's obligation or other domain state;
// callers must compose those into their work-list key. A missing predecessor
// stays distinct from entry block zero.
type FlowLocationKey struct {
	block       int
	predecessor int
	index       int
	guards      string
}

// FlowLocationKeyWithin names a valid block position and its path guards using
// budget. Block indexes are function-local, so keys must not be shared between
// functions. Guard cutoff makes the key unavailable; WalkStatesWithin checks
// that allowance before admitting it. A nil budget preserves guard rendering.
func FlowLocationKeyWithin(block, predecessor *ssa.BasicBlock, index int, guards PathGuards, budget *proofs.SearchBudget) FlowLocationKey {
	return flowLocationKeyWithMemo(block, predecessor, index, guards, budget, nil)
}

func flowLocationKeyWithMemo(
	block, predecessor *ssa.BasicBlock, index int, guards PathGuards, budget *proofs.SearchBudget, keys *guardKeys,
) FlowLocationKey {
	predecessorIndex := -1
	if predecessor != nil {
		predecessorIndex = predecessor.Index
	}
	return FlowLocationKey{block: block.Index, predecessor: predecessorIndex, index: index, guards: keys.keyWithin(guards, budget)}
}
