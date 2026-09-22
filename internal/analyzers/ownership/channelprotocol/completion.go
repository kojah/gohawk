package channelprotocol

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
	groupAdd  = syntax.PackageMethod(syntax.MethodSymbol{PackagePath: "sync", Receiver: "WaitGroup", Name: "Add"})
	groupDone = syntax.PackageMethod(syntax.MethodSymbol{PackagePath: "sync", Receiver: "WaitGroup", Name: "Done"})
	groupWait = syntax.PackageMethod(syntax.MethodSymbol{PackagePath: "sync", Receiver: "WaitGroup", Name: "Wait"})
)

func (engine *summaryEngine) callSummary(instruction ssa.CallInstruction) summary {
	common := instruction.Common()
	var kind operationKind
	var resource ssa.Value
	switch {
	case ssaflow.CallMatchesSymbol(common, syntax.Builtin("close")):
		kind, resource = closeOperation, common.Args[0]
	case ssaflow.CallMatchesSymbol(common, groupDone):
		kind, resource = groupDoneOperation, ssaflow.CallReceiver(common)
	case ssaflow.CallMatchesSymbol(common, groupWait):
		kind, resource = groupWaitOperation, ssaflow.CallReceiver(common)
	case ssaflow.CallMatchesSymbol(common, groupAdd):
		if len(common.Args) != 2 {
			return summary{reason: "protocol-group-count-unknown"}
		}
		count, ok := common.Args[1].(*ssa.Const)
		if !ok || count.Value == nil || !constant.Compare(count.Value, token.EQL, constant.MakeInt64(1)) {
			return summary{reason: "protocol-group-count-unknown"}
		}
		kind, resource = groupAddOperation, ssaflow.CallReceiver(common)
	default:
		return engine.instantiate(instruction)
	}
	var result summary
	result.reason = engine.appendOperation(&result, kind, resource, instruction.Pos())
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

func (engine *summaryEngine) deferCompletion(result *summary, instruction *ssa.Defer) string {
	called := engine.callSummary(instruction)
	if called.reason != "" {
		return called.reason
	}
	if len(called.operations) != 1 {
		return "protocol-deferred-effects-unknown"
	}
	op := called.operations[0]
	if op.kind != closeOperation && op.kind != groupDoneOperation {
		return "protocol-deferred-effects-unknown"
	}
	// Arguments are bound now. Only their execution moves to RunDefers;
	// captured cells still require stable storage through the invocation.
	result.deferred = append(result.deferred, op)
	return ""
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
