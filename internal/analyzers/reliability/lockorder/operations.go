package lockorder

import (
	"go/token"
	"maps"
	"slices"

	"github.com/kojah/gohawk/internal/check"
	"github.com/kojah/gohawk/internal/ssaflow"
	"github.com/kojah/gohawk/internal/syntax"

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
		// A lock selected by a loop iteration is a different mutex each time
		// the same instruction runs, so re-acquiring its identity on the next
		// iteration is not recursion. multigres locks every key mutex in a
		// loop and defers the unlocks:
		// https://github.com/multigres/multigres/blob/360b8f123dff8ad6bcc721acaec103c52081bebd/go/tools/viperutil/internal/sync/sync.go#L236-L240
		if !loopVariantLock(instruction) {
			check.Reportf(pass, check.LockRecursiveAcquire, instruction.Pos(), "lock %s is acquired while already held", identity)
		}
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
			if ssaflow.ValueCallsMethod(result, "Unlock", value) || ssaflow.ValueCallsMethod(result, "RUnlock", value) {
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
	if inner, ok := ssaflow.UnwrapTransparentValue(
		value,
		ssaflow.TransparentChangeInterface|ssaflow.TransparentChangeType|ssaflow.TransparentConvert|ssaflow.TransparentMakeInterface,
	); ok {
		return dynamicIndexedMutexSeen(inner, seen)
	}
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
	common := ssaflow.InstructionCall(instruction)
	if common == nil {
		return 0, "", nil, false
	}
	name := ssaflow.CallName(common)
	var operation mutexOperation
	switch name {
	case "Lock", "RLock":
		operation = mutexAcquire
	case "Unlock", "RUnlock":
		operation = mutexRelease
	default:
		return 0, "", nil, false
	}
	receiver := ssaflow.CallReceiver(common)
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
	if syntax.NamedType(value.Type(), "sync", "Mutex") || syntax.NamedType(value.Type(), "sync", "RWMutex") {
		return value
	}
	if inner, ok := ssaflow.UnwrapTransparentValue(
		value,
		ssaflow.TransparentChangeInterface|ssaflow.TransparentMakeInterface,
	); ok {
		return concreteMutexReceiver(inner, seen)
	}
	switch typed := value.(type) {
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
		if ssaflow.SameValue(value, candidate) || candidateIdentity != "" && lockIdentity(value, map[ssa.Value]bool{}) == candidateIdentity {
			return values
		}
	}
	return append(values, candidate)
}

// loopVariantLock reports whether the acquisition's receiver is selected by a
// loop iteration: it is defined inside a cycle and derives from a phi or map
// iterator in that cycle, as a range element does. A field of a receiver or
// a package variable locked inside a loop is the same mutex every time and
// stays reportable.
func loopVariantLock(instruction ssa.Instruction) bool {
	receiver := ssaflow.CallReceiver(ssaflow.InstructionCall(instruction))
	return receiver != nil && loopVariantValue(receiver, map[ssa.Value]bool{})
}

func loopVariantValue(value ssa.Value, seen map[ssa.Value]bool) bool {
	if value == nil || seen[value] {
		return false
	}
	seen[value] = true
	switch typed := value.(type) {
	case *ssa.Phi:
		return ssaflow.BlockInCycle(typed.Block())
	case *ssa.Extract:
		if next, ok := typed.Tuple.(*ssa.Next); ok {
			return ssaflow.BlockInCycle(next.Block())
		}
		return loopVariantValue(typed.Tuple, seen)
	case *ssa.UnOp:
		return loopVariantValue(typed.X, seen)
	case *ssa.IndexAddr:
		return loopVariantValue(typed.Index, seen) || loopVariantValue(typed.X, seen)
	case *ssa.Index:
		return loopVariantValue(typed.Index, seen) || loopVariantValue(typed.X, seen)
	case *ssa.FieldAddr:
		return loopVariantValue(typed.X, seen)
	case *ssa.Field:
		return loopVariantValue(typed.X, seen)
	case *ssa.BinOp:
		// A range index is the loop phi plus one.
		return loopVariantValue(typed.X, seen) || loopVariantValue(typed.Y, seen)
	case *ssa.Call:
		// A lookup keyed by the iteration, such as the per-session state a
		// cleanup loop fetches before locking it, selects a different mutex
		// each time; a call with no loop-variant argument returns the same one.
		// CodeKanban locks every session's state this way:
		// https://github.com/fy0/CodeKanban/blob/745699cb67d3d34cec4793168f148eb61e43e766/service/websession/history_cleanup.go#L262-L276
		return slices.ContainsFunc(typed.Common().Args, func(argument ssa.Value) bool {
			return loopVariantValue(argument, seen)
		})
	case *ssa.ChangeType, *ssa.ChangeInterface, *ssa.Convert, *ssa.MakeInterface:
		inner, ok := ssaflow.UnwrapTransparentValue(
			value,
			ssaflow.TransparentChangeInterface|ssaflow.TransparentChangeType|ssaflow.TransparentConvert|ssaflow.TransparentMakeInterface,
		)
		return ok && loopVariantValue(inner, seen)
	}
	return false
}
