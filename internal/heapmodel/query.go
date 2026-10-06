package heapmodel

import (
	"go/types"

	proofs "github.com/kojah/gohawk/internal/proof"
	"github.com/kojah/gohawk/internal/ssaflow"
	"golang.org/x/tools/go/ssa"
)

// ProveMayAlias asks one function's graph whether two values may name the
// same object. An unavailable graph falls back to the structural value walk.
func ProveMayAlias(value, target ssa.Value) proofs.AliasProof {
	graph := regionsOf(value)
	if graph.available && valueFunction(target) == graph.function {
		return graph.aliasProof(value, target)
	}
	return proofs.AliasProof{
		Aliases:    ssaflow.StructurallySame(value, target),
		Reason:     proofs.EvidenceStructuralWalk,
		Provenance: proofs.EvidenceFromLocalSSA,
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
	if !CanHoldReference(owner.Type()) {
		return false
	}
	graph := regionsOf(owner)
	return graph.available && valueFunction(value) == graph.function && graph.contains(owner, value)
}

// ContainsAt reports possible containment at one instruction, with known
// false distinguished from a graph that could not answer.
func ContainsAt(owner, value ssa.Value, at ssa.Instruction) (bool, bool) {
	if !CanHoldReference(owner.Type()) {
		return false, true
	}
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

// CanHoldReference reports whether a value of type value can refer to another
// object. A string, a number, or a struct or array made only of them cannot:
// a string's bytes are never an object the program releases. The points-to
// graph can still link such a value to the object it was read from, as a
// string field is to its owner, so containment asks the type first. SSA result
// tuples can retain references only through their component types.
// Real-world form: ForceCLI passes a zip entry's Name to strings.HasPrefix
// while the entry's reader is open,
// https://github.com/ForceCLI/force/blob/662af739b980a568fa55e3a4d7efe65cf2ec15b1/command/fetch.go#L321-L330
func CanHoldReference(value types.Type) bool {
	return anyByValueType(value, func(value types.Type) bool {
		switch value := value.Underlying().(type) {
		case *types.Basic:
			return value.Kind() == types.UnsafePointer || value.Kind() == types.Invalid
		case *types.Struct, *types.Array, *types.Tuple:
			return false
		default:
			return true
		}
	})
}
