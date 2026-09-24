package concurrencyfacts

import (
	"go/constant"
	"go/token"
	"go/types"
	"slices"

	"github.com/kojah/gohawk/internal/ssaflow"
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
	common := engine.resolvedCommon(instruction)
	var kind Kind
	var resource ssa.Value
	// Keep RWMutex modes explicit. Sharing a resource identity does not make
	// two readers mutually exclusive or turn RUnlock into an exclusive release.
	switch {
	case ssaflow.CallMatchesSymbol(common, newCond):
		if _, ok := condLocker(instruction); ok {
			return Summary{}
		}
		return Summary{Reason: ReasonCondLockerUnknown}
	case ssaflow.CallMatchesSymbol(common, condWait):
		kind, resource = CondWait, ssaflow.CallReceiver(common)
	case ssaflow.CallMatchesAnySymbol(common, mutexLock, rwLock):
		kind, resource = Lock, ssaflow.CallReceiver(common)
	case ssaflow.CallMatchesAnySymbol(common, mutexUnlock, rwUnlock):
		kind, resource = Unlock, ssaflow.CallReceiver(common)
	case ssaflow.CallMatchesSymbol(common, rwReadLock):
		kind, resource = ReadLock, ssaflow.CallReceiver(common)
	case ssaflow.CallMatchesSymbol(common, rwReadUnlock):
		kind, resource = ReadUnlock, ssaflow.CallReceiver(common)
	case ssaflow.CallMatchesSymbol(common, syntax.Builtin("close")):
		kind, resource = Close, common.Args[0]
	case ssaflow.CallMatchesSymbol(common, groupDone):
		kind, resource = GroupDone, ssaflow.CallReceiver(common)
	case ssaflow.CallMatchesSymbol(common, groupWait):
		kind, resource = GroupWait, ssaflow.CallReceiver(common)
	case ssaflow.CallMatchesSymbol(common, groupAdd):
		if len(common.Args) != 2 {
			return Summary{Reason: ReasonGroupCountUnknown}
		}
		count, ok := common.Args[1].(*ssa.Const)
		if !ok || count.Value == nil || !constant.Compare(count.Value, token.EQL, constant.MakeInt64(1)) {
			return Summary{Reason: ReasonGroupCountUnknown}
		}
		kind, resource = GroupAdd, ssaflow.CallReceiver(common)
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
	_, structure := pointer.Elem().Underlying().(*types.Struct)
	return structure && syntax.NamedType(pointer.Elem(), "sync", "WaitGroup")
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
	// SSA adds a detached recovery return to functions containing defers.
	// It is not an ordinary branch. No user recovery logic or other blocks
	// are admitted, and all deferred calls must themselves be understood.
	if len(function.Blocks) != 2 || function.Recover != function.Blocks[1] || len(function.Blocks[0].Succs) != 0 {
		return false
	}
	recovery := function.Recover
	if len(recovery.Instrs) != 1 {
		return false
	}
	returned, ok := recovery.Instrs[0].(*ssa.Return)
	return ok && len(returned.Results) == 0
}
