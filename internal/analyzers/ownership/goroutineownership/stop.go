package goroutineownership

import (
	"go/token"
	"go/types"

	"github.com/kojah/gohawk/internal/ssaflow"

	"golang.org/x/tools/go/ssa"
)

// Stop-lifecycle evidence proves that a launched function observes the exact
// channel supplied by its owner. Static access paths and helper parameters are
// followed, but opaque callbacks and ambiguous aliases remain unproven.

func goroutineHasStopLifecycle(spawn *ssa.Go) bool {
	function := spawn.Common().StaticCallee()
	closure, _ := spawn.Common().Value.(*ssa.MakeClosure)
	if closure != nil {
		function, _ = closure.Fn.(*ssa.Function)
	}
	if function == nil {
		return false
	}
	// Generic calls may point at an instantiated wrapper whose body does not
	// expose the receive. The origin has the same parameter positions and the
	// source body needed to prove that the helper joins the signal.
	if origin := function.Origin(); origin != nil {
		function = origin
	}
	for _, block := range function.Blocks {
		for _, instruction := range block.Instrs {
			switch candidate := instruction.(type) {
			case *ssa.UnOp:
				if candidate.Op == token.ARROW && spawnedLifecycleValueAtCall(spawn, function, closure, candidate.X) {
					return true
				}
			case *ssa.Range:
				if _, channelRange := candidate.X.Type().Underlying().(*types.Chan); channelRange &&
					spawnedLifecycleValueAtCall(spawn, function, closure, candidate.X) {
					// Ranging a channel is the repeated form of the direct receive
					// lifecycle above: the goroutine remains owned until the channel
					// closes.
					return true
				}
			case *ssa.Select:
				for _, state := range candidate.States {
					if state.Dir == types.RecvOnly && spawnedLifecycleValueAtCall(spawn, function, closure, state.Chan) {
						return true
					}
				}
			}
		}
	}
	return false
}

func spawnedLifecycleValueAtCall(spawn *ssa.Go, function *ssa.Function, closure *ssa.MakeClosure, value ssa.Value) bool {
	if supplied := ssaflow.SpawnedValueAtCall(spawn, function, closure, value); supplied != nil {
		return ssaflow.ExternallyOwnedValue(supplied)
	}
	if closure == nil {
		return false
	}
	for index, free := range function.FreeVars {
		if index < len(closure.Bindings) && ssaflow.ValueIsAccessPathFrom(value, free) &&
			ssaflow.ExternallyOwnedValue(ssaflow.CapturedBindingValue(closure.Bindings[index])) {
			// A closure may receive through a channel field of a captured aggregate.
			// Keep the proof tied to the exact field path rooted at that capture;
			// unrelated local ranges do not establish an external stop lifecycle.
			// https://github.com/charmbracelet/wishlist/blob/3404a9e6f1d3e544a59e95302bfbe575bf1cf75e/server.go#L44-L51
			return true
		}
	}
	return false
}

func goroutineHasHelperStopLifecycle(spawn *ssa.Go) bool {
	function := spawn.Common().StaticCallee()
	closure, _ := spawn.Common().Value.(*ssa.MakeClosure)
	if closure != nil {
		function, _ = closure.Fn.(*ssa.Function)
	}
	if function == nil {
		return false
	}
	if origin := function.Origin(); origin != nil {
		function = origin
	}
	for index, parameter := range function.Params {
		if index < len(spawn.Common().Args) && receiveOnlyChannel(parameter) && ssaflow.ExternallyOwnedValue(spawn.Common().Args[index]) &&
			functionReceivesParameter(function, parameter, map[*ssa.Function]bool{}) {
			// A source-visible helper may own the receive while its caller owns
			// the goroutine. Follow only exact static parameter flow: Reminal's
			// directory host passes its stop channel through several small helpers.
			// https://github.com/harshalgajjar/Reminal/blob/c4fd9e64b3b1deabaaacd5e10b9090a28792148d/internal/client/directoryhost.go#L62-L106
			return true
		}
	}
	if closure == nil {
		return false
	}
	for index, free := range function.FreeVars {
		if index < len(closure.Bindings) && receiveOnlyChannel(free) &&
			ssaflow.ExternallyOwnedValue(ssaflow.CapturedBindingValue(closure.Bindings[index])) &&
			functionReceivesParameter(function, free, map[*ssa.Function]bool{}) {
			return true
		}
	}
	return false
}

func goroutineConsumesContextLifecycle(spawn *ssa.Go) bool {
	function := spawn.Common().StaticCallee()
	closure, _ := spawn.Common().Value.(*ssa.MakeClosure)
	if closure != nil {
		function, _ = closure.Fn.(*ssa.Function)
	}
	if function == nil {
		return false
	}
	if origin := function.Origin(); origin != nil {
		function = origin
	}
	for index, parameter := range function.Params {
		if index < len(spawn.Common().Args) && contextValue(parameter) &&
			functionReceivesParameter(function, parameter, map[*ssa.Function]bool{}) {
			return true
		}
	}
	if closure == nil {
		return false
	}
	for index, free := range function.FreeVars {
		if index < len(closure.Bindings) && contextValue(free) &&
			functionReceivesParameter(function, free, map[*ssa.Function]bool{}) {
			return true
		}
	}
	return false
}

func receiveOnlyChannel(value ssa.Value) bool {
	channel, ok := value.Type().Underlying().(*types.Chan)
	return ok && channel.Dir() == types.RecvOnly
}
