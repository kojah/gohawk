package goroutineownership

import (
	"go/token"
	"go/types"
	"slices"
	"strings"

	"github.com/kojah/gohawk/internal/analysisutil"
	ssautil "github.com/kojah/gohawk/internal/analysisutil/ssa"

	"golang.org/x/tools/go/ssa"
)

func goroutineHasContextLifecycle(spawn *ssa.Go) bool {
	if slices.ContainsFunc(spawn.Common().Args, contextValue) {
		return true
	}
	closure, ok := spawn.Common().Value.(*ssa.MakeClosure)
	if !ok {
		return false
	}
	for _, binding := range closure.Bindings {
		if contextValue(ssautil.CapturedBindingValue(binding)) {
			return true
		}
	}
	return false
}

func contextValue(value ssa.Value) bool {
	return value != nil && analysisutil.NamedType(value.Type(), "context", "Context")
}

func externallyOwnedLifecycle(owners []ssa.Value) bool {
	return slices.ContainsFunc(owners, ssautil.ExternallyOwnedValue)
}

func externallyOwnedJoin(signals, groups []ssa.Value) bool {
	// A goroutine that completes through a caller-owned channel or wait group
	// transfers its join obligation across the call boundary.
	return slices.ContainsFunc(append(slices.Clone(signals), groups...), ssautil.ExternallyOwnedValue)
}

func goroutineLifecycleValues(spawn *ssa.Go) []ssa.Value {
	var owners []ssa.Value
	receiver := ssautil.CallReceiver(spawn.Common())
	if lifecycleOwner(receiver) {
		owners = append(owners, receiver)
	}
	closure, ok := spawn.Common().Value.(*ssa.MakeClosure)
	if !ok {
		return owners
	}
	for _, binding := range closure.Bindings {
		value := ssautil.CapturedBindingValue(binding)
		if lifecycleOwner(value) {
			owners = append(owners, value)
		}
	}
	return owners
}

func goroutineJoinValues(spawn *ssa.Go) (signals, groups []ssa.Value) {
	// Infer join handles from what the goroutine produces (send, close, Done),
	// rather than treating every channel argument as completion. A channel the
	// goroutine only receives from governs its lifetime instead.
	function := spawn.Common().StaticCallee()
	closure, _ := spawn.Common().Value.(*ssa.MakeClosure)
	if closure != nil {
		function, _ = closure.Fn.(*ssa.Function)
	}
	if function == nil {
		return signals, nil
	}
	for _, block := range function.Blocks {
		for _, instruction := range block.Instrs {
			signal, group := spawnedOwnershipValue(spawn, function, closure, instruction)
			if signal != nil {
				signals = append(signals, signal)
			}
			if group != nil {
				groups = append(groups, group)
			}
		}
	}
	return signals, groups
}

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
				if candidate.Op == token.ARROW && ssautil.SpawnedValueAtCall(spawn, function, closure, candidate.X) != nil {
					return true
				}
			case *ssa.Select:
				for _, state := range candidate.States {
					if state.Dir == types.RecvOnly && ssautil.SpawnedValueAtCall(spawn, function, closure, state.Chan) != nil {
						return true
					}
				}
			}
		}
	}
	return false
}

func goroutineTransferredToCaller(function *ssa.Function, spawn *ssa.Go) bool {
	for _, owner := range function.Params {
		if !lifecycleOwner(owner) {
			continue
		}
		if ssautil.SameValue(ssautil.CallReceiver(spawn.Common()), owner) {
			return true
		}
		closure, ok := spawn.Common().Value.(*ssa.MakeClosure)
		if !ok {
			continue
		}
		for _, binding := range closure.Bindings {
			if ssautil.SameValue(ssautil.CapturedBindingValue(binding), owner) {
				return true
			}
		}
	}
	return false
}

func lifecycleOwner(value ssa.Value) bool {
	if value == nil {
		return false
	}
	methods := types.NewMethodSet(value.Type())
	for method := range methods.Methods() {
		if lifecycleMethod(method.Obj().Name()) {
			return true
		}
	}
	return false
}

func ownsGoroutineLifecycle(instruction ssa.Instruction, owners []ssa.Value) bool {
	common := ssautil.InstructionCall(instruction)
	if common != nil && lifecycleMethod(ssautil.CallName(common)) && ssautil.SameAsAny(ssautil.CallReceiver(common), owners) {
		return true
	}
	for _, owner := range owners {
		for _, method := range []string{"Close", "Kill", "Shutdown", "Stop", "Wait"} {
			if ssautil.DeferredClosureCalls(instruction, method, owner) {
				return true
			}
		}
	}
	return false
}

func waitsForLifecycleOwner(instruction ssa.Instruction, owners []ssa.Value) bool {
	common := ssautil.InstructionCall(instruction)
	if common != nil && ssautil.CallName(common) == "Wait" && ssautil.SameAsAny(ssautil.CallReceiver(common), owners) {
		return true
	}
	for _, owner := range owners {
		if ssautil.DeferredClosureCalls(instruction, "Wait", owner) {
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

func ownershipRegisteredBefore(spawn *ssa.Go, signals, groups []ssa.Value) bool {
	index := ssautil.InstructionIndex(spawn)
	if index < 0 {
		return false
	}
	for _, block := range spawn.Parent().Blocks {
		if !block.Dominates(spawn.Block()) {
			continue
		}
		limit := len(block.Instrs)
		if block == spawn.Block() {
			limit = index
		}
		for _, instruction := range block.Instrs[:limit] {
			for _, signal := range signals {
				if ssautil.StoresValueInEscapingField(instruction, signal) {
					return true
				}
			}
			if _, deferred := instruction.(*ssa.Defer); deferred && callReceivesAny(instruction, signals) {
				return true
			}
			common := ssautil.InstructionCall(instruction)
			name := strings.ToLower(ssautil.CallName(common))
			if common == nil || !ownershipRegistrationName(name) {
				continue
			}
			for _, argument := range common.Args {
				if ssautil.SameAsAny(argument, signals) || ssautil.SameAsAny(argument, groups) {
					return true
				}
			}
		}
	}
	return false
}

func ownershipRegistrationName(name string) bool {
	return name == "add" || strings.Contains(name, "register") || strings.Contains(name, "track") || strings.Contains(name, "own")
}

func transfersGoroutineOwnership(instruction ssa.Instruction, signals, groups, owners []ssa.Value) bool {
	values := append(slices.Clone(signals), groups...)
	values = append(values, owners...)
	for _, value := range values {
		if ssautil.StoresValueInField(instruction, value) || ssautil.StoresOwnerOfValueInField(instruction, value) ||
			ssautil.StoresValueInOwnedMap(instruction, value) ||
			ssautil.CallTransfersValueToField(instruction, value) {
			return true
		}
	}
	if _, ok := instruction.(*ssa.Go); ok {
		// A second goroutine may take ownership of a completion signal and
		// publish its own join handle. The original worker is then joined
		// transitively when the caller joins that relay.
		if callReceivesAny(instruction, signals) {
			return true
		}
		for _, group := range groups {
			if ssautil.ClosureCallsMethod(instruction, "Wait", group) {
				return true
			}
		}
	}
	common := ssautil.InstructionCall(instruction)
	if mockReturnOwnsSignal(common, signals) {
		return true
	}
	if common == nil || !ownershipRegistrationName(strings.ToLower(ssautil.CallName(common))) {
		return false
	}
	for _, argument := range common.Args {
		if ssautil.SameAsAny(argument, signals) || ssautil.SameAsAny(argument, groups) {
			return true
		}
	}
	return false
}

func mockReturnOwnsSignal(common *ssa.CallCommon, signals []ssa.Value) bool {
	if !ssautil.HasLibraryContract(common, ssautil.ContractGoMockReturn) {
		return false
	}
	// gomock.Return publishes these values as the configured result of the
	// mocked call, transferring a produced stream to the code under test.
	// https://github.com/uber-go/mock/blob/539d81c0f42174d17e8f91abcb869bed37605a15/gomock/call.go#L185-L205
	for _, argument := range common.Args {
		for _, signal := range signals {
			if ssautil.SameValue(argument, signal) || ssautil.ValueContainsValue(argument, signal) {
				return true
			}
		}
	}
	return false
}
