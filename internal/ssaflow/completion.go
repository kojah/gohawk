package ssaflow

import (
	"slices"

	"golang.org/x/tools/go/ssa"
)

// Completion evidence proves that a callee invokes a lifecycle method on the
// caller's target. Every variant is the same search over the callee's body:
// a receiver mapping decides which callee-local receiver stands for the
// caller's target, and a coverage kind decides how much of the callee's
// control flow the call must dominate. Modes differ only in those two choices
// plus the launch form (deferred, called, or started) they accept.

// CompletionCoverage selects how much of a callee's control flow a lifecycle
// call must cover before it counts as completion.
type CompletionCoverage uint8

const (
	// CoverageAnywhere accepts a call on any path; a deferred callee already
	// runs on every return of its parent, so this settles a deferred closure.
	CoverageAnywhere CompletionCoverage = iota
	// CoverageBeforeBranch accepts a call on the unconditional path from the
	// callee's entry block.
	CoverageBeforeBranch
	// CoverageEveryReturn accepts a call that precedes every normal return; a
	// callee that never returns or never calls proves nothing.
	CoverageEveryReturn
)

// MethodCallCoverage reports whether calls holds over function's normal paths
// with the requested coverage. nonNil, when set, restricts every-return
// analysis to paths feasible when that value is non-nil at entry.
func MethodCallCoverage(function *ssa.Function, calls func(ssa.Instruction) bool, coverage CompletionCoverage, nonNil ssa.Value) bool {
	if function == nil || len(function.Blocks) == 0 {
		return false
	}
	switch coverage {
	case CoverageBeforeBranch:
		visited := map[*ssa.BasicBlock]bool{}
		for block := function.Blocks[0]; block != nil && !visited[block]; {
			visited[block] = true
			if slices.ContainsFunc(block.Instrs, calls) {
				return true
			}
			if len(block.Succs) != 1 {
				return false
			}
			block = block.Succs[0]
		}
		return false
	case CoverageEveryReturn:
		hasReturn, hasCall := false, false
		for _, block := range function.Blocks {
			for _, candidate := range block.Instrs {
				if _, ok := candidate.(*ssa.Return); ok {
					hasReturn = true
				}
				hasCall = hasCall || calls(candidate)
			}
		}
		return hasReturn && hasCall && !unownedReturnFromEntry(function, calls, nil, nonNil)
	case CoverageAnywhere:
	}
	return slices.ContainsFunc(function.Blocks, func(block *ssa.BasicBlock) bool {
		return slices.ContainsFunc(block.Instrs, calls)
	})
}

// mappedCompletion is the shared search for a call-like instruction whose
// callee's receivers are mapped back to target through closure bindings and
// call arguments. Path-sensitive coverage also accepts a nested deferred
// closure on the same mapping, because that defer runs when the callee
// returns; anywhere coverage does not, since a conditionally registered defer
// would otherwise settle a started worker on a path that never registers it.
func mappedCompletion(instruction ssa.Instruction, method string, target ssa.Value, coverage CompletionCoverage) bool {
	common, closure, function := calledFunction(instruction)
	if function == nil {
		return false
	}
	calls := func(candidate ssa.Instruction) bool {
		called := InstructionCall(candidate)
		if CallName(called) == method && calledReceiverMatches(common, closure, function, CallReceiver(called), target) {
			return true
		}
		return coverage != CoverageAnywhere && closureDefersMethodOnMappedTarget(candidate, method, common, closure, function, target)
	}
	return MethodCallCoverage(function, calls, coverage, nil)
}

// ClosureCallsMethod reports whether a call-like closure calls method on target
// on any path. It maps both captured free variables and explicit closure
// parameters back to the values supplied by the enclosing function.
func ClosureCallsMethod(instruction ssa.Instruction, method string, target ssa.Value) bool {
	return mappedCompletion(instruction, method, target, CoverageAnywhere)
}

// DeferredClosureCallsMethodOnDerivedArgumentOnEveryReturn reports whether a
// deferred function literal receives a value projected from target and calls
// method on that exact parameter along every normal return path.
func DeferredClosureCallsMethodOnDerivedArgumentOnEveryReturn(instruction ssa.Instruction, method string, target ssa.Value) bool {
	if _, ok := instruction.(*ssa.Defer); !ok {
		return false
	}
	common, _, function := calledFunction(instruction)
	if common == nil || function == nil || function.Parent() == nil {
		return false
	}
	calls := func(candidate ssa.Instruction) bool {
		called := InstructionCall(candidate)
		if CallName(called) != method {
			return false
		}
		receiver := CallReceiver(called)
		for index, parameter := range function.Params {
			if index < len(common.Args) && ValueDerivesFrom(receiver, parameter, map[ssa.Value]bool{}) &&
				ValueIsAccessPathFrom(common.Args[index], target) {
				return true
			}
		}
		return false
	}
	return MethodCallCoverage(function, calls, CoverageEveryReturn, nil)
}

// StartedClosureCallsMethodOnEveryReturn reports whether a launched closure
// calls method on target before each of its normal returns.
func StartedClosureCallsMethodOnEveryReturn(instruction ssa.Instruction, method string, target ssa.Value) bool {
	if _, ok := instruction.(*ssa.Go); !ok {
		return false
	}
	return mappedCompletion(instruction, method, target, CoverageEveryReturn)
}

// CalledClosureCallsMethodOnEveryReturn reports whether an immediately
// invoked function literal calls method on target before every normal return.
func CalledClosureCallsMethodOnEveryReturn(instruction ssa.Instruction, method string, target ssa.Value) bool {
	return CalledCallbackCallsMethodOnEveryReturn(instruction, method, target)
}

// CallStartsClosureCallingMethodOnArgument reports whether a source-visible
// helper delegates an argument's lifecycle method to a goroutine. Starting the
// waiter transfers the obligation even when the caller joins it separately.
// https://github.com/siemens/wfx/blob/392dde941e73ce9560df2c42b2d480eb528bfc96/middleware/plugin/process_unix_test.go#L35-L45
func CallStartsClosureCallingMethodOnArgument(instruction ssa.Instruction, method string, target ssa.Value) bool {
	return callStartsClosureCallingMethodOnArgument(instruction, method, target, map[*ssa.Function]bool{})
}

// StartedClosureCallsMethodViaHelper reports whether a launched closure passes
// a captured value, or a value derived from it, to a helper chain that starts a
// goroutine calling method. This covers wrappers that project a process handle
// from a captured exec.Cmd before handing it to a waiter helper.
func StartedClosureCallsMethodViaHelper(instruction ssa.Instruction, method string, target ssa.Value) bool {
	if _, ok := instruction.(*ssa.Go); !ok {
		return false
	}
	_, closure, function := calledFunction(instruction)
	if closure == nil || function == nil {
		return false
	}
	for index, free := range function.FreeVars {
		if index >= len(closure.Bindings) || !CapturedBindingMatches(closure.Bindings[index], target) &&
			!ValueContainsValue(closure.Bindings[index], target) {
			continue
		}
		for _, block := range function.Blocks {
			for _, candidate := range block.Instrs {
				if callStartsClosureCallingMethodOnArgument(candidate, method, free, map[*ssa.Function]bool{}) {
					return true
				}
			}
		}
	}
	return false
}

func callStartsClosureCallingMethodOnArgument(instruction ssa.Instruction, method string, target ssa.Value, seen map[*ssa.Function]bool) bool {
	common := InstructionCall(instruction)
	if common == nil || common.StaticCallee() == nil || seen[common.StaticCallee()] {
		return false
	}
	callee := common.StaticCallee()
	seen[callee] = true
	defer delete(seen, callee)
	for index, argument := range common.Args {
		if index >= len(callee.Params) || !SameValue(argument, target) && !ValueContainsValue(argument, target) &&
			!ValueDerivesFrom(argument, target, map[ssa.Value]bool{}) {
			continue
		}
		for _, block := range callee.Blocks {
			for _, candidate := range block.Instrs {
				if _, ok := candidate.(*ssa.Go); ok && ClosureCallsMethod(candidate, method, callee.Params[index]) {
					return true
				}
				if callStartsClosureCallingMethodOnArgument(candidate, method, callee.Params[index], seen) {
					return true
				}
			}
		}
	}
	return false
}

// ClosureCallsMethodBeforeBranch reports whether a called function invokes
// method on target along an unconditional path from its entry block.
func ClosureCallsMethodBeforeBranch(instruction ssa.Instruction, method string, target ssa.Value) bool {
	return mappedCompletion(instruction, method, target, CoverageBeforeBranch)
}

// CalledClosureCallsMethodBeforeBranch reports whether an immediately invoked
// closure unconditionally calls method, including through a nested defer.
func CalledClosureCallsMethodBeforeBranch(instruction ssa.Instruction, method string, target ssa.Value) bool {
	if _, ok := instruction.(*ssa.Call); !ok {
		return false
	}
	common := InstructionCall(instruction)
	if common == nil {
		return false
	}
	if _, ok := common.Value.(*ssa.MakeClosure); !ok {
		return false
	}
	return ClosureCallsMethodBeforeBranch(instruction, method, target)
}

func closureDefersMethodOnMappedTarget(
	instruction ssa.Instruction,
	method string,
	common *ssa.CallCommon,
	closure *ssa.MakeClosure,
	function *ssa.Function,
	target ssa.Value,
) bool {
	if _, ok := instruction.(*ssa.Defer); !ok {
		return false
	}
	for _, free := range function.FreeVars {
		if calledReceiverMatches(common, closure, function, free, target) && DeferredClosureCalls(instruction, method, free) {
			return true
		}
	}
	for _, parameter := range function.Params {
		if calledReceiverMatches(common, closure, function, parameter, target) && DeferredClosureCalls(instruction, method, parameter) {
			return true
		}
	}
	return false
}

func calledFunction(instruction ssa.Instruction) (*ssa.CallCommon, *ssa.MakeClosure, *ssa.Function) {
	if closure, ok := instruction.(*ssa.MakeClosure); ok {
		function, _ := closure.Fn.(*ssa.Function)
		return nil, closure, function
	}
	common := InstructionCall(instruction)
	if common == nil {
		return nil, nil, nil
	}
	closure, _ := common.Value.(*ssa.MakeClosure)
	function := common.StaticCallee()
	if closure != nil {
		function, _ = closure.Fn.(*ssa.Function)
	}
	return common, closure, function
}

func calledReceiverMatches(common *ssa.CallCommon, closure *ssa.MakeClosure, function *ssa.Function, receiver, target ssa.Value) bool {
	if closure != nil {
		for index, free := range function.FreeVars {
			if ValueDerivesFrom(receiver, free, map[ssa.Value]bool{}) && index < len(closure.Bindings) &&
				(CapturedBindingMatches(closure.Bindings[index], target) ||
					ValueDerivesFrom(CapturedBindingValue(closure.Bindings[index]), target, map[ssa.Value]bool{}) ||
					SameAccessPath(
						AccessPath{Value: receiver, Root: free},
						AccessPath{Value: target, Root: closure.Bindings[index]},
					)) {
				return true
			}
		}
	}
	if common == nil {
		return false
	}
	for index, parameter := range function.Params {
		if !ValueDerivesFrom(receiver, parameter, map[ssa.Value]bool{}) || index >= len(common.Args) {
			continue
		}
		direct := ProveIdentity(
			AccessPath{Value: common.Args[index]},
			AccessPath{Value: target},
		)
		mapped := ProveIdentity(
			AccessPath{Value: receiver, Root: parameter},
			AccessPath{Value: target, Root: common.Args[index]},
		)
		if direct.Proven() || mapped.Proven() {
			return true
		}
	}
	return false
}
