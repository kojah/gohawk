package lockorder

import (
	"go/token"
	"slices"

	"github.com/kojah/gohawk/internal/check"
	"github.com/kojah/gohawk/internal/heapmodel"
	"github.com/kojah/gohawk/internal/lifecycle"
	"github.com/kojah/gohawk/internal/ssaflow"
	"github.com/kojah/gohawk/internal/syntax"

	analysisTrace "github.com/kojah/gohawk/internal/trace"
	"golang.org/x/tools/go/ssa"
)

type conditionalCallerSet struct {
	calls   []*ssa.Call
	escaped bool
}

type callerReleaseProof struct {
	proven bool
	reason lockReason
}

// A lock is reported only when some path releases it and another returns with
// it held. Without a witnessed release, ownership may belong to a caller.
// Explicit successful-return and conditional caller-release contracts likewise
// distinguish a transferred critical section from an abandoned acquisition.
func (flow lockFlowContext) reportMissingReleases(
	function *ssa.Function, unreleased map[string][]token.Pos,
	heldAt map[string]map[*ssa.Return]bool, callers conditionalCallerSet,
) {
	for identity, returns := range unreleased {
		position := flow.acquiredAt[identity]
		if flow.uncertainGuards[identity] {
			analysisTrace.For(flow.pass, "lockorder", string(check.LockMissingRelease), position).Decision(analysisTrace.Step{
				Reason: lockReasonLoadedAcquisitionGuardUnknown.String(), Outcome: analysisTrace.OutcomeUnknown, Pos: position,
			})
			continue
		}
		values := flow.lockValues[identity]
		if slices.ContainsFunc(values, privateMutexOnly) || !flow.released[identity] ||
			acquiresForCaller(function, flow.acquisitions[identity], heldAt[identity]) {
			continue
		}
		if proof := conditionalCallerRelease(function, values, heldAt[identity], callers); proof.proven {
			traceCallerRelease(flow.pass, position, proof.reason)
			continue
		}
		for _, returned := range returns {
			if returned == token.NoPos {
				returned = position
			}
			check.Reportf(flow.pass, check.LockMissingRelease, returned, "lock %s is not released on this return path", identity)
		}
	}
}

// callerSetBudget bounds the one walk over every instruction of the
// package that collects callers of private function values. It is a
// whole-package pass rather than a question about one candidate, so it is
// far larger than a query; exhaustion leaves every caller set incomplete,
// which no proof may then rely on.
const callerSetBudget = 20_000

// Private function values must have a complete, bounded synchronous caller set.
// Include generated source bodies when collecting uses; skipping their callers
// would turn an incomplete set into a cleanup guarantee.
func conditionalCallerSets(functions []*ssa.Function) map[*ssa.Function]conditionalCallerSet {
	callers := make(map[*ssa.Function]conditionalCallerSet)
	budget := ssaflow.NewSearchBudget(callerSetBudget)
	for _, function := range functions {
		if function == nil {
			continue
		}
		for _, block := range function.Blocks {
			for _, instruction := range block.Instrs {
				if !budget.Spend() {
					return nil
				}
				if _, debug := instruction.(*ssa.DebugRef); debug {
					continue
				}
				for _, operand := range instruction.Operands(nil) {
					if operand == nil {
						continue
					}
					callee, ok := (*operand).(*ssa.Function)
					if !ok || callee.Object() == nil || callee.Object().Exported() || callee.Signature.Recv() != nil {
						continue
					}
					entry := callers[callee]
					call, synchronous := instruction.(*ssa.Call)
					if synchronous && operand == &call.Common().Value && len(entry.calls) < 32 {
						entry.calls = append(entry.calls, call)
					} else {
						entry.escaped = true
					}
					callers[callee] = entry
				}
			}
		}
	}
	return callers
}

// A Boolean result can mean "already unlocked", not necessarily success.
// Infer no naming convention: require every normal return to expose one exact
// held-state polarity and every known caller to release the same global mutex
// on that polarity. Opaque/escaping callers leave this contract unknown.
// https://github.com/fortio/fortio/blob/5c19725ff61c9f7ad944b91ec32d96a399341d87/fnet/network.go#L328-L393
func conditionalCallerRelease(
	function *ssa.Function, values []ssa.Value, heldAt map[*ssa.Return]bool, callers conditionalCallerSet,
) callerReleaseProof {
	unknown := callerReleaseProof{reason: lockReasonConditionalCallerReleaseUnknown}
	if len(values) != 1 || callers.escaped || len(callers.calls) == 0 {
		return unknown
	}
	global, ok := values[0].(*ssa.Global)
	if !ok || !syntax.NamedType(global.Type(), "sync", "Mutex") {
		return unknown
	}
	for index := range function.Signature.Results().Len() {
		held, known := heldResultPolarity(function, heldAt, index)
		if known && !slices.ContainsFunc(callers.calls, func(call *ssa.Call) bool {
			return !callerReleasesOnFlag(call, global, index, held)
		}) {
			return callerReleaseProof{proven: true, reason: lockReasonConditionalCallerReleaseProven}
		}
	}
	return unknown
}

func heldResultPolarity(function *ssa.Function, heldAt map[*ssa.Return]bool, index int) (bool, bool) {
	var held, unheld, sawHeld, sawUnheld bool
	for _, returned := range ssaflow.InstructionsOf[*ssa.Return](function) {
		truth, known := lockBooleanValue(lifecycle.ReturnedResult(returned, index), nil)
		if !known {
			return false, false
		}
		if heldAt[returned] {
			if sawHeld && held != truth {
				return false, false
			}
			held, sawHeld = truth, true
		} else {
			if sawUnheld && unheld != truth {
				return false, false
			}
			unheld, sawUnheld = truth, true
		}
	}
	return held, sawHeld && sawUnheld && held != unheld
}

func callerReleasesOnFlag(call *ssa.Call, mutex *ssa.Global, index int, held bool) bool {
	block := call.Block()
	if ssaflow.BlockInCycle(block) || len(block.Succs) != 2 {
		return false
	}
	branch, ok := block.Instrs[len(block.Instrs)-1].(*ssa.If)
	if call.Common().Signature().Results().Len() == 1 {
		index = -1
	}
	if !ok || branch.Cond != ssaflow.CallResult(call, index) {
		return false
	}
	unheld := block.Succs[0]
	if held {
		unheld = block.Succs[1]
	}
	if len(unheld.Preds) != 1 {
		return false
	}
	witness := false
	unowned := ssaflow.UnownedReturn(call, func(instruction ssa.Instruction) bool {
		if instruction.Block() == unheld {
			return true
		}
		operation, _, receiver, direct := mutexAction(instruction)
		if direct && operation == mutexRelease && receiver == mutex && !readModeRelease(instruction) {
			witness = true
			return true
		}
		return false
	}, nil)
	return witness && !unowned
}

func callerOwnedLocks(function *ssa.Function, summaries map[ssa.Instruction][]mutexEffect) map[string]bool {
	type firstAction struct {
		operation mutexOperation
		position  token.Pos
	}
	first := map[string]firstAction{}
	for _, block := range function.Blocks {
		for _, instruction := range block.Instrs {
			effects := summaries[instruction]
			if effect, ok := directMutexEffect(instruction); ok {
				effects = []mutexEffect{effect}
			}
			for _, effect := range effects {
				current, exists := first[effect.identity]
				if instruction.Pos() != token.NoPos && (!exists || instruction.Pos() < current.position) {
					first[effect.identity] = firstAction{operation: effect.operation, position: instruction.Pos()}
				}
			}
		}
	}
	result := map[string]bool{}
	for identity, action := range first {
		result[identity] = action.operation == mutexRelease
	}
	return result
}

func (flow lockFlowContext) acquireLock(
	instruction ssa.Instruction,
	held []string,
	identity string,
	releaseUnproven bool,
	variant bool,
) []string {
	if slices.Contains(held, identity) {
		// A lock selected by a loop iteration may be a different mutex each
		// time the same instruction runs, so its repeated SSA identity does
		// not prove recursion. multigres locks every key mutex in a
		// loop and defers the unlocks:
		// https://github.com/multigres/multigres/blob/360b8f123dff8ad6bcc721acaec103c52081bebd/go/tools/viperutil/internal/sync/sync.go#L236-L240
		// A lock a callee may already have released is not proven held, and a
		// recursive acquisition has to claim that it is.
		if !variant && !releaseUnproven {
			site := lockReportSite{instruction: instruction, identity: identity}
			// Distinct feasible proof states can witness the same reacquisition.
			// Retain those states, but report their shared acquisition only once.
			if !flow.recursiveReports[site] {
				flow.recursiveReports[site] = true
				check.Reportf(flow.pass, check.LockRecursiveAcquire, instruction.Pos(), "lock %s is acquired while already held", identity)
			}
		}
		return held
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
			if lifecycle.ValueCallsMethod(result, "Unlock", value) || lifecycle.ValueCallsMethod(result, "RUnlock", value) {
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

// optionalLoadedGuard identifies an acquisition whose guard tests mutable
// storage rather than one SSA value. Repeated reads of that slot have unrelated
// condition identities, while lock identity deliberately names the slot. Until
// that relationship is proved, an unreleased path may be infeasible. Decline
// missing-release, including changed guards, rather than assume either stable
// contents or a leak; direct mutex parameters and Boolean guards stay precise.
// https://github.com/devld/go-drive/blob/91c3ac7253642bf58629d6a87cf7ab718f2d3827/common/utils/path_tree.go#L133-L141
func optionalLoadedGuard(instruction ssa.Instruction, identity string) bool {
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
		if heapmodel.MayAlias(value, candidate) || candidateIdentity != "" && lockIdentityOf(value) == candidateIdentity {
			return values
		}
	}
	return append(values, candidate)
}

// loopVariantLock reports whether the acquisition's receiver is selected by a
// loop iteration: it is defined inside a cycle and derives from a phi or map
// iterator or channel receive in that cycle, as a range element does. This
// is uncertainty, not freshness: a sender could send the same pointer again.
// A field of a receiver or
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
