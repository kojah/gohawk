package concurrencyfacts

import (
	"go/constant"
	"go/token"
	"go/types"
	"slices"

	proofs "github.com/kojah/gohawk/internal/proof"
	"github.com/kojah/gohawk/internal/ssaflow"
	ssacall "github.com/kojah/gohawk/internal/ssaflow/calls"
	"github.com/kojah/gohawk/internal/syntax"
	"golang.org/x/tools/go/ssa"
)

// Completion events retain their exact receiver and execution position. Only
// the standard library's counter contract is recognized, never method names
// on project types. Unknown counter changes cannot establish a waiting cycle.
var (
	groupAdd     = syntax.PackageMethod(syntax.MethodSymbol{PackagePath: "sync", Receiver: "WaitGroup", Name: "Add"})
	groupDone    = syntax.PackageMethod(syntax.MethodSymbol{PackagePath: "sync", Receiver: "WaitGroup", Name: "Done"})
	groupWait    = syntax.PackageMethod(syntax.MethodSymbol{PackagePath: "sync", Receiver: "WaitGroup", Name: "Wait"})
	mutexLock    = syntax.PackageMethod(syntax.MethodSymbol{PackagePath: "sync", Receiver: "Mutex", Name: "Lock"})
	mutexUnlock  = syntax.PackageMethod(syntax.MethodSymbol{PackagePath: "sync", Receiver: "Mutex", Name: "Unlock"})
	rwLock       = syntax.PackageMethod(syntax.MethodSymbol{PackagePath: "sync", Receiver: "RWMutex", Name: "Lock"})
	rwUnlock     = syntax.PackageMethod(syntax.MethodSymbol{PackagePath: "sync", Receiver: "RWMutex", Name: "Unlock"})
	rwReadLock   = syntax.PackageMethod(syntax.MethodSymbol{PackagePath: "sync", Receiver: "RWMutex", Name: "RLock"})
	rwReadUnlock = syntax.PackageMethod(syntax.MethodSymbol{PackagePath: "sync", Receiver: "RWMutex", Name: "RUnlock"})
)

func (engine *Engine) callSummary(instruction ssa.CallInstruction) Summary {
	if result, handled := engine.cancellationCall(instruction); handled {
		return result
	}
	if hole, ok := callbackHole(instruction); ok {
		return hole
	}
	common := engine.resolvedCommon(instruction)
	var kind Kind
	var resource ssa.Value
	// Keep RWMutex modes explicit. Sharing a resource identity does not make
	// two readers mutually exclusive or turn RUnlock into an exclusive release.
	switch {
	case ssacall.CallMatchesSymbol(common, newCond):
		if _, ok := condLocker(instruction); ok {
			return Summary{}
		}
		return Summary{Reason: ReasonCondLockerUnknown}
	case ssacall.CallMatchesSymbol(common, condWait):
		kind, resource = CondWait, ssaflow.CallReceiver(common)
	case ssacall.CallMatchesAnySymbol(common, mutexLock, rwLock):
		kind, resource = Lock, ssaflow.CallReceiver(common)
	case ssacall.CallMatchesAnySymbol(common, mutexUnlock, rwUnlock):
		kind, resource = Unlock, ssaflow.CallReceiver(common)
	case ssacall.CallMatchesSymbol(common, rwReadLock):
		kind, resource = ReadLock, ssaflow.CallReceiver(common)
	case ssacall.CallMatchesSymbol(common, rwReadUnlock):
		kind, resource = ReadUnlock, ssaflow.CallReceiver(common)
	case ssacall.CallMatchesSymbol(common, syntax.Builtin("close")):
		kind, resource = Close, common.Args[0]
	case ssacall.CallMatchesSymbol(common, groupDone):
		kind, resource = GroupDone, ssaflow.CallReceiver(common)
	case ssacall.CallMatchesSymbol(common, groupWait):
		kind, resource = GroupWait, ssaflow.CallReceiver(common)
	case ssacall.CallMatchesSymbol(common, groupAdd):
		if len(common.Args) != 2 {
			return Summary{Reason: ReasonGroupCountUnknown}
		}
		count, ok := common.Args[1].(*ssa.Const)
		if !ok || count.Value == nil || !constant.Compare(count.Value, token.EQL, constant.MakeInt64(1)) {
			return Summary{Reason: ReasonGroupCountUnknown}
		}
		kind, resource = GroupAdd, ssaflow.CallReceiver(common)
	case inertBuiltin(common), inertAtomic(common):
		return Summary{}
	default:
		return engine.instantiate(instruction)
	}
	var result Summary
	result.Reason = engine.appendOperation(&result, kind, resource, instruction.Pos())
	return result
}

func waitGroupPointer(value types.Type) bool {
	pointer, ok := value.Underlying().(*types.Pointer)
	if !ok {
		return false
	}
	return syntax.PointerStruct(pointer) != nil && syntax.NamedType(pointer.Elem(), "sync", "WaitGroup")
}

func (engine *Engine) deferCompletion(result *Summary, instruction *ssa.Defer) Reason {
	called := engine.callSummary(instruction)
	// A deferred helper can launch another participant. Its synchronous
	// cleanup operations alone are not an exhaustive deferred effect list.
	if len(called.Workers) != 0 {
		return ReasonDeferredEffectsUnknown
	}
	if !composableLinear(called) {
		return called.Reason
	}
	if len(called.Operations) == 0 {
		return ReasonDeferredEffectsUnknown
	}
	for _, op := range called.Operations {
		if op.Kind != Close && op.Kind != GroupDone && op.Kind != Unlock && op.Kind != ReadUnlock && op.Kind != Cancel {
			return ReasonDeferredEffectsUnknown
		}
	}
	result.CancellationInputs = append(result.CancellationInputs, called.CancellationInputs...)
	// Arguments are bound now. Only their execution moves to RunDefers;
	// captured cells still require stable storage through the invocation.
	// RunDefers reverses this stack, so push the helper backwards to retain
	// its internal execution order while reversing the order of deferred calls.
	for _, op := range slices.Backward(called.Operations) {
		result.deferred = append(result.deferred, op)
	}
	return ReasonNone
}

func synchronizationPointer(value types.Type) bool {
	return waitGroupPointer(value) || MutexPointer(value) || condPointer(value)
}

// MutexPointer identifies sync.Mutex and sync.RWMutex pointers, not lookalikes.
func MutexPointer(value types.Type) bool {
	pointer, ok := value.Underlying().(*types.Pointer)
	return ok && (syntax.NamedType(pointer.Elem(), "sync", "Mutex") || syntax.NamedType(pointer.Elem(), "sync", "RWMutex"))
}

func straightLineBody(function *ssa.Function) bool {
	if len(function.Blocks) == 1 {
		return true
	}
	// A function with defers also has a detached recovery block; see
	// detachedRecovery for why it is not an ordinary branch.
	return len(function.Blocks) == 2 && function.Recover == function.Blocks[1] &&
		len(function.Blocks[0].Succs) == 0 && detachedRecovery(function)
}

// Conditions retain their own identity in summaries. The associated locker
// can only be recovered from an exact NewCond call, never a mutable L field.

var (
	newCond  = syntax.PackageFunction("sync", "NewCond")
	condWait = syntax.PackageMethod(syntax.MethodSymbol{PackagePath: "sync", Receiver: "Cond", Name: "Wait"})
)

func condPointer(value types.Type) bool {
	pointer, ok := value.Underlying().(*types.Pointer)
	return ok && syntax.NamedType(pointer.Elem(), "sync", "Cond")
}

func condLocker(call ssa.CallInstruction) (ssa.Value, bool) {
	if !ssacall.CallMatchesSymbol(call.Common(), newCond) || len(call.Common().Args) != 1 {
		return nil, false
	}
	wrapped, ok := call.Common().Args[0].(*ssa.MakeInterface)
	if !ok || !MutexPointer(wrapped.X.Type()) {
		return nil, false
	}
	return wrapped.X, true
}

// CondMutex resolves the exact Mutex supplied to a NewCond allocation. It does
// not prove that L remains unchanged: callers need a complete root summary,
// which rejects condition-field access, mutation and opaque publication.
func (engine *Engine) CondMutex(reference Reference, budget *proofs.SearchBudget) (Reference, bool) {
	engine.mu.Lock()
	defer engine.mu.Unlock()
	call, ok := reference.Value.(*ssa.Call)
	if !ok || reference.Indirect {
		return Reference{}, false
	}
	locker, ok := condLocker(call)
	if !ok {
		return Reference{}, false
	}
	return engine.query(budget).reference(locker)
}
