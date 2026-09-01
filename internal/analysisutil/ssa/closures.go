package ssautil

import (
	"slices"
	"strings"

	"golang.org/x/tools/go/ssa"
)

func DeferredClosureCalls(instruction ssa.Instruction, method string, target ssa.Value) bool {
	if _, ok := instruction.(*ssa.Defer); !ok {
		return false
	}
	return ClosureCallsMethod(instruction, method, target)
}

// DeferredClosureCallsValue reports whether a deferred closure calls target.
func DeferredClosureCallsValue(instruction ssa.Instruction, target ssa.Value) bool {
	if _, ok := instruction.(*ssa.Defer); !ok {
		return false
	}
	return ClosureCallsValue(instruction, target)
}

// DeferredClosureInvokesArgumentOnEveryReturn reports whether a deferred
// closure delegates target to a helper that invokes it on every normal path.
func DeferredClosureInvokesArgumentOnEveryReturn(instruction ssa.Instruction, target ssa.Value) bool {
	if _, ok := instruction.(*ssa.Defer); !ok {
		return false
	}
	common, closure, function := calledFunction(instruction)
	if function == nil {
		return false
	}
	for _, block := range function.Blocks {
		for _, candidate := range block.Instrs {
			for index, free := range function.FreeVars {
				if index < len(closure.Bindings) && CapturedBindingMatches(closure.Bindings[index], target) &&
					CallInvokesArgumentOnEveryReturn(candidate, free) {
					return true
				}
			}
			for index, parameter := range function.Params {
				if common != nil && index < len(common.Args) && SameValue(common.Args[index], target) &&
					CallInvokesArgumentOnEveryReturn(candidate, parameter) {
					return true
				}
			}
		}
	}
	return false
}

// DeferredClosurePassesValueToNamedCall reports whether a deferred closure
// passes target to a call whose name contains one of fragments.
func DeferredClosurePassesValueToNamedCall(instruction ssa.Instruction, target ssa.Value, fragments ...string) bool {
	if _, ok := instruction.(*ssa.Defer); !ok {
		return false
	}
	common, closure, function := calledFunction(instruction)
	if function == nil {
		return false
	}
	for _, block := range function.Blocks {
		for _, candidate := range block.Instrs {
			called := InstructionCall(candidate)
			name := strings.ToLower(CallName(called))
			if called == nil || !slices.ContainsFunc(fragments, func(fragment string) bool {
				return strings.Contains(name, fragment)
			}) {
				continue
			}
			for _, argument := range called.Args {
				for index, free := range function.FreeVars {
					if index < len(closure.Bindings) && ValueDerivesFrom(argument, free, map[ssa.Value]bool{}) &&
						CapturedBindingMatches(closure.Bindings[index], target) {
						return true
					}
				}
				for index, parameter := range function.Params {
					if common != nil && index < len(common.Args) && ValueDerivesFrom(argument, parameter, map[ssa.Value]bool{}) &&
						SameValue(common.Args[index], target) {
						return true
					}
				}
			}
		}
	}
	return false
}

// ClosureCallsValue reports whether a call-like closure or created callback calls target.
func ClosureCallsValue(instruction ssa.Instruction, target ssa.Value) bool {
	var closure *ssa.MakeClosure
	if created, ok := instruction.(*ssa.MakeClosure); ok {
		if created.Referrers() == nil || len(*created.Referrers()) == 0 {
			return false
		}
		closure = created
	} else if common := InstructionCall(instruction); common != nil {
		closure, _ = common.Value.(*ssa.MakeClosure)
	}
	if closure == nil {
		return false
	}
	return closureCallsValue(closure, target)
}

// ValueCallsValue reports whether value is, or wraps, a callback that invokes
// target. It follows common callback wrappers and addressable locals so callers
// can recognize cleanup registered through higher-order APIs.
func ValueCallsValue(value, target ssa.Value) bool {
	return valueCallsValue(value, target, map[ssa.Value]bool{})
}

// ValueCallsMethod reports whether value is, or wraps, a callback that invokes
// method on target.
func ValueCallsMethod(value ssa.Value, method string, target ssa.Value) bool {
	return valueCallsMethod(value, method, target, map[ssa.Value]bool{})
}

func valueCallsMethod(value ssa.Value, method string, target ssa.Value, seen map[ssa.Value]bool) bool {
	if value == nil || seen[value] {
		return false
	}
	seen[value] = true
	if closure, ok := value.(*ssa.MakeClosure); ok {
		if ClosureCallsMethod(closure, method, target) {
			return true
		}
		if function, ok := closure.Fn.(*ssa.Function); ok {
			for _, block := range function.Blocks {
				for _, instruction := range block.Instrs {
					if nested, ok := instruction.(*ssa.MakeClosure); ok && valueCallsMethod(nested, method, target, seen) {
						return true
					}
					common := InstructionCall(instruction)
					if common == nil {
						continue
					}
					for index, free := range function.FreeVars {
						if index < len(closure.Bindings) && ValueDerivesFrom(common.Value, free, map[ssa.Value]bool{}) &&
							valueCallsMethod(closure.Bindings[index], method, target, seen) {
							return true
						}
					}
				}
			}
		}
	}
	switch typed := value.(type) {
	case *ssa.Alloc:
		if typed.Referrers() != nil {
			for _, reference := range *typed.Referrers() {
				store, ok := reference.(*ssa.Store)
				if ok && store.Addr == typed && valueCallsMethod(store.Val, method, target, seen) {
					return true
				}
			}
		}
	case *ssa.Call:
		for _, argument := range typed.Common().Args {
			if valueCallsMethod(argument, method, target, seen) {
				return true
			}
		}
	case *ssa.ChangeInterface:
		return valueCallsMethod(typed.X, method, target, seen)
	case *ssa.ChangeType:
		return valueCallsMethod(typed.X, method, target, seen)
	case *ssa.Convert:
		return valueCallsMethod(typed.X, method, target, seen)
	case *ssa.MakeInterface:
		return valueCallsMethod(typed.X, method, target, seen)
	case *ssa.UnOp:
		if typed.X.Referrers() != nil {
			for _, reference := range *typed.X.Referrers() {
				store, ok := reference.(*ssa.Store)
				if ok && store.Addr == typed.X && valueCallsMethod(store.Val, method, target, seen) {
					return true
				}
			}
		}
	case *ssa.Phi:
		for _, edge := range typed.Edges {
			if valueCallsMethod(edge, method, target, seen) {
				return true
			}
		}
	}
	return false
}

func valueCallsValue(value, target ssa.Value, seen map[ssa.Value]bool) bool {
	if value == nil || seen[value] {
		return false
	}
	seen[value] = true
	if closure, ok := value.(*ssa.MakeClosure); ok {
		if closureCallsValue(closure, target) {
			return true
		}
		if function, ok := closure.Fn.(*ssa.Function); ok {
			for _, block := range function.Blocks {
				for _, instruction := range block.Instrs {
					inner, ok := instruction.(*ssa.MakeClosure)
					if ok && valueCallsValue(inner, target, seen) {
						return true
					}
				}
			}
		}
	}
	switch typed := value.(type) {
	case *ssa.Alloc:
		if typed.Referrers() != nil {
			for _, reference := range *typed.Referrers() {
				store, ok := reference.(*ssa.Store)
				if ok && store.Addr == typed && valueCallsValue(store.Val, target, seen) {
					return true
				}
			}
		}
	case *ssa.ChangeInterface:
		return valueCallsValue(typed.X, target, seen)
	case *ssa.ChangeType:
		return valueCallsValue(typed.X, target, seen)
	case *ssa.Convert:
		return valueCallsValue(typed.X, target, seen)
	case *ssa.MakeInterface:
		return valueCallsValue(typed.X, target, seen)
	case *ssa.UnOp:
		if typed.X.Referrers() != nil {
			for _, reference := range *typed.X.Referrers() {
				store, ok := reference.(*ssa.Store)
				if ok && store.Addr == typed.X && valueCallsValue(store.Val, target, seen) {
					return true
				}
			}
		}
	case *ssa.Phi:
		for _, edge := range typed.Edges {
			if valueCallsValue(edge, target, seen) {
				return true
			}
		}
	}
	return false
}

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
					SameAccessPath(receiver, free, target, closure.Bindings[index])) {
				return true
			}
		}
	}
	if common == nil {
		return false
	}
	for index, parameter := range function.Params {
		if ValueDerivesFrom(receiver, parameter, map[ssa.Value]bool{}) && index < len(common.Args) && SameValue(common.Args[index], target) {
			return true
		}
	}
	return false
}
