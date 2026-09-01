package ssautil

import "golang.org/x/tools/go/ssa"

// ClosureCallsMethod reports whether a call-like closure calls method on target.
// It maps both captured free variables and explicit closure parameters back to
// the values supplied by the enclosing function.
func ClosureCallsMethod(instruction ssa.Instruction, method string, target ssa.Value) bool {
	common, closure, function := calledFunction(instruction)
	if function == nil {
		return false
	}
	for _, block := range function.Blocks {
		for _, candidate := range block.Instrs {
			called := InstructionCall(candidate)
			if CallName(called) != method {
				continue
			}
			receiver := CallReceiver(called)
			if calledReceiverMatches(common, closure, function, receiver, target) {
				return true
			}
		}
	}
	return false
}

// StartedClosureCallsMethodOnEveryReturn reports whether a launched closure
// calls method on target before each of its normal returns.
func StartedClosureCallsMethodOnEveryReturn(instruction ssa.Instruction, method string, target ssa.Value) bool {
	if _, ok := instruction.(*ssa.Go); !ok {
		return false
	}
	common, closure, function := calledFunction(instruction)
	if function == nil || len(function.Blocks) == 0 {
		return false
	}
	hasReturn, hasCleanup := false, false
	for _, block := range function.Blocks {
		for _, candidate := range block.Instrs {
			if _, ok := candidate.(*ssa.Return); ok {
				hasReturn = true
			}
			called := InstructionCall(candidate)
			if CallName(called) == method && calledReceiverMatches(common, closure, function, CallReceiver(called), target) {
				hasCleanup = true
			}
		}
	}
	return hasReturn && hasCleanup && !UnownedReturnFromEntry(function, func(candidate ssa.Instruction) bool {
		called := InstructionCall(candidate)
		return CallName(called) == method && calledReceiverMatches(common, closure, function, CallReceiver(called), target)
	})
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
	common, closure, function := calledFunction(instruction)
	if function == nil || len(function.Blocks) == 0 {
		return false
	}
	visited := map[*ssa.BasicBlock]bool{}
	for block := function.Blocks[0]; block != nil && !visited[block]; {
		visited[block] = true
		for _, candidate := range block.Instrs {
			called := InstructionCall(candidate)
			if CallName(called) == method && calledReceiverMatches(common, closure, function, CallReceiver(called), target) {
				return true
			}
		}
		if len(block.Succs) != 1 {
			return false
		}
		block = block.Succs[0]
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
