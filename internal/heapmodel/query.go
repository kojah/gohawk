package heapmodel

import (
	"github.com/kojah/gohawk/internal/ssaflow"
	"golang.org/x/tools/go/ssa"
)

// ProveMayAlias asks one function's graph whether two values may name the
// same object. An unavailable graph falls back to the structural value walk.
func ProveMayAlias(value, target ssa.Value) ssaflow.AliasProof {
	graph := regionsOf(value)
	if graph.available && valueFunction(target) == graph.function {
		return graph.aliasProof(value, target)
	}
	return ssaflow.AliasProof{
		Aliases:    ssaflow.StructurallySame(value, target),
		Reason:     ssaflow.EvidenceStructuralWalk,
		Provenance: ssaflow.EvidenceFromLocalSSA,
	}
}

// DefinitelySame reports exact graph identity, including through stable slots.
func DefinitelySame(left, right ssa.Value) bool {
	graph := regionsOf(left)
	return graph.available && valueFunction(right) == graph.function && graph.mustSame(left, right)
}

// ContentValue returns the exact SSA value held by an address at observation.
func ContentValue(address ssa.Value, observation ssa.Instruction) (ssa.Value, bool) {
	return regionsOf(address).contentValue(address, observation)
}

// Contains reports possible object containment in a function's graph.
func Contains(owner, value ssa.Value) bool {
	graph := regionsOf(owner)
	return graph.available && valueFunction(value) == graph.function && graph.contains(owner, value)
}

// ContainsAt reports possible containment at one instruction, with known
// false distinguished from a graph that could not answer.
func ContainsAt(owner, value ssa.Value, at ssa.Instruction) (bool, bool) {
	graph := regionsOf(owner)
	if !graph.available || valueFunction(value) != graph.function {
		return false, false
	}
	return graph.containsAt(owner, value, at)
}

// ValueAtPath finds the exact object at a static path beneath root.
func graphValueAtPath(root ssa.Value, path []string, at ssa.Instruction) (ssa.Value, bool) {
	return regionsOf(root).valueAtPath(root, path, at)
}

// ContentIsNilAt requires the observed slot to hold nil on every path.
// Nested pointer fields follow only exact pointees; opaque contents are
// unknown, not a proof of nil.
func ContentIsNilAt(root ssa.Value, path []string, at ssa.Instruction) bool {
	if at == nil || at.Parent() == nil {
		return false
	}
	return regionsOfFunction(at.Parent()).contentIsNil(root, path, at)
}

// ExclusiveAt proves a graph object's caller/local exclusivity at one point.
func ExclusiveAt(value ssa.Value, at ssa.Instruction) (ExclusiveObject, bool) {
	if at == nil || at.Parent() == nil {
		return ExclusiveObject{}, false
	}
	return regionsOfFunction(at.Parent()).exclusiveAt(value, at)
}

// StoredPath finds a path from root to target in the observed graph.
func graphStoredPath(root, target ssa.Value, at ssa.Instruction) ([]string, bool) {
	return regionsOf(root).storedPath(root, target, at)
}

// DeferredCellMatch distinguishes a captured cell that contains exactly the
// target from one whose every possible occupant contains it indirectly.
type DeferredCellMatch uint8

const (
	DeferredCellUnknown DeferredCellMatch = iota
	DeferredCellExact
	DeferredCellContains
)

// DeferredCellRelation reads the cell when deferred calls execute. Known is
// false when either side could not be read; callers must not use a fallback
// proof in that case. A stale or unrelated occupant prevents an exact claim.
func DeferredCellRelation(cell *ssa.Alloc, target ssa.Value, invocation ssa.Instruction) (DeferredCellMatch, bool) {
	graph := regionsOf(cell)
	held, ok := graph.contentWhenDeferredRun(cell, invocation)
	if !ok || len(held) == 0 {
		return DeferredCellUnknown, false
	}
	object, ok := graph.pointsTo(target)
	if !ok {
		return DeferredCellUnknown, false
	}
	targetSlot, exact := singleSlot(object)
	isTarget, contains := exact, true
	for entry, stale := range held {
		if entry.region.kind == regionNil {
			continue
		}
		if entry != targetSlot || stale {
			isTarget = false
		}
		if !graph.everContained(entry, object) {
			contains = false
		}
	}
	if isTarget {
		return DeferredCellExact, true
	}
	if contains {
		return DeferredCellContains, true
	}
	return DeferredCellUnknown, true
}
