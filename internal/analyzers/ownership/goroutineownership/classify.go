package goroutineownership

import (
	"go/token"
	"go/types"
	"slices"

	"github.com/kojah/gohawk/internal/ssaflow"
	"github.com/kojah/gohawk/internal/syntax"

	"golang.org/x/tools/go/ssa"
)

// The classifier labels each instruction of the spawning function with the
// effect it has on the worker's tracked values. A join is an exact observation
// of completion; a transfer hands the exact value to a caller or an owner that
// outlives the function; unknown covers any other consumption the analysis
// cannot see through, such as an opaque call, a send, or a helper that lets
// the value escape. Anything that does not touch a tracked value is none.
// Unknown is deliberately not a weaker join: it suppresses the diagnostic
// instead of proving ownership.

type ownershipAction uint8

const (
	actionNone ownershipAction = iota
	actionJoin
	actionTransfer
	actionUnknown
)

func (action ownershipAction) String() string {
	switch action {
	case actionJoin:
		return "join"
	case actionTransfer:
		return "transfer"
	case actionUnknown:
		return "opaque-use"
	case actionNone:
	}
	return "none"
}

// strongerAction merges the labels of several tracked values touched by one
// instruction. A proven join wins because it already required every-return
// coverage; otherwise any escape keeps the instruction opaque.
func strongerAction(current, next ownershipAction) ownershipAction {
	switch {
	case current == actionJoin || next == actionJoin:
		return actionJoin
	case current == actionUnknown || next == actionUnknown:
		return actionUnknown
	case current == actionTransfer || next == actionTransfer:
		return actionTransfer
	default:
		return actionNone
	}
}

func (analysis *spawnAnalysis) action(instruction ssa.Instruction) ownershipAction {
	if action, ok := analysis.actions[instruction]; ok {
		return action
	}
	action := analysis.classify(instruction)
	analysis.actions[instruction] = action
	return action
}

func (analysis *spawnAnalysis) classify(instruction ssa.Instruction) ownershipAction {
	switch typed := instruction.(type) {
	case *ssa.MakeClosure:
		// Capturing a value has no effect by itself. The closure's defer,
		// return, store, launch, or opaque call is classified where it happens.
		return actionNone
	case *ssa.UnOp, *ssa.Select, *ssa.Range:
		if receivesFrom(instruction, analysis.isSignal) {
			return actionJoin
		}
		if analysis.selectSends(instruction) {
			return actionUnknown
		}
	case *ssa.Store:
		return analysis.storeAction(typed)
	case *ssa.Send:
		if analysis.consumes(typed.X) {
			return actionUnknown
		}
	case *ssa.MapUpdate:
		if analysis.consumes(typed.Value) {
			return actionUnknown
		}
	case *ssa.Call, *ssa.Defer, *ssa.Go:
		return analysis.callAction(instruction, ssaflow.InstructionCall(instruction))
	}
	return actionNone
}

// returnTransfers reports whether a return hands a tracked value, or an
// aggregate or callback containing one, to the caller.
func (analysis *spawnAnalysis) returnTransfers(returned *ssa.Return) bool {
	return slices.ContainsFunc(returned.Results, analysis.consumes)
}

// storeAction transfers the obligation when a tracked value is installed on
// storage that outlives the function. A field of a local aggregate changes
// nothing yet: returning or handing off that aggregate is classified there.
func (analysis *spawnAnalysis) storeAction(store *ssa.Store) ownershipAction {
	if !analysis.consumes(store.Val) {
		return actionNone
	}
	switch address := store.Addr.(type) {
	case *ssa.Global, *ssa.FreeVar:
		return actionTransfer
	case *ssa.FieldAddr:
		if ssaflow.ExternallyOwnedValue(address.X) {
			return actionTransfer
		}
	}
	return actionNone
}

func (analysis *spawnAnalysis) callAction(instruction ssa.Instruction, common *ssa.CallCommon) ownershipAction {
	if common == nil {
		return actionNone
	}
	if builtin, ok := common.Value.(*ssa.Builtin); ok {
		// append retains its arguments in a slice the caller keeps; the other
		// builtins only observe a channel or its capacity.
		if builtin.Name() == "append" && analysis.anyArgumentConsumes(common) {
			return actionUnknown
		}
		return actionNone
	}
	if analysis.callJoinsDirectly(common) {
		return actionJoin
	}
	if analysis.waitGroupBookkeeping(common) {
		return actionNone
	}
	if ssaflow.HasLibraryContract(common, ssaflow.ContractTestingCleanup) {
		return analysis.testingCleanupAction(common)
	}
	if ssaflow.HasLibraryContract(common, ssaflow.ContractGoMockReturn) && analysis.anyArgumentConsumes(common) {
		// gomock.Return publishes these values as the configured result of the
		// mocked call, transferring a produced stream to the code under test.
		// https://github.com/uber-go/mock/blob/539d81c0f42174d17e8f91abcb869bed37605a15/gomock/call.go#L185-L205
		return actionTransfer
	}
	callee, closure := calledFunction(common)
	_, launched := instruction.(*ssa.Go)
	if callee == nil || len(callee.Blocks) == 0 || launched {
		// An opaque callee may retain the value. A launched helper may be a
		// relay or waiter, but it observes completion on its own goroutine, so
		// the parent has not joined anything here either.
		if analysis.anyArgumentConsumes(common) || analysis.closureConsumes(closure) {
			return actionUnknown
		}
		return actionNone
	}
	return analysis.helperAction(common, callee, closure)
}

// callJoinsDirectly recognizes Wait on a settling group and, for the detached
// audit, a lifecycle method on a tracked owner. A deferred call counts because
// it runs on every return.
func (analysis *spawnAnalysis) callJoinsDirectly(common *ssa.CallCommon) bool {
	receiver := ssaflow.CallReceiver(common)
	if receiver == nil {
		return false
	}
	if ssaflow.CallMatchesSymbol(common, waitGroupWait) && ssaflow.SameAsAny(receiver, analysis.groups) {
		return true
	}
	return lifecycleMethod(ssaflow.CallName(common)) && ownerReceiver(receiver, analysis.owners)
}

var waitGroupAdd = syntax.PackageMethod(syntax.MethodSymbol{PackagePath: "sync", Receiver: "WaitGroup", Name: "Add"})

var waitGroupGo = syntax.PackageMethod(syntax.MethodSymbol{PackagePath: "sync", Receiver: "WaitGroup", Name: "Go"})

// waitGroupBookkeeping recognizes the documented sync.WaitGroup counter
// methods on a tracked group. They neither observe completion nor let the
// group escape, so they must not make the group opaque.
func (analysis *spawnAnalysis) waitGroupBookkeeping(common *ssa.CallCommon) bool {
	receiver := ssaflow.CallReceiver(common)
	if receiver == nil || !ssaflow.SameAsAny(receiver, analysis.groups) && !analysis.unsettledGroup(receiver) {
		return false
	}
	return waitGroupMethod(common)
}

func waitGroupMethod(common *ssa.CallCommon) bool {
	return ssaflow.CallMatchesSymbol(common, waitGroupAdd) || ssaflow.CallMatchesSymbol(common, waitGroupDone) ||
		ssaflow.CallMatchesSymbol(common, waitGroupGo) || ssaflow.CallMatchesSymbol(common, waitGroupWait)
}

// unsettledGroup keeps an early-Done WaitGroup out of the opaque set so the
// early Done itself, not the group's bookkeeping, decides the diagnostic.
func (analysis *spawnAnalysis) unsettledGroup(receiver ssa.Value) bool {
	if analysis.unsettledDone == nil {
		return false
	}
	pointer, ok := receiver.Type().Underlying().(*types.Pointer)
	return ok && syntax.NamedType(pointer.Elem(), "sync", "WaitGroup")
}

// helperAction follows every tracked value that the call site supplies to a
// source-visible callee, whether as an argument or a captured variable.
func (analysis *spawnAnalysis) helperAction(common *ssa.CallCommon, callee *ssa.Function, closure *ssa.MakeClosure) ownershipAction {
	result := actionNone
	for _, pair := range suppliedValues(common, callee, closure) {
		for _, tracked := range analysis.tracked {
			if bindingCarries(pair.supplied, tracked.value) {
				result = strongerAction(result, helperUse(callee, pair.local, tracked.kind, map[*ssa.Function]bool{}))
			}
		}
	}
	return result
}

// testingCleanupAction treats a testing Cleanup callback like a deferred
// helper: testing guarantees that it runs after the test completes, so a join
// on every path through the callback settles the worker.
// https://github.com/charmbracelet/crush/blob/6fa9e6905041c32ffceb1c9b1a3189b3db1eec07/internal/server/socket_test.go#L162-L177
func (analysis *spawnAnalysis) testingCleanupAction(common *ssa.CallCommon) ownershipAction {
	result := actionNone
	for _, argument := range common.Args {
		closure, ok := argument.(*ssa.MakeClosure)
		if !ok {
			continue
		}
		if callee, _ := closure.Fn.(*ssa.Function); callee != nil {
			result = strongerAction(result, analysis.helperAction(nil, callee, closure))
		}
	}
	return result
}

// receivesFrom reports whether instruction receives from a channel accepted by
// matches, through a receive expression, a select case, or a channel range.
func receivesFrom(instruction ssa.Instruction, matches func(ssa.Value) bool) bool {
	switch typed := instruction.(type) {
	case *ssa.UnOp:
		return typed.Op == token.ARROW && matches(typed.X)
	case *ssa.Select:
		for _, state := range typed.States {
			if state.Dir == types.RecvOnly && matches(state.Chan) {
				return true
			}
		}
	case *ssa.Range:
		_, channel := typed.X.Type().Underlying().(*types.Chan)
		return channel && matches(typed.X)
	}
	return false
}

// selectSends reports whether a select statement offers a tracked value on a
// send case; like a plain send, the receiver may retain it.
func (analysis *spawnAnalysis) selectSends(instruction ssa.Instruction) bool {
	choice, ok := instruction.(*ssa.Select)
	if !ok {
		return false
	}
	return slices.ContainsFunc(choice.States, func(state *ssa.SelectState) bool {
		return state.Dir == types.SendOnly && analysis.consumes(state.Send)
	})
}

// isSignal matches a received channel against the tracked signals, including
// an element or field selected from a signal aggregate. A receive from any
// element of the slice a signal element was loaded from also counts: element
// addresses are not distinguished by index, and the over-approximation can
// only make a join unproven or accepted, never reported.
func (analysis *spawnAnalysis) isSignal(value ssa.Value) bool {
	if ssaflow.SameAsAny(value, analysis.signals) {
		return true
	}
	root := aggregateRoot(value)
	return root != value && slices.ContainsFunc(analysis.signals, func(signal ssa.Value) bool {
		if !ssaflow.ChannelType(signal) {
			return ssaflow.SameValue(root, signal)
		}
		signalRoot := aggregateRoot(signal)
		return signalRoot != signal && ssaflow.SameValue(root, signalRoot)
	})
}

func (analysis *spawnAnalysis) anyArgumentConsumes(common *ssa.CallCommon) bool {
	return slices.ContainsFunc(common.Args, analysis.consumes)
}

func (analysis *spawnAnalysis) closureConsumes(closure *ssa.MakeClosure) bool {
	if closure == nil {
		return false
	}
	return slices.ContainsFunc(closure.Bindings, func(binding ssa.Value) bool {
		return slices.ContainsFunc(analysis.tracked, func(tracked trackedValue) bool {
			return bindingCarries(binding, tracked.value)
		})
	})
}
