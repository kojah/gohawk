package lockorder

import (
	"go/token"
	"maps"
	"slices"

	"github.com/kojah/gohawk/internal/analysisutil"
	ssautil "github.com/kojah/gohawk/internal/analysisutil/ssa"
	"github.com/kojah/gohawk/internal/check"

	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/ssa"
)

func callerOwnedLocks(function *ssa.Function) map[string]bool {
	type firstAction struct {
		operation mutexOperation
		position  token.Pos
	}
	first := map[string]firstAction{}
	for _, block := range function.Blocks {
		for _, instruction := range block.Instrs {
			operation, identity, _, ok := mutexAction(instruction)
			if !ok || instruction.Pos() == token.NoPos {
				continue
			}
			current, exists := first[identity]
			if !exists || instruction.Pos() < current.position {
				first[identity] = firstAction{operation: operation, position: instruction.Pos()}
			}
		}
	}
	result := map[string]bool{}
	for identity, action := range first {
		result[identity] = action.operation == mutexRelease
	}
	return result
}

func acquireLock(pass *analysis.Pass, instruction ssa.Instruction, held []string, identity string, relations map[lockRelation]token.Pos) []string {
	if slices.Contains(held, identity) {
		check.Reportf(pass, check.LockRecursiveAcquire, instruction.Pos(), "lock %s is acquired while already held", identity)
		return held
	}
	for _, owner := range held {
		relation := lockRelation{from: owner, to: identity}
		reverse := lockRelation{from: identity, to: owner}
		if _, exists := relations[reverse]; exists {
			check.Reportf(pass, check.LockContradictoryOrder, instruction.Pos(), "contradictory lock order: %s and %s", identity, owner)
		}
		relations[relation] = instruction.Pos()
	}
	return append(held, identity)
}

func appendUniquePosition(positions []token.Pos, candidate token.Pos) []token.Pos {
	if !slices.Contains(positions, candidate) {
		return append(positions, candidate)
	}
	return positions
}

func appendUniqueString(values []string, candidate string) []string {
	if !slices.Contains(values, candidate) {
		return append(values, candidate)
	}
	return values
}

func returnedUnlockOwner(returned *ssa.Return, values []ssa.Value) bool {
	for _, result := range returned.Results {
		for _, value := range values {
			if ssautil.ValueCallsMethod(result, "Unlock", value) || ssautil.ValueCallsMethod(result, "RUnlock", value) {
				return true
			}
		}
	}
	return false
}

func dynamicIndexedMutex(value ssa.Value) bool {
	return dynamicIndexedMutexSeen(value, make(map[ssa.Value]bool))
}

func dynamicIndexedMutexSeen(value ssa.Value, seen map[ssa.Value]bool) bool {
	if value == nil {
		return false
	}
	if seen[value] {
		// Phi nodes and interface conversions can form SSA cycles. Revisiting a
		// value adds no new evidence; the other reachable edges still determine
		// whether the mutex originated from a dynamic collection selection.
		return false
	}
	seen[value] = true
	switch typed := value.(type) {
	case *ssa.IndexAddr:
		_, constant := typed.Index.(*ssa.Const)
		return !constant
	case *ssa.Index, *ssa.Lookup:
		return true
	case *ssa.Extract:
		return dynamicIndexedMutexSeen(typed.Tuple, seen)
	case *ssa.Phi:
		for _, edge := range typed.Edges {
			if dynamicIndexedMutexSeen(edge, seen) {
				return true
			}
		}
	case *ssa.FieldAddr:
		return dynamicIndexedMutexSeen(typed.X, seen)
	case *ssa.ChangeInterface:
		return dynamicIndexedMutexSeen(typed.X, seen)
	case *ssa.ChangeType:
		return dynamicIndexedMutexSeen(typed.X, seen)
	case *ssa.Convert:
		return dynamicIndexedMutexSeen(typed.X, seen)
	case *ssa.MakeInterface:
		return dynamicIndexedMutexSeen(typed.X, seen)
	case *ssa.UnOp:
		return dynamicIndexedMutexSeen(typed.X, seen)
	}
	return false
}

func releaseLock(held []string, identity string) []string {
	for index, candidate := range slices.Backward(held) {
		if candidate == identity {
			return append(held[:index], held[index+1:]...)
		}
	}
	return held
}

func mutexAction(instruction ssa.Instruction) (mutexOperation, string, ssa.Value, bool) {
	common := ssautil.InstructionCall(instruction)
	if common == nil {
		return 0, "", nil, false
	}
	name := ssautil.CallName(common)
	var operation mutexOperation
	switch name {
	case "Lock", "RLock":
		operation = mutexAcquire
	case "Unlock", "RUnlock":
		operation = mutexRelease
	default:
		return 0, "", nil, false
	}
	receiver := ssautil.CallReceiver(common)
	receiver = concreteMutexReceiver(receiver, map[ssa.Value]bool{})
	if receiver == nil {
		return 0, "", nil, false
	}
	identity := lockIdentity(receiver, map[ssa.Value]bool{})
	return operation, identity, receiver, identity != ""
}

// concreteMutexReceiver unwraps interface values only when every possible SSA
// origin proves the same concrete sync mutex identity.
func concreteMutexReceiver(value ssa.Value, seen map[ssa.Value]bool) ssa.Value { //nolint:ireturn // SSA values have several concrete forms.
	if value == nil || seen[value] {
		return nil
	}
	seen[value] = true
	if analysisutil.NamedType(value.Type(), "sync", "Mutex") || analysisutil.NamedType(value.Type(), "sync", "RWMutex") {
		return value
	}
	switch typed := value.(type) {
	case *ssa.MakeInterface:
		return concreteMutexReceiver(typed.X, seen)
	case *ssa.ChangeInterface:
		return concreteMutexReceiver(typed.X, seen)
	case *ssa.Phi:
		var resolved ssa.Value
		var identity string
		for _, edge := range typed.Edges {
			candidate := concreteMutexReceiver(edge, maps.Clone(seen))
			candidateIdentity := lockIdentity(candidate, map[ssa.Value]bool{})
			if candidate == nil || candidateIdentity == "" || identity != "" && candidateIdentity != identity {
				return nil
			}
			resolved = candidate
			identity = candidateIdentity
		}
		return resolved
	default:
		return nil
	}
}

func appendLockValue(values []ssa.Value, candidate ssa.Value) []ssa.Value {
	candidateIdentity := lockIdentity(candidate, map[ssa.Value]bool{})
	for _, value := range values {
		if ssautil.SameValue(value, candidate) || candidateIdentity != "" && lockIdentity(value, map[ssa.Value]bool{}) == candidateIdentity {
			return values
		}
	}
	return append(values, candidate)
}
