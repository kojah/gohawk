package lockorder

import (
	"go/token"
	"slices"

	"github.com/kojah/gohawk/internal/heapmodel"
	"github.com/kojah/gohawk/internal/ssaflow"
	"github.com/kojah/gohawk/internal/syntax"

	proofs "github.com/kojah/gohawk/internal/proof"
	"golang.org/x/tools/go/ssa"
)

type callerReleaseProof struct {
	proven bool
	reason lockReason
}

func appendUniqueString(values []string, candidate string) []string {
	if !slices.Contains(values, candidate) {
		return append(values, candidate)
	}
	return values
}

// optionalLoadedGuard identifies an acquisition whose guard tests mutable
// storage rather than one SSA value. Repeated reads of that slot have unrelated
// condition identities, while lock identity deliberately names the slot. Until
// that relationship is proved, an unreleased path may be infeasible. Decline
// missing-release, including changed guards, rather than assume either stable
// contents or a leak; direct mutex parameters and Boolean guards stay precise.
// https://github.com/devld/go-drive/blob/91c3ac7253642bf58629d6a87cf7ab718f2d3827/common/utils/path_tree.go#L133-L141
func optionalLoadedGuard(instruction ssa.Instruction, identity string, budget *proofs.SearchBudget) bool {
	block := instruction.Block()
	if len(block.Preds) != 1 {
		return false
	}
	pred := block.Preds[0]
	if len(pred.Instrs) == 0 {
		return false
	}
	branch, ok := pred.Instrs[len(pred.Instrs)-1].(*ssa.If)
	if !ok {
		return false
	}
	// A loaded Boolean may be read again around Unlock. Its SSA loads differ,
	// but this flow cannot prove their contents differ. Keep that path unknown
	// rather than invent an inconsistent acquisition/release guard. This also
	// declines genuinely changed loaded guards; direct SSA booleans stay precise.
	// https://github.com/kataras/neffos/blob/60df99445da7c0b56c1b81a32161f86b53fd462a/conn.go#L813-L853
	if loaded, ok := branch.Cond.(*ssa.UnOp); ok && loaded.Op == token.MUL {
		return true // An SSA If condition is necessarily Boolean.
	}
	// A trivial Boolean field getter has the same uncertainty as a direct
	// load. Its body supplies no promise that separate calls agree. This
	// deliberately also declines changed-field bugs rather than proving a
	// release from a construction-time convention.
	// https://github.com/pion/sctp/blob/a09fb03516289d7cd89bc589ac49ee84ac331c62/stream.go#L324-L350
	if call, ok := branch.Cond.(*ssa.Call); ok && booleanFieldGetter(call) {
		return true
	}
	comparison, ok := branch.Cond.(*ssa.BinOp)
	if !ok || (comparison.Op != token.EQL && comparison.Op != token.NEQ) {
		return false
	}
	for _, pair := range [][2]ssa.Value{{comparison.X, comparison.Y}, {comparison.Y, comparison.X}} {
		loaded, ok := pair[0].(*ssa.UnOp)
		if ok && loaded.Op == token.MUL && ssaflow.DefinitelyNil(pair[1]) && lockIdentityWithin(loaded, budget) == identity {
			return true
		}
	}
	return false
}

func booleanFieldGetter(call *ssa.Call) bool {
	callee := call.Common().StaticCallee()
	if callee == nil || len(callee.Params) != 1 || len(call.Common().Args) != 1 || len(callee.Blocks) != 1 {
		return false
	}
	instructions := callee.Blocks[0].Instrs
	if len(instructions) != 3 {
		return false
	}
	field, ok := instructions[0].(*ssa.FieldAddr)
	if !ok || field.X != callee.Params[0] {
		return false
	}
	loaded, ok := instructions[1].(*ssa.UnOp)
	if !ok || loaded.Op != token.MUL || loaded.X != field {
		return false
	}
	returned, ok := instructions[2].(*ssa.Return)
	return ok && len(returned.Results) == 1 && returned.Results[0] == loaded
}

// mutexForms are the wrappers a mutex keeps its origin through.
const mutexForms = ssaflow.TransparentChangeInterface | ssaflow.TransparentChangeType | ssaflow.TransparentConvert | ssaflow.TransparentMakeInterface

func dynamicIndexedMutex(value ssa.Value) bool {
	return ssaflow.NewReachingWalk(mutexForms).Any(value, dynamicIndexedMutexLeaf)
}

func dynamicIndexedMutexLeaf(walk ssaflow.ReachingWalk, value ssa.Value) bool {
	switch typed := value.(type) {
	case *ssa.IndexAddr:
		_, constant := typed.Index.(*ssa.Const)
		return !constant
	case *ssa.Index, *ssa.Lookup:
		return true
	case *ssa.Extract:
		return walk.Any(typed.Tuple, dynamicIndexedMutexLeaf)
	case *ssa.FieldAddr:
		return walk.Any(typed.X, dynamicIndexedMutexLeaf)
	case *ssa.UnOp:
		return walk.Any(typed.X, dynamicIndexedMutexLeaf)
	}
	return false
}

// readModeAcquisition reports whether the instruction takes a lock for reading.
// sync.Mutex has no RLock, so the name alone identifies the shared mode.
func readModeAcquisition(instruction ssa.Instruction) bool {
	common := ssaflow.InstructionCall(instruction)
	return common != nil && ssaflow.CallName(common) == "RLock"
}

// readModeRelease reports whether the instruction releases a lock held for
// reading.
func readModeRelease(instruction ssa.Instruction) bool {
	common := ssaflow.InstructionCall(instruction)
	return common != nil && ssaflow.CallName(common) == "RUnlock"
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
	return mutexActionWithin(instruction, nil)
}

func mutexActionWithin(instruction ssa.Instruction, budget *proofs.SearchBudget) (mutexOperation, string, ssa.Value, bool) {
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
	receiver, known := concreteMutexIdentityWithin(ssaflow.CallReceiver(common), budget)
	if !known {
		return 0, "", nil, false
	}
	return operation, receiver.identity, receiver.value, true
}

type mutexReceiverIdentity struct {
	value    ssa.Value
	identity string
}

// Concrete receiver selection computes identity once per reaching leaf. The
// agreement key and final action carry that same proof; neither redoes storage.
func concreteMutexIdentityWithin(value ssa.Value, budget *proofs.SearchBudget) (mutexReceiverIdentity, bool) {
	walk := ssaflow.NewReachingWalk(ssaflow.TransparentChangeInterface | ssaflow.TransparentMakeInterface).Within(budget)
	leaf := func(_ ssaflow.ReachingWalk, candidate ssa.Value) (mutexReceiverIdentity, bool) {
		if !syntax.NamedType(candidate.Type(), "sync", "Mutex") && !syntax.NamedType(candidate.Type(), "sync", "RWMutex") {
			return mutexReceiverIdentity{}, false
		}
		identity := lockIdentityWithin(candidate, budget)
		return mutexReceiverIdentity{value: candidate, identity: identity}, identity != ""
	}
	return ssaflow.ResolveReachingValue(walk, value, leaf, func(receiver mutexReceiverIdentity) string { return receiver.identity })
}

func appendLockValue(values []ssa.Value, candidate ssa.Value, budget *proofs.SearchBudget) []ssa.Value {
	candidateIdentity := lockIdentityWithin(candidate, budget)
	for _, value := range values {
		if !budget.Spend() {
			return values
		}
		if heapmodel.MayAlias(value, candidate) || candidateIdentity != "" && lockIdentityWithin(value, budget) == candidateIdentity {
			return values
		}
	}
	if budget.Exhausted() || budget.PoolExhausted() {
		return values
	}
	return append(values, candidate)
}

// loopVariantValue reports whether the acquisition's receiver is selected by a
// loop iteration: it is defined inside a cycle and derives from a phi or map
// iterator or channel receive in that cycle, as a range element does. This
// is uncertainty, not freshness: a sender could send the same pointer again.
// A field of a receiver or
// a package variable locked inside a loop is the same mutex every time and
// stays reportable.
func loopVariantValue(walk ssaflow.ReachingWalk, value ssa.Value) bool {
	if value == nil || !walk.Mark(value) {
		return false
	}
	switch typed := value.(type) {
	case *ssa.Alloc:
		// An allocation executed again by a loop creates a different lock,
		// even though the allocation has one SSA name.
		return ssaflow.BlockInCycle(typed.Block())
	case *ssa.Phi:
		return ssaflow.BlockInCycle(typed.Block())
	case *ssa.Extract:
		return loopVariantExtract(walk, typed)
	case *ssa.UnOp:
		// Queue consumers can receive a different object each iteration.
		// https://github.com/encodeous/nylon/blob/c4a96c804f7aa08512721dec7994907eab100bc8/polyamide/device/channels.go#L83-L99
		if typed.Op == token.ARROW {
			return ssaflow.BlockInCycle(typed.Block())
		}
		return loopVariantValue(walk, typed.X)
	case *ssa.IndexAddr:
		return loopVariantValue(walk, typed.Index) || loopVariantValue(walk, typed.X)
	case *ssa.Index:
		return loopVariantValue(walk, typed.Index) || loopVariantValue(walk, typed.X)
	case *ssa.FieldAddr:
		return loopVariantValue(walk, typed.X)
	case *ssa.Field:
		return loopVariantValue(walk, typed.X)
	case *ssa.BinOp:
		// A range index is the loop phi plus one.
		return loopVariantValue(walk, typed.X) || loopVariantValue(walk, typed.Y)
	case *ssa.Call:
		// A lookup keyed by the iteration, such as the per-session state a
		// cleanup loop fetches before locking it, selects a different mutex
		// each time; a call with no loop-variant argument returns the same one.
		// CodeKanban locks every session's state this way:
		// https://github.com/fy0/CodeKanban/blob/745699cb67d3d34cec4793168f148eb61e43e766/service/websession/history_cleanup.go#L262-L276
		return slices.ContainsFunc(typed.Common().Args, func(argument ssa.Value) bool {
			return loopVariantValue(walk, argument)
		})
	case *ssa.ChangeType, *ssa.ChangeInterface, *ssa.Convert, *ssa.MakeInterface:
		inner, ok := ssaflow.UnwrapTransparentValue(
			value,
			ssaflow.TransparentChangeInterface|ssaflow.TransparentChangeType|ssaflow.TransparentConvert|ssaflow.TransparentMakeInterface,
		)
		return ok && loopVariantValue(walk, inner)
	}
	return false
}

func loopVariantExtract(walk ssaflow.ReachingWalk, value *ssa.Extract) bool {
	switch tuple := value.Tuple.(type) {
	case *ssa.Next:
		return ssaflow.BlockInCycle(tuple.Block())
	case *ssa.Select:
		// The first two results are the selected case index and receive-ok;
		// only the remaining results are received payloads.
		return value.Index >= 2 && ssaflow.BlockInCycle(tuple.Block())
	default:
		return loopVariantValue(walk, value.Tuple)
	}
}
