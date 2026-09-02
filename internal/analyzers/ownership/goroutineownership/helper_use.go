package goroutineownership

import (
	"slices"

	"github.com/kojah/gohawk/internal/ssaflow"

	"golang.org/x/tools/go/ssa"
)

// Helper-use evidence follows one tracked value into a source-visible callee
// through the exact parameter or captured variable that carries it, and
// reports what the callee does with it. Only static call chains are followed;
// a dynamic callee, a launched goroutine, or an invoked callback derived from
// the value ends the proof as an escape.

// suppliedValue pairs a callee-local value with the value the call site
// supplies for it.
type suppliedValue struct {
	local    ssa.Value
	supplied ssa.Value
}

// suppliedValues lists the parameters and captured variables of callee together
// with the argument or binding supplied at the call. common may be nil for a
// callback that is only created, not called, at this instruction.
func suppliedValues(common *ssa.CallCommon, callee *ssa.Function, closure *ssa.MakeClosure) []suppliedValue {
	var pairs []suppliedValue
	if common != nil {
		for index, argument := range common.Args {
			if index < len(callee.Params) {
				pairs = append(pairs, suppliedValue{local: callee.Params[index], supplied: argument})
			}
		}
	}
	if closure != nil {
		for index, free := range callee.FreeVars {
			if index < len(closure.Bindings) {
				pairs = append(pairs, suppliedValue{local: free, supplied: closure.Bindings[index]})
			}
		}
	}
	return pairs
}

func calledFunction(common *ssa.CallCommon) (*ssa.Function, *ssa.MakeClosure) {
	closure, _ := common.Value.(*ssa.MakeClosure)
	function := common.StaticCallee()
	if closure != nil {
		function, _ = closure.Fn.(*ssa.Function)
	}
	// Generic calls may point at an instantiated wrapper whose body does not
	// expose the receive. The origin has the same parameter positions and the
	// source body needed to prove that the helper joins the signal.
	if function != nil && function.Origin() != nil {
		function = function.Origin()
	}
	return function, closure
}

// helperUse classifies how a callee treats one parameter or captured variable.
// The callee joins only when its observation of the value covers every normal
// return, which is what makes a deferred helper or cleanup callback
// equivalent to an inline join. Storing, sending, returning, capturing, or
// passing the value to an opaque call ends the proof as unknown. A callee that
// merely reads the value, or joins it on some paths, proves nothing.
func helperUse(function *ssa.Function, local ssa.Value, kind trackedKind, seen map[*ssa.Function]bool) ownershipAction {
	if function == nil || seen[function] || len(function.Blocks) == 0 {
		return actionNone
	}
	seen[function] = true
	defer delete(seen, function)
	derives := func(value ssa.Value) bool {
		return ssaflow.ValueDerivesFrom(value, local, map[ssa.Value]bool{})
	}
	joins := func(instruction ssa.Instruction) bool {
		return instructionJoins(instruction, kind, derives, seen)
	}
	joined, escaped := false, false
	for _, block := range function.Blocks {
		for _, instruction := range block.Instrs {
			joined = joined || joins(instruction)
			escaped = escaped || instructionEscapes(instruction, local, kind, derives, seen)
		}
	}
	if joined && !ssaflow.UnownedReturnFromEntry(function, joins) {
		return actionJoin
	}
	if escaped {
		return actionUnknown
	}
	return actionNone
}

func instructionJoins(instruction ssa.Instruction, kind trackedKind, derives func(ssa.Value) bool, seen map[*ssa.Function]bool) bool {
	if kind == trackedSignal && receivesFrom(instruction, derives) {
		return true
	}
	common := ssaflow.InstructionCall(instruction)
	if common == nil {
		return false
	}
	if receiverJoins(common, kind, derives) {
		return true
	}
	callee, closure := calledFunction(common)
	if _, launched := instruction.(*ssa.Go); launched || callee == nil {
		return false
	}
	return slices.ContainsFunc(suppliedValues(common, callee, closure), func(pair suppliedValue) bool {
		return derives(pair.supplied) && helperUse(callee, pair.local, kind, seen) == actionJoin
	})
}

// receiverJoins recognizes the receiver-side join for each tracked kind: Wait
// on a group, or a lifecycle method on an owner.
func receiverJoins(common *ssa.CallCommon, kind trackedKind, derives func(ssa.Value) bool) bool {
	receiver := ssaflow.CallReceiver(common)
	if receiver == nil || !derives(receiver) {
		return false
	}
	switch kind {
	case trackedGroup:
		return ssaflow.CallMatchesSymbol(common, waitGroupWait)
	case trackedOwner:
		return lifecycleMethod(ssaflow.CallName(common))
	case trackedSignal:
	}
	return false
}

func instructionEscapes(
	instruction ssa.Instruction,
	local ssa.Value,
	kind trackedKind,
	derives func(ssa.Value) bool,
	seen map[*ssa.Function]bool,
) bool {
	switch typed := instruction.(type) {
	case *ssa.Store:
		return derives(typed.Val)
	case *ssa.Send:
		return derives(typed.X)
	case *ssa.MapUpdate:
		return derives(typed.Value)
	case *ssa.MakeClosure:
		return slices.ContainsFunc(typed.Bindings, derives)
	case *ssa.Return:
		return ssaflow.ReturnedValueOwnsValue(typed, local)
	case *ssa.Call, *ssa.Defer, *ssa.Go:
		return callEscapes(instruction, kind, derives, seen)
	default:
		return false
	}
}

func callEscapes(instruction ssa.Instruction, kind trackedKind, derives func(ssa.Value) bool, seen map[*ssa.Function]bool) bool {
	common := ssaflow.InstructionCall(instruction)
	if builtin, ok := common.Value.(*ssa.Builtin); ok {
		return builtin.Name() == "append" && slices.ContainsFunc(common.Args, derives)
	}
	if derives(common.Value) {
		// Invoking a callback derived from the value runs code this proof does
		// not follow.
		return true
	}
	if receiverCallRetainsNothing(common, kind, derives) {
		return false
	}
	callee, closure := calledFunction(common)
	_, launched := instruction.(*ssa.Go)
	if launched || callee == nil || len(callee.Blocks) == 0 {
		return slices.ContainsFunc(common.Args, derives) || closure != nil && slices.ContainsFunc(closure.Bindings, derives)
	}
	return slices.ContainsFunc(suppliedValues(common, callee, closure), func(pair suppliedValue) bool {
		return derives(pair.supplied) && helperUse(callee, pair.local, kind, seen) == actionUnknown
	})
}

// receiverCallRetainsNothing recognizes the documented sync.WaitGroup methods
// and, for the detached audit, a lifecycle method: they observe or settle the
// receiver without letting it escape.
func receiverCallRetainsNothing(common *ssa.CallCommon, kind trackedKind, derives func(ssa.Value) bool) bool {
	receiver := ssaflow.CallReceiver(common)
	if receiver == nil || !derives(receiver) {
		return false
	}
	switch kind {
	case trackedGroup:
		return waitGroupMethod(common)
	case trackedOwner:
		return lifecycleMethod(ssaflow.CallName(common))
	case trackedSignal:
	}
	return false
}
