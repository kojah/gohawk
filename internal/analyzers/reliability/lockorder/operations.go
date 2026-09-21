package lockorder

import (
	"go/token"
	"slices"

	"github.com/kojah/gohawk/internal/check"
	"github.com/kojah/gohawk/internal/ssaflow"
	"github.com/kojah/gohawk/internal/syntax"
	analysisTrace "github.com/kojah/gohawk/internal/trace"

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

func acquireLock(
	pass *analysis.Pass,
	instruction ssa.Instruction,
	held []string,
	identity string,
	keys map[string]string,
	relations map[lockRelation]token.Pos,
	releaseUnproven bool,
) []string {
	if slices.Contains(held, identity) {
		// A lock selected by a loop iteration is a different mutex each time
		// the same instruction runs, so re-acquiring its identity on the next
		// iteration is not recursion. multigres locks every key mutex in a
		// loop and defers the unlocks:
		// https://github.com/multigres/multigres/blob/360b8f123dff8ad6bcc721acaec103c52081bebd/go/tools/viperutil/internal/sync/sync.go#L236-L240
		// A lock a callee may already have released is not proven held, and a
		// recursive acquisition has to claim that it is.
		if !loopVariantLock(instruction) && !releaseUnproven {
			check.Reportf(pass, check.LockRecursiveAcquire, instruction.Pos(), "lock %s is acquired while already held", identity)
		}
		return held
	}
	// recursive-acquire above stays on instance identity, where re-locking the
	// same object is the defect. Ordering below compares lock classes, so a
	// mutex held in a struct field is comparable across the methods that take
	// it; see lockClassOf for why the class claim is sound.
	for _, owner := range held {
		recordOrder(pass, instruction.Pos(), relations, keys[owner], keys[identity])
	}
	return append(held, identity)
}

// recordOrder records that ownerKey was held while key was acquired, and
// reports when the opposite order was already seen somewhere in this package.
// Both the direct acquisition and the order a call implies come through here,
// so one place decides what an ordering claim requires.
func recordOrder(pass *analysis.Pass, position token.Pos, relations map[lockRelation]token.Pos, ownerKey, key string) {
	if ownerKey == "" || key == "" || ownerKey == key {
		// An unclassified lock compares with nothing, and two locks of one
		// class are ordered by which object holds them -- evidence this
		// analysis does not have. Reporting the latter would flag every routine
		// that locks two peers at once, such as transfer(from, to *Account)
		// taking from.mu then to.mu while another caller passes the same
		// accounts the other way around.
		return
	}
	relation := lockRelation{from: ownerKey, to: key}
	if _, recorded := relations[relation]; recorded {
		return
	}
	if opposite, exists := relations[lockRelation{from: key, to: ownerKey}]; exists {
		// Preserve the other half of the cycle in candidate-scoped evidence.
		// The diagnostic's own position otherwise identifies only one order,
		// making an interprocedural cycle impossible to audit from the trace.
		probe := analysisTrace.For(pass, "lockorder", string(check.LockContradictoryOrder), position)
		if probe.Enabled() {
			probe.Evidence(analysisTrace.Step{
				Reason: "opposite-order-recorded", Outcome: analysisTrace.OutcomeRejected, Pos: opposite,
				Details: map[string]string{"held": key, "acquired": ownerKey},
			})
		}
		check.Reportf(pass, check.LockContradictoryOrder, position, "contradictory lock order: %s and %s", key, ownerKey)
	}
	relations[relation] = position
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
			// Returning the object containing a held mutex exposes its release to
			// the caller. This is unknown ownership, not proof that any method
			// named Unlock releases it. Kube-vip returns such an owner on success:
			// https://github.com/kube-vip/kube-vip/blob/be536eaaf73c80fa5161e757ac18b472498f986e/pkg/iptables/lock.go#L54-L67
			if ssaflow.NewReachingWalk(mutexForms).Any(result, func(_ ssaflow.ReachingWalk, owner ssa.Value) bool {
				return ssaflow.ValueIsAccessPathFrom(value, owner)
			}) {
				return true
			}
		}
	}
	return false
}

// optionalLoadedMutex identifies an acquisition whose guard tests mutable
// storage rather than one SSA value. Repeated reads of that slot have unrelated
// condition identities, while lock identity deliberately names the slot. Until
// that relationship is proved, an unreleased path may be infeasible. Decline
// missing-release, including changed guards, rather than assume either stable
// contents or a leak; direct mutex parameters and Boolean guards stay precise.
// https://github.com/devld/go-drive/blob/91c3ac7253642bf58629d6a87cf7ab718f2d3827/common/utils/path_tree.go#L133-L141
func optionalLoadedMutex(instruction ssa.Instruction, identity string) bool {
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
	comparison, ok := branch.Cond.(*ssa.BinOp)
	if !ok || (comparison.Op != token.EQL && comparison.Op != token.NEQ) {
		return false
	}
	for _, pair := range [][2]ssa.Value{{comparison.X, comparison.Y}, {comparison.Y, comparison.X}} {
		loaded, ok := pair[0].(*ssa.UnOp)
		if ok && loaded.Op == token.MUL && ssaflow.DefinitelyNil(pair[1]) && lockIdentityOf(loaded) == identity {
			return true
		}
	}
	return false
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

// reportMismatchedRelease reports a release whose method does not match the way
// the lock was acquired. sync.RWMutex keeps separate reader and writer state,
// so RUnlock on a write-held mutex and Unlock on a read-held one are both
// fatal at run time rather than merely wrong: the runtime reports an unlock of
// an unlocked mutex and stops the process.
//
// The acquisition has to be visible on this path for the mode to be known. A
// helper that releases a lock its caller took holds no acquisition here, and
// the flow already treats that as a borrowed lock rather than an error.
func reportMismatchedRelease(pass *analysis.Pass, instruction ssa.Instruction, identity string, state lockFlowState) {
	if !slices.Contains(state.held, identity) {
		return
	}
	acquiredForReading := slices.Contains(state.readHeld, identity)
	if acquiredForReading == readModeRelease(instruction) {
		return
	}
	acquire, release := "Lock", "RUnlock"
	if acquiredForReading {
		acquire, release = "RLock", "Unlock"
	}
	check.Reportf(pass, check.LockMismatchedRelease, instruction.Pos(),
		"lock %s is acquired with %s and released with %s", identity, acquire, release)
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
	receiver = concreteMutexReceiver(receiver)
	if receiver == nil {
		return 0, "", nil, false
	}
	identity := lockIdentityOf(receiver)
	return operation, identity, receiver, identity != ""
}

// concreteMutexReceiver unwraps interface values only when every possible SSA
// origin proves the same concrete sync mutex identity.
func concreteMutexReceiver(value ssa.Value) ssa.Value { //nolint:ireturn // SSA values have several concrete forms.
	walk := ssaflow.NewReachingWalk(ssaflow.TransparentChangeInterface | ssaflow.TransparentMakeInterface)
	receiver, ok := ssaflow.ResolveReachingValue(walk, value, concreteMutexLeaf, lockIdentityOf)
	if !ok {
		return nil
	}
	return receiver
}

func concreteMutexLeaf(_ ssaflow.ReachingWalk, value ssa.Value) (ssa.Value, bool) { //nolint:ireturn // SSA values have several concrete forms.
	if !syntax.NamedType(value.Type(), "sync", "Mutex") && !syntax.NamedType(value.Type(), "sync", "RWMutex") {
		return nil, false
	}
	return value, lockIdentityOf(value) != ""
}

func appendLockValue(values []ssa.Value, candidate ssa.Value) []ssa.Value {
	candidateIdentity := lockIdentityOf(candidate)
	for _, value := range values {
		if ssaflow.SameValue(value, candidate) || candidateIdentity != "" && lockIdentityOf(value) == candidateIdentity {
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
	return receiver != nil && loopVariantValue(ssaflow.NewReachingWalk(ssaflow.TransparentNone), receiver)
}

func loopVariantValue(walk ssaflow.ReachingWalk, value ssa.Value) bool {
	if value == nil || !walk.Mark(value) {
		return false
	}
	switch typed := value.(type) {
	case *ssa.Phi:
		return ssaflow.BlockInCycle(typed.Block())
	case *ssa.Extract:
		if next, ok := typed.Tuple.(*ssa.Next); ok {
			return ssaflow.BlockInCycle(next.Block())
		}
		return loopVariantValue(walk, typed.Tuple)
	case *ssa.UnOp:
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
