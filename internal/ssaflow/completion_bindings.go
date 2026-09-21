package ssaflow

import "golang.org/x/tools/go/ssa"

// Callback bindings preserve direct function arguments while the existing
// completion search enters a visible helper. They resolve identity only:
// ordinary completion coverage still decides whether the call must happen.
// No caller enumeration, heap propagation, or cross-package facts are needed.
type callbackBindings struct {
	values map[ssa.Value]ssa.Value
}

func (search *completionSearch) bindCallbackArguments(callee completionCallee) *callbackBindings {
	if callee.common == nil {
		return nil
	}
	var bindings *callbackBindings
	for index, parameter := range callee.function.Params {
		if index >= len(callee.common.Args) || !search.budget.Spend() {
			break
		}
		value := callee.common.Args[index]
		if search.bindings != nil {
			if supplied, ok := search.bindings.values[value]; ok {
				value = supplied
			}
		}
		// Immutable SSA function values need no alias proof. Loads, phis,
		// returned callbacks and interface dispatch deliberately remain opaque.
		switch value.(type) {
		case *ssa.Function, *ssa.MakeClosure:
			if bindings == nil {
				bindings = &callbackBindings{values: make(map[ssa.Value]ssa.Value)}
			}
			bindings.values[parameter] = value
		}
	}
	return bindings
}

func (search *completionSearch) boundCallees(instruction ssa.Instruction) ([]completionCallee, bool) {
	common := InstructionCall(instruction)
	if common == nil || common.IsInvoke() || search.bindings == nil {
		return resolveCallees(instruction)
	}
	value, ok := search.bindings.values[common.Value]
	if !ok {
		return resolveCallees(instruction)
	}
	// Keep the invocation's arguments in its own SSA scope. Only its function
	// value is substituted; mappedLocals maps those arguments into the resolved
	// callback. Rewriting the shared SSA would contaminate other call sites.
	resolved := *common
	resolved.Value = value
	switch instruction.(type) {
	case *ssa.Call:
		return calleesOf(&resolved, launchCalled, instruction, false)
	case *ssa.Defer:
		return calleesOf(&resolved, launchDeferred, instruction, true)
	case *ssa.Go:
		return calleesOf(&resolved, launchStarted, instruction, false)
	}
	return nil, false
}
