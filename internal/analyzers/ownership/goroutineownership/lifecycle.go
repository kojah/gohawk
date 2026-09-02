package goroutineownership

import (
	"go/types"
	"strings"

	"github.com/kojah/gohawk/internal/ssaflow"
	"github.com/kojah/gohawk/internal/syntax"

	"golang.org/x/tools/go/ssa"
)

// Lifecycle evidence bounds a worker from outside the spawning function: it
// receives from a caller-owned stop channel or context, runs inside a synctest
// bubble, or, for the opt-in detached audit only, runs on a value whose
// lifecycle method the parent later invokes. None of this proves a join; it
// proves that the parent was never the owner in the first place.

// goroutineReceivesCallerSignal reports whether the worker receives from a
// channel supplied by the caller, directly or through static helpers that take
// the exact channel. Kubernetes informers express context ownership as
// Run(ctx.Done()):
// https://github.com/prometheus/prometheus/blob/e06b2dc5a6149e20ca82fe936fb044a6dfe45958/discovery/kubernetes/kubernetes.go#L438-L458
// Reminal passes its stop channel through several small helpers:
// https://github.com/harshalgajjar/Reminal/blob/c4fd9e64b3b1deabaaacd5e10b9090a28792148d/internal/client/directoryhost.go#L62-L106
func goroutineReceivesCallerSignal(spawn *ssa.Go) bool {
	function, closure := spawnedFunction(spawn)
	if function == nil {
		return false
	}
	for _, block := range function.Blocks {
		for _, instruction := range block.Instrs {
			if receivesFrom(instruction, func(channel ssa.Value) bool {
				return callerSuppliedValue(spawn, function, closure, channel)
			}) {
				return true
			}
		}
	}
	return spawnedParameterIsReceived(spawn, function, closure, func(value ssa.Value) bool {
		channel, ok := value.Type().Underlying().(*types.Chan)
		return ok && channel.Dir() == types.RecvOnly
	})
}

// goroutineReceivesCallerContext reports whether the worker, or a static helper
// it passes the exact context to, receives from a caller-owned context.
func goroutineReceivesCallerContext(spawn *ssa.Go) bool {
	function, closure := spawnedFunction(spawn)
	if function == nil {
		return false
	}
	return spawnedParameterIsReceived(spawn, function, closure, func(value ssa.Value) bool {
		return syntax.NamedType(value.Type(), "context", "Context")
	})
}

// callerSuppliedValue maps a value used by the worker back to the parent and
// requires that parent value to outlive the call. A channel field of a captured
// aggregate keeps the exact field path rooted at that capture:
// https://github.com/charmbracelet/wishlist/blob/3404a9e6f1d3e544a59e95302bfbe575bf1cf75e/server.go#L44-L51
func callerSuppliedValue(spawn *ssa.Go, function *ssa.Function, closure *ssa.MakeClosure, value ssa.Value) bool {
	if supplied := ssaflow.SpawnedValueAtCall(spawn, function, closure, value); supplied != nil {
		return ssaflow.ExternallyOwnedValue(supplied)
	}
	if closure == nil {
		return false
	}
	for index, free := range function.FreeVars {
		if index < len(closure.Bindings) && ssaflow.ValueIsAccessPathFrom(value, free) &&
			ssaflow.ExternallyOwnedValue(ssaflow.CapturedBindingValue(closure.Bindings[index])) {
			return true
		}
	}
	return false
}

// spawnedParameterIsReceived reports whether a caller-owned parameter or
// capture accepted by typed is received from by the worker or by a static
// helper chain it hands the exact value to.
func spawnedParameterIsReceived(spawn *ssa.Go, function *ssa.Function, closure *ssa.MakeClosure, typed func(ssa.Value) bool) bool {
	for index, parameter := range function.Params {
		if index < len(spawn.Common().Args) && typed(parameter) && ssaflow.ExternallyOwnedValue(spawn.Common().Args[index]) &&
			receivesAnywhere(function, parameter, map[*ssa.Function]bool{}) {
			return true
		}
	}
	if closure == nil {
		return false
	}
	for index, free := range function.FreeVars {
		if index < len(closure.Bindings) && typed(free) &&
			ssaflow.ExternallyOwnedValue(ssaflow.CapturedBindingValue(closure.Bindings[index])) &&
			receivesAnywhere(function, free, map[*ssa.Function]bool{}) {
			return true
		}
	}
	return false
}

// receivesAnywhere reports whether function, or a static helper it hands the
// exact value to, receives from local on any path. A bounded worker commonly
// selects on its stop signal inside a loop, so every-return coverage is not
// required here; this evidence never proves a join, only a caller-owned bound.
func receivesAnywhere(function *ssa.Function, local ssa.Value, seen map[*ssa.Function]bool) bool {
	if function == nil || seen[function] {
		return false
	}
	seen[function] = true
	derives := func(value ssa.Value) bool {
		return ssaflow.ValueDerivesFrom(value, local, map[ssa.Value]bool{})
	}
	for _, block := range function.Blocks {
		for _, instruction := range block.Instrs {
			if receivesFrom(instruction, derives) {
				return true
			}
			common := ssaflow.InstructionCall(instruction)
			if common == nil {
				continue
			}
			callee, closure := calledFunction(common)
			if callee == nil {
				continue
			}
			for _, pair := range suppliedValues(common, callee, closure) {
				if derives(pair.supplied) && receivesAnywhere(callee, pair.local, seen) {
					return true
				}
			}
		}
	}
	return false
}

// synctestOwnsGoroutine recognizes a worker launched from the callback passed
// to synctest.Test, which waits for every goroutine in its bubble:
// https://github.com/golang/go/blob/8af21751f066eced273ca3ce49506b366847c623/src/testing/synctest/synctest.go#L275-L293
func synctestOwnsGoroutine(function *ssa.Function) bool {
	if function == nil || function.Parent() == nil {
		return false
	}
	for _, block := range function.Parent().Blocks {
		for _, instruction := range block.Instrs {
			common := ssaflow.InstructionCall(instruction)
			if !ssaflow.CallMatchesSymbol(common, syntax.PackageFunction("testing/synctest", "Test")) {
				continue
			}
			for _, argument := range common.Args {
				if callbackFunction(argument) == function {
					return true
				}
			}
		}
	}
	return false
}

func callbackFunction(value ssa.Value) *ssa.Function {
	if inner, ok := ssaflow.UnwrapTransparentValue(
		value,
		ssaflow.TransparentChangeInterface|ssaflow.TransparentChangeType|ssaflow.TransparentConvert|ssaflow.TransparentMakeInterface,
	); ok {
		return callbackFunction(inner)
	}
	switch typed := value.(type) {
	case *ssa.Function:
		return typed
	case *ssa.MakeClosure:
		function, _ := typed.Fn.(*ssa.Function)
		return function
	default:
		return nil
	}
}

// spawnedLifecycleOwners returns the receiver and captured values that expose
// a lifecycle method. This is the only name-based evidence in the analyzer and
// it feeds the opt-in detached audit alone; the default check never consults
// it. A WaitGroup is excluded so its Wait cannot bypass the terminal Done proof.
func spawnedLifecycleOwners(spawn *ssa.Go) []ssa.Value {
	var owners []ssa.Value
	if receiver := ssaflow.CallReceiver(spawn.Common()); lifecycleOwner(receiver) {
		owners = append(owners, receiver)
	}
	closure, ok := spawn.Common().Value.(*ssa.MakeClosure)
	if !ok {
		return owners
	}
	for _, binding := range closure.Bindings {
		if value := ssaflow.CapturedBindingValue(binding); lifecycleOwner(value) {
			owners = append(owners, value)
		}
	}
	return owners
}

func lifecycleOwner(value ssa.Value) bool {
	if value == nil || syntax.NamedType(value.Type(), "sync", "WaitGroup") {
		return false
	}
	for method := range types.NewMethodSet(value.Type()).Methods() {
		if lifecycleMethod(method.Obj().Name()) {
			return true
		}
	}
	return false
}

func lifecycleMethod(name string) bool {
	switch strings.ToLower(name) {
	case "close", "kill", "shutdown", "stop", "wait":
		return true
	default:
		return false
	}
}
