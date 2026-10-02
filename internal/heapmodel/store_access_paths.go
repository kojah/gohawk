package heapmodel

import (
	"go/token"

	"github.com/kojah/gohawk/internal/ssaflow"
	"golang.org/x/tools/go/ssa"
)

// Access paths name the part of an aggregate a value is, so that a claim
// about "the file in field out of this parameter" can be made and matched
// exactly, rather than collapsing to "something derived from the parameter".
// A path is the static sequence of field and constant-index selections from
// a root; a runtime index has no path. These helpers extract a path beneath
// a parameter, including through the cell a by-value parameter is spilled
// into, and resolve the value a caller stored at a path beneath an argument.

// AccessPathFromParameter extends ssaflow.AccessPathSteps with spill cells as
// alternative roots: a struct or array parameter is copied into a local
// cell before a field is selected. The cell must still contain that parameter
// when its contents are read; writing only whole values is not sufficient.
func AccessPathFromParameter(value, parameter ssa.Value) ([]string, bool) {
	return AccessPathFromParameterWithin(value, parameter, nil)
}

// AccessPathFromParameterWithin shares direct path, spill-store and whole-cell
// questions with budget. A cutoff publishes no path, including an empty one;
// callers retain budget availability. Replaced, ambiguous or exposed contents
// cannot name the original parameter. Nil retains the default storage allowance.
func AccessPathFromParameterWithin(value, parameter ssa.Value, budget *ssaflow.SearchBudget) ([]string, bool) {
	if path, ok := ssaflow.AccessPathStepsWithin(value, parameter, budget); ok {
		return path, true
	}
	if budget.Exhausted() || budget.PoolExhausted() || parameter.Referrers() == nil {
		return nil, false
	}
	for _, reference := range *parameter.Referrers() {
		if !budget.Spend() {
			return nil, false
		}
		store, ok := reference.(*ssa.Store)
		if !ok || store.Val != parameter {
			continue
		}
		cell, ok := store.Addr.(*ssa.Alloc)
		if !ok || !ssaflow.WholeWrittenCellWithin(cell, budget) {
			continue
		}
		if budget.Exhausted() || budget.PoolExhausted() {
			return nil, false
		}
		path, read, ok := ssaflow.AccessPathReadWithin(value, cell, budget)
		if ok && spillStillContainsParameter(cell, parameter, read, budget) {
			return path, true
		}
	}
	return nil, false
}

// Whole-written cells can also hold a different aggregate later. A loaded
// value keeps the contents read at its own snapshot, including through later
// wrappers; the reaching-write query already owns that point-in-time proof.
// An address has no snapshot, so every whole write must agree with the parameter
// before it can stand for the parameter's field at an unknown later use.
func spillStillContainsParameter(cell *ssa.Alloc, parameter ssa.Value, read *ssa.UnOp, budget *ssaflow.SearchBudget) bool {
	storage := NewStorage(budget)
	// Fact path discovery must not build a graph for an opaque aggregate.
	// Exact load identities use the same reaching-write engine as contents.
	storage.writesOnly = true
	if read != nil {
		stored := storage.ContentFromWrites(cell, read)
		return stored.Proven() && storage.Same(stored.Value, parameter).Proven()
	}
	for _, reference := range *cell.Referrers() {
		if !budget.Spend() {
			return false
		}
		store, ok := reference.(*ssa.Store)
		if ok && store.Addr == cell && !storage.Same(store.Val, parameter).Proven() {
			return false
		}
	}
	return !budget.Exhausted() && !budget.PoolExhausted()
}

// ValueAtPath resolves the value stored at path beneath root as observed at
// observation. The root may be an address, such as a local aggregate or a
// pointer, or a load of a whole aggregate, in which case the loaded cell is
// the root. An empty path is the root itself. The address is found by
// following the selections the function actually made, so a path nobody
// selected resolves to nothing.
func ValueAtPath(root ssa.Value, path []string, observation ssa.Instruction) (ssa.Value, bool) { //nolint:ireturn // SSA values keep their concrete forms.
	if len(path) == 0 {
		return root, true
	}
	// The graph knows the slot whether or not the function selected it by
	// that path; the selection walk below is kept for the paths the graph
	// cannot resolve to one object.
	if value, ok := graphValueAtPath(root, path, observation); ok {
		return value, true
	}
	if load, ok := root.(*ssa.UnOp); ok && load.Op == token.MUL {
		root = load.X
	}
	for _, address := range SelectionsOf(root, path) {
		if content := NewStorage(nil).Content(address, observation); content.Proven() {
			return content.Value, true
		}
	}
	return nil, false
}

// SelectionsOf returns every address the function selected beneath root by
// exactly path.
func SelectionsOf(root ssa.Value, path []string) []ssa.Value {
	return SelectionsOfWithin(root, path, nil)
}

// SelectionsOfWithin shares path, address and referrer visits with budget.
// Cutoff returns no selections and cannot establish that a path is absent.
func SelectionsOfWithin(root ssa.Value, path []string, budget *ssaflow.SearchBudget) []ssa.Value {
	frontier := []ssa.Value{root}
	for _, step := range path {
		if !budget.Spend() {
			return nil
		}
		var next []ssa.Value
		for _, address := range frontier {
			if !budget.Spend() {
				return nil
			}
			if address.Referrers() == nil {
				continue
			}
			for _, reference := range *address.Referrers() {
				if !budget.Spend() {
					return nil
				}
				selected, ok := reference.(ssa.Value)
				if !ok {
					continue
				}
				if selection, ok := ssaflow.AccessPathStepsWithin(selected, address, budget); ok && len(selection) == 1 && selection[0] == step {
					next = append(next, selected)
				}
			}
		}
		frontier = next
	}
	if budget.Exhausted() || budget.PoolExhausted() {
		return nil
	}
	return frontier
}

// StoredPathProof identifies the exact observed field/element path or preserves
// an unavailable search. Possible containment does not establish this relation.
type StoredPathProof struct {
	ssaflow.Proof
	Path []string
}

// ProveStoredPathWithin shares graph dispatch, selection/referrer visits and
// storage queries with budget. The storage child retains its QueryBudget cap;
// its cutoff is unknown even when the caller remains available. Graph/alias/type
// internals remain separate costs. Nil retains the default storage allowance.
func ProveStoredPathWithin(root, target ssa.Value, observation ssa.Instruction, budget *ssaflow.SearchBudget) StoredPathProof {
	if !budget.Spend() {
		return storedPathProof(nil, budget, nil)
	}
	if path, ok := graphStoredPath(root, target, observation); ok {
		return storedPathProof(path, budget, nil)
	}
	if load, ok := root.(*ssa.UnOp); ok && load.Op == token.MUL {
		root = load.X
	}
	storageBudget := budget.Within(ssaflow.QueryBudget)
	query := storedPathQuery{budget: budget, storageBudget: storageBudget, storage: NewStorage(storageBudget), target: target, observation: observation}
	path := query.walk(root, nil, 2)
	return storedPathProof(path, budget, storageBudget)
}

type storedPathQuery struct {
	budget, storageBudget *ssaflow.SearchBudget
	storage               *Storage
	target                ssa.Value
	observation           ssa.Instruction
}

func (query storedPathQuery) walk(address ssa.Value, prefix []string, depth int) []string {
	if !query.budget.Spend() || depth == 0 || address.Referrers() == nil {
		return nil
	}
	for _, reference := range *address.Referrers() {
		if !query.budget.Spend() {
			return nil
		}
		selected, ok := reference.(ssa.Value)
		if !ok {
			continue
		}
		step, ok := ssaflow.AccessPathStepsWithin(selected, address, query.budget)
		if !ok || len(step) != 1 {
			continue
		}
		path := append(append([]string(nil), prefix...), step[0])
		if content := query.storage.Content(selected, query.observation); content.Proven() && query.storage.Same(content.Value, query.target).Proven() {
			return path
		}
		if query.storageBudget.Exhausted() || query.storageBudget.PoolExhausted() {
			return nil
		}
		if found := query.walk(selected, path, depth-1); found != nil {
			return found
		}
	}
	return nil
}

func storedPathProof(path []string, budget, storageBudget *ssaflow.SearchBudget) StoredPathProof {
	if budget.Exhausted() || budget.PoolExhausted() || storageBudget.Exhausted() || storageBudget.PoolExhausted() {
		return StoredPathProof{Proof: ssaflow.Proof{State: ssaflow.EvidenceUnknown, Reason: ssaflow.EvidenceBudgetExhausted}}
	}
	if path == nil {
		return StoredPathProof{Proof: ssaflow.Proof{State: ssaflow.EvidenceUnknown, Reason: ssaflow.EvidenceUnavailable}}
	}
	return StoredPathProof{Proof: ssaflow.Proof{State: ssaflow.EvidenceProven, Reason: ssaflow.EvidenceSameAccessPath}, Path: path}
}
