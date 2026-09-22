package concurrencyfacts

import (
	"go/constant"
	"go/token"
	"go/types"

	"github.com/kojah/gohawk/internal/ssaflow"
	"github.com/kojah/gohawk/internal/syntax"
	"golang.org/x/tools/go/ssa"
)

// Completion events retain their exact receiver and execution position. Only
// the standard library's counter contract is recognized, never method names
// on project types. Unknown counter changes cannot establish a waiting cycle.
var (
	groupAdd    = syntax.PackageMethod(syntax.MethodSymbol{PackagePath: "sync", Receiver: "WaitGroup", Name: "Add"})
	groupDone   = syntax.PackageMethod(syntax.MethodSymbol{PackagePath: "sync", Receiver: "WaitGroup", Name: "Done"})
	groupWait   = syntax.PackageMethod(syntax.MethodSymbol{PackagePath: "sync", Receiver: "WaitGroup", Name: "Wait"})
	mutexLock   = syntax.PackageMethod(syntax.MethodSymbol{PackagePath: "sync", Receiver: "Mutex", Name: "Lock"})
	mutexUnlock = syntax.PackageMethod(syntax.MethodSymbol{PackagePath: "sync", Receiver: "Mutex", Name: "Unlock"})
)

func (engine *Engine) callSummary(instruction ssa.CallInstruction) Summary {
	common := instruction.Common()
	var kind Kind
	var resource ssa.Value
	switch {
	case ssaflow.CallMatchesSymbol(common, mutexLock):
		kind, resource = Lock, ssaflow.CallReceiver(common)
	case ssaflow.CallMatchesSymbol(common, mutexUnlock):
		kind, resource = Unlock, ssaflow.CallReceiver(common)
	case ssaflow.CallMatchesSymbol(common, syntax.Builtin("close")):
		kind, resource = Close, common.Args[0]
	case ssaflow.CallMatchesSymbol(common, groupDone):
		kind, resource = GroupDone, ssaflow.CallReceiver(common)
	case ssaflow.CallMatchesSymbol(common, groupWait):
		kind, resource = GroupWait, ssaflow.CallReceiver(common)
	case ssaflow.CallMatchesSymbol(common, groupAdd):
		if len(common.Args) != 2 {
			return Summary{Reason: "protocol-group-count-unknown"}
		}
		count, ok := common.Args[1].(*ssa.Const)
		if !ok || count.Value == nil || !constant.Compare(count.Value, token.EQL, constant.MakeInt64(1)) {
			return Summary{Reason: "protocol-group-count-unknown"}
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

func (engine *Engine) deferCompletion(result *Summary, instruction *ssa.Defer) string {
	called := engine.callSummary(instruction)
	if called.Reason != "" {
		return called.Reason
	}
	if len(called.Operations) != 1 {
		return "protocol-deferred-effects-unknown"
	}
	op := called.Operations[0]
	if op.Kind != Close && op.Kind != GroupDone && op.Kind != Unlock {
		return "protocol-deferred-effects-unknown"
	}
	// Arguments are bound now. Only their execution moves to RunDefers;
	// captured cells still require stable storage through the invocation.
	result.deferred = append(result.deferred, op)
	return ""
}

func synchronizationPointer(value types.Type) bool {
	return waitGroupPointer(value) || MutexPointer(value)
}

// MutexPointer identifies only sync.Mutex pointers, not RWMutex or lookalikes.
func MutexPointer(value types.Type) bool {
	pointer, ok := value.Underlying().(*types.Pointer)
	return ok && syntax.NamedType(pointer.Elem(), "sync", "Mutex")
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
