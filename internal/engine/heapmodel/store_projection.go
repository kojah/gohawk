package heapmodel

import (
	"go/token"

	proofs "github.com/kojah/gohawk/internal/engine/proof"
	"github.com/kojah/gohawk/internal/engine/ssaflow"
	cfg "github.com/kojah/gohawk/internal/engine/ssaflow/cfg"
	"golang.org/x/tools/go/ssa"
)

// Projection evidence selects exact aggregate paths and proves their stability
// at an observation. An ambiguous selection or intervening mutation ends the
// proof rather than widening the path into possible identity.

// Projection stability proves that an exact field or constant-index path still
// names storage owned by its original root at one observation. Assigning or
// exposing either the root or selected address before that observation stops
// the proof; uses after the observation do not retroactively invalidate it.

// Projection proves that value is a strict, non-empty
// access path from root whose selected storage cannot have been replaced before
// observation. It intentionally rejects phi-selected roots and roots without a
// source instruction, because neither supplies one exact ownership interval.
func (storage *Storage) Projection(value, root ssa.Value, observation ssa.Instruction) proofs.IdentityProof {
	proof := storage.proveUnmodifiedProjection(value, root, observation)
	if !proof.Proven() {
		proof = storage.unknown(proof.Reason, observation).Proof
	}
	return proofs.IdentityProof{Proof: proof}
}

func (storage *Storage) proveUnmodifiedProjection(value, root ssa.Value, observation ssa.Instruction) proofs.Proof {
	unavailable := proofs.Proof{Reason: proofs.EvidenceStorageProjectionModified}
	if !storage.budget.Spend() {
		return proofs.Proof{Reason: proofs.EvidenceBudgetExhausted}
	}
	if observation == nil || root == nil || root.Parent() != observation.Parent() {
		return unavailable
	}
	origin, ok := root.(ssa.Instruction)
	if !ok || origin.Block() == nil {
		return unavailable
	}
	if _, ambiguous := root.(*ssa.Phi); ambiguous {
		return unavailable
	}
	projection := ProveStrictProjectionPathWithin(value, root, storage.budget)
	if projection.Reason == proofs.EvidenceBudgetExhausted {
		return projection.Proof
	}
	if !projection.Proven() {
		return unavailable
	}
	address := projectedStorageAddress(value, storage.budget)
	if address == nil || !storage.projectionAddressStableBetween(address, root, origin, observation) {
		return unavailable
	}
	if !storage.rootDoesNotEscapeBetween(root, origin, observation, map[ssa.Value]bool{}) {
		return unavailable
	}
	return proofs.Proof{State: proofs.EvidenceProven, Reason: proofs.EvidenceSameAccessPath, Provenance: proofs.EvidenceFromLocalSSA}
}

func projectedStorageAddress(value ssa.Value, budget *proofs.SearchBudget) ssa.Value { //nolint:ireturn // SSA address forms are intentionally preserved.
	if !budget.Spend() {
		return nil
	}
	if inner, ok := ssaflow.UnwrapTransparentValue(
		value, ssaflow.TransparentChangeInterface|ssaflow.TransparentChangeType|ssaflow.TransparentConvert|ssaflow.TransparentMakeInterface,
	); ok {
		return projectedStorageAddress(inner, budget)
	}
	switch typed := value.(type) {
	case *ssa.UnOp:
		if typed.Op == token.MUL {
			return typed.X
		}
	case *ssa.FieldAddr, *ssa.IndexAddr:
		return value
	}
	return nil
}

func (storage *Storage) projectionAddressStableBetween(address, root ssa.Value, origin, observation ssa.Instruction) bool {
	for _, block := range observation.Parent().Blocks {
		for _, instruction := range block.Instrs {
			if !storage.budget.Spend() {
				return false
			}
			candidate, ok := instruction.(ssa.Value)
			if !ok || !addressValue(candidate) {
				continue
			}
			same := ssaflow.SameAccessPathWithin(
				ssaflow.AccessPath{Value: candidate, Root: root}, ssaflow.AccessPath{Value: address, Root: root},
				storage.budget,
			)
			if storage.budget.Exhausted() || storage.budget.PoolExhausted() {
				return false
			}
			if !same {
				continue
			}
			if !storage.addressDoesNotEscapeBetween(candidate, origin, observation, map[ssa.Value]bool{}) {
				return false
			}
		}
	}
	return true
}

func addressValue(value ssa.Value) bool {
	switch value.(type) {
	case *ssa.FieldAddr, *ssa.IndexAddr:
		return true
	default:
		return false
	}
}

func (storage *Storage) addressDoesNotEscapeBetween(address ssa.Value, origin, observation ssa.Instruction, seen map[ssa.Value]bool) bool {
	return storage.projectionUsesPreserveStorage(address, origin, observation, seen, storage.addressProjectionUse)
}

func (storage *Storage) rootDoesNotEscapeBetween(root ssa.Value, origin, observation ssa.Instruction, seen map[ssa.Value]bool) bool {
	return storage.projectionUsesPreserveStorage(root, origin, observation, seen, storage.rootProjectionUse)
}

// Both projection boundaries use the same observation window and wrapper
// traversal. Keep depth-first visits and reject repeats: changing either can
// change which query spends the shared budget or accepts a circular proof.
func (storage *Storage) projectionUsesPreserveStorage(
	value ssa.Value, origin, observation ssa.Instruction, seen map[ssa.Value]bool,
	accept func(ssa.Value, ssa.Instruction) bool,
) bool {
	if value == nil || value.Referrers() == nil || seen[value] {
		return false
	}
	seen[value] = true
	for _, reference := range *value.Referrers() {
		if !storage.budget.Spend() {
			return false
		}
		within := instructionWithinObservation(reference, origin, observation, storage.budget)
		if storage.budget.Exhausted() || storage.budget.PoolExhausted() {
			return false
		}
		if !within || accept(value, reference) {
			continue
		}
		if wrapper, ok := outwardProjectionWrapper(reference, value); ok &&
			storage.projectionUsesPreserveStorage(wrapper, origin, observation, seen, accept) {
			continue
		}
		return false
	}
	return true
}

func (storage *Storage) addressProjectionUse(address ssa.Value, reference ssa.Instruction) bool {
	switch typed := reference.(type) {
	case *ssa.DebugRef:
		return true
	case *ssa.Call, *ssa.Defer, *ssa.Go:
		return storage.effects.Call(reference, address).PreservesStorage()
	case *ssa.UnOp:
		return typed.Op == token.MUL && typed.X == address
	}
	return false
}

func (storage *Storage) rootProjectionUse(root ssa.Value, reference ssa.Instruction) bool {
	switch typed := reference.(type) {
	case *ssa.DebugRef, *ssa.FieldAddr, *ssa.IndexAddr:
		return true
	case *ssa.Call, *ssa.Defer, *ssa.Go:
		return storage.effects.Call(reference, root).PreservesStorage()
	case *ssa.BinOp:
		return (typed.Op == token.EQL || typed.Op == token.NEQ) &&
			(ssaflow.DefinitelyNilWithin(typed.X, storage.budget) || ssaflow.DefinitelyNilWithin(typed.Y, storage.budget))
	}
	return false
}

func outwardProjectionWrapper(reference ssa.Instruction, inner ssa.Value) (ssa.Value, bool) { //nolint:ireturn // Preserve the concrete SSA wrapper.
	wrapper, ok := reference.(ssa.Value)
	if !ok {
		return nil, false
	}
	unwrapped, ok := ssaflow.UnwrapTransparentValue(
		wrapper, ssaflow.TransparentChangeInterface|ssaflow.TransparentChangeType|ssaflow.TransparentConvert|ssaflow.TransparentMakeInterface,
	)
	return wrapper, ok && unwrapped == inner
}

func instructionWithinObservation(candidate, origin, observation ssa.Instruction, budget *proofs.SearchBudget) bool {
	return candidate != nil && candidate != origin && cfg.InstructionMayFollowWithin(origin, candidate, budget) &&
		cfg.InstructionMayFollowWithin(candidate, observation, budget)
}

// ProjectionPathProof proves a strict projection and retains its exact static
// parameter path when available. A proven storage-derived projection can have
// no Path; that cannot publish an exact field cleanup contract.
type ProjectionPathProof struct {
	proofs.Proof
	Path []string
}

// ProveStrictProjectionPathWithin shares path and stored-value visits with
// budget while retaining the default QueryBudget cap. A child cutoff remains
// unknown even if its parent still has allowance. This establishes a path,
// not stability or ownership. Parameter paths reuse read-time spill identity;
// other roots retain the existing storage-derived projection rule. Graph and
// alias internals retain separate costs.
func ProveStrictProjectionPathWithin(value, root ssa.Value, budget *proofs.SearchBudget) ProjectionPathProof {
	child := budget.Within(proofs.QueryBudget)
	if _, parameter := root.(*ssa.Parameter); parameter {
		path, known := AccessPathFromParameterWithin(value, root, child)
		if child.Exhausted() || child.PoolExhausted() {
			return ProjectionPathProof{Proof: proofs.Proof{Reason: proofs.EvidenceBudgetExhausted}}
		}
		if known && len(path) > 0 {
			return ProjectionPathProof{Proof: proofs.Proof{
				State: proofs.EvidenceProven, Reason: proofs.EvidenceSameAccessPath, Provenance: proofs.EvidenceFromLocalSSA,
			}, Path: path}
		}
	}
	depth, ok := strictAccessPathDepth(value, root, map[ssa.Value]bool{}, child)
	if child.Exhausted() || child.PoolExhausted() {
		return ProjectionPathProof{Proof: proofs.Proof{Reason: proofs.EvidenceBudgetExhausted}}
	}
	state, reason := proofs.EvidenceDisproven, proofs.EvidenceNotFound
	if ok && depth > 0 {
		state, reason = proofs.EvidenceProven, proofs.EvidenceSameAccessPath
	}
	return ProjectionPathProof{Proof: proofs.Proof{State: state, Reason: reason, Provenance: proofs.EvidenceFromLocalSSA}}
}

func strictAccessPathDepth(value, root ssa.Value, seen map[ssa.Value]bool, budget *proofs.SearchBudget) (int, bool) {
	if value == nil || root == nil || seen[value] || !budget.Spend() {
		return 0, false
	}
	if value == root {
		return 0, true
	}
	seen[value] = true
	if inner, ok := ssaflow.UnwrapTransparentValue(
		value, ssaflow.TransparentChangeInterface|ssaflow.TransparentChangeType|ssaflow.TransparentConvert|ssaflow.TransparentMakeInterface,
	); ok {
		return strictAccessPathDepth(inner, root, seen, budget)
	}
	switch typed := value.(type) {
	case *ssa.FieldAddr:
		depth, ok := strictAccessPathDepth(typed.X, root, seen, budget)
		return depth + 1, ok
	case *ssa.IndexAddr:
		if _, ok := ssaflow.ConstantIndex(typed.Index); !ok {
			return 0, false
		}
		depth, ok := strictAccessPathDepth(typed.X, root, seen, budget)
		return depth + 1, ok
	case *ssa.UnOp:
		if typed.Op == token.MUL {
			if depth, ok := strictAccessPathDepth(typed.X, root, seen, budget); ok {
				return depth, true
			}
			stored := NewStorage(budget).Content(typed.X, typed)
			if !stored.Proven() {
				return 0, false
			}
			return strictAccessPathDepth(stored.Value, root, seen, budget)
		}
	}
	return 0, false
}

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
func AccessPathFromParameterWithin(value, parameter ssa.Value, budget *proofs.SearchBudget) ([]string, bool) {
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
func spillStillContainsParameter(cell *ssa.Alloc, parameter ssa.Value, read *ssa.UnOp, budget *proofs.SearchBudget) bool {
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
func SelectionsOfWithin(root ssa.Value, path []string, budget *proofs.SearchBudget) []ssa.Value {
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
	proofs.Proof
	Path []string
}

// ProveStoredPathWithin shares graph dispatch, selection/referrer visits and
// storage queries with budget. The storage child retains its QueryBudget cap;
// its cutoff is unknown even when the caller remains available. Graph/alias/type
// internals remain separate costs. Nil retains the default storage allowance.
func ProveStoredPathWithin(root, target ssa.Value, observation ssa.Instruction, budget *proofs.SearchBudget) StoredPathProof {
	if !budget.Spend() {
		return storedPathProof(nil, budget, nil)
	}
	if path, ok := graphStoredPath(root, target, observation); ok {
		return storedPathProof(path, budget, nil)
	}
	if load, ok := root.(*ssa.UnOp); ok && load.Op == token.MUL {
		root = load.X
	}
	storageBudget := budget.Within(proofs.QueryBudget)
	query := storedPathQuery{budget: budget, storageBudget: storageBudget, storage: NewStorage(storageBudget), target: target, observation: observation}
	path := query.walk(root, nil, 2)
	return storedPathProof(path, budget, storageBudget)
}

type storedPathQuery struct {
	budget, storageBudget *proofs.SearchBudget
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

func storedPathProof(path []string, budget, storageBudget *proofs.SearchBudget) StoredPathProof {
	if budget.Exhausted() || budget.PoolExhausted() || storageBudget.Exhausted() || storageBudget.PoolExhausted() {
		return StoredPathProof{Proof: proofs.Proof{State: proofs.EvidenceUnknown, Reason: proofs.EvidenceBudgetExhausted}}
	}
	if path == nil {
		return StoredPathProof{Proof: proofs.Proof{State: proofs.EvidenceUnknown, Reason: proofs.EvidenceUnavailable}}
	}
	return StoredPathProof{Proof: proofs.Proof{State: proofs.EvidenceProven, Reason: proofs.EvidenceSameAccessPath}, Path: path}
}
