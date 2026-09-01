package goroutineownership

import (
	"go/token"
	"go/types"
	"slices"
	"strings"

	"github.com/kojah/gohawk/internal/ssaflow"
	"github.com/kojah/gohawk/internal/syntax"

	"golang.org/x/tools/go/ssa"
)

// Lifecycle evidence identifies caller-owned contexts, stop mechanisms, join
// handles, and explicit ownership registration. An obligation transfers only
// when the spawned goroutine and the receiving owner share traceable values.

func goroutineHasContextLifecycle(spawn *ssa.Go) bool {
	if slices.ContainsFunc(spawn.Common().Args, contextValue) {
		return true
	}
	closure, ok := spawn.Common().Value.(*ssa.MakeClosure)
	if !ok {
		return false
	}
	for _, binding := range closure.Bindings {
		if contextValue(ssaflow.CapturedBindingValue(binding)) {
			return true
		}
	}
	return false
}

func contextValue(value ssa.Value) bool {
	return value != nil && syntax.NamedType(value.Type(), "context", "Context")
}

func externallyOwnedLifecycle(owners []ssa.Value) bool {
	return slices.ContainsFunc(owners, ssaflow.ExternallyOwnedValue)
}

func externallyOwnedJoin(signals, groups []ssa.Value) bool {
	// A goroutine that completes through a caller-owned channel or wait group
	// transfers its join obligation across the call boundary.
	return slices.ContainsFunc(append(slices.Clone(signals), groups...), ssaflow.ExternallyOwnedValue)
}

func returnedAggregateOwnsLifecycle(
	function *ssa.Function,
	spawn *ssa.Go,
	returned *ssa.Return,
	owners []ssa.Value,
) bool {
	if function == nil || spawn == nil || returned == nil || len(owners) == 0 {
		return false
	}
	spawnIndex := ssaflow.InstructionIndex(spawn)
	if spawnIndex < 0 {
		return false
	}
	for _, block := range function.Blocks {
		if !block.Dominates(spawn.Block()) {
			continue
		}
		limit := len(block.Instrs)
		if block == spawn.Block() {
			limit = spawnIndex
		}
		for _, instruction := range block.Instrs[:limit] {
			store, field, ok := lifecycleOwnerFieldStore(instruction, owners)
			if !ok || lifecycleFieldOverwritten(function, store, field) ||
				!ssaflow.ReturnedValueOwnsValue(returned, field.X) {
				continue
			}
			// Returning a concrete aggregate that already held the spawned
			// lifecycle owner transfers the obligation just like returning that
			// owner directly. Requiring the field store to dominate the spawn and
			// rejecting later replacement keeps the proof tied to the exact owner.
			// https://github.com/ob-labs/powercontext-go/blob/22c3e09e67805eb8629fe3872e15a747f4199918/test/downstream/consumer_test.go#L357-L369
			// https://github.com/ob-labs/powercontext-go/blob/22c3e09e67805eb8629fe3872e15a747f4199918/test/downstream/consumer_test.go#L425-L443
			return true
		}
	}
	return false
}

func lifecycleOwnerFieldStore(instruction ssa.Instruction, owners []ssa.Value) (*ssa.Store, *ssa.FieldAddr, bool) {
	store, ok := instruction.(*ssa.Store)
	if !ok || !ssaflow.SameAsAny(store.Val, owners) {
		return nil, nil, false
	}
	field, ok := store.Addr.(*ssa.FieldAddr)
	return store, field, ok
}

func lifecycleFieldOverwritten(function *ssa.Function, original *ssa.Store, field *ssa.FieldAddr) bool {
	for _, block := range function.Blocks {
		for _, instruction := range block.Instrs {
			store, ok := instruction.(*ssa.Store)
			if !ok || store == original || !ssaflow.InstructionMayFollow(original, store) {
				continue
			}
			candidate, ok := store.Addr.(*ssa.FieldAddr)
			if ok && candidate.Field == field.Field && ssaflow.SameValue(candidate.X, field.X) {
				return true
			}
		}
	}
	return false
}

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
					// synctest.Test waits for every goroutine in its bubble before
					// returning, and fails the test if the bubble deadlocks.
					// https://github.com/golang/go/blob/8af21751f066eced273ca3ce49506b366847c623/src/testing/synctest/synctest.go#L275-L293
					return true
				}
			}
		}
	}
	return false
}

func callbackFunction(value ssa.Value) *ssa.Function {
	switch typed := value.(type) {
	case *ssa.Function:
		return typed
	case *ssa.MakeClosure:
		function, _ := typed.Fn.(*ssa.Function)
		return function
	case *ssa.ChangeInterface:
		return callbackFunction(typed.X)
	case *ssa.ChangeType:
		return callbackFunction(typed.X)
	case *ssa.Convert:
		return callbackFunction(typed.X)
	case *ssa.MakeInterface:
		return callbackFunction(typed.X)
	default:
		return nil
	}
}

func goroutineLifecycleValues(spawn *ssa.Go) []ssa.Value {
	var owners []ssa.Value
	receiver := ssaflow.CallReceiver(spawn.Common())
	if lifecycleOwner(receiver) {
		owners = append(owners, receiver)
	}
	closure, ok := spawn.Common().Value.(*ssa.MakeClosure)
	if !ok {
		return owners
	}
	for _, binding := range closure.Bindings {
		value := ssaflow.CapturedBindingValue(binding)
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
				if candidate.Op == token.ARROW && ssaflow.SpawnedValueAtCall(spawn, function, closure, candidate.X) != nil {
					return true
				}
			case *ssa.Select:
				for _, state := range candidate.States {
					if state.Dir == types.RecvOnly && ssaflow.SpawnedValueAtCall(spawn, function, closure, state.Chan) != nil {
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
		if ssaflow.SameValue(ssaflow.CallReceiver(spawn.Common()), owner) {
			return true
		}
		closure, ok := spawn.Common().Value.(*ssa.MakeClosure)
		if !ok {
			continue
		}
		for _, binding := range closure.Bindings {
			if ssaflow.SameValue(ssaflow.CapturedBindingValue(binding), owner) {
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

// A lifecycle owner settles the goroutine obligation only through a direct
// lifecycle method or a deferred completion proof. Merely capturing an object
// with Close or Wait methods would conflate reachability with cleanup.
func ownsGoroutineLifecycle(evidence *ssaflow.EvidenceQuery, instruction ssa.Instruction, owners []ssa.Value) bool {
	common := ssaflow.InstructionCall(instruction)
	if common != nil && lifecycleMethod(ssaflow.CallName(common)) && ssaflow.SameAsAny(ssaflow.CallReceiver(common), owners) {
		return true
	}
	for _, owner := range owners {
		if evidence.Completion(ssaflow.CompletionRequest{
			Instruction: instruction,
			Target:      owner,
			Methods:     []string{"Close", "Kill", "Shutdown", "Stop", "Wait"},
			Modes:       ssaflow.CompletionDeferred,
		}).Proven() {
			return true
		}
	}
	return false
}

func waitsForLifecycleOwner(evidence *ssaflow.EvidenceQuery, instruction ssa.Instruction, owners []ssa.Value) bool {
	common := ssaflow.InstructionCall(instruction)
	if common != nil && ssaflow.CallName(common) == "Wait" && ssaflow.SameAsAny(ssaflow.CallReceiver(common), owners) {
		return true
	}
	for _, owner := range owners {
		if evidence.Completion(ssaflow.CompletionRequest{
			Instruction: instruction,
			Target:      owner,
			Methods:     []string{"Wait"},
			Modes:       ssaflow.CompletionDeferred,
		}).Proven() {
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
	index := ssaflow.InstructionIndex(spawn)
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
				if ssaflow.StoresValueInEscapingField(instruction, signal) {
					return true
				}
			}
			if _, deferred := instruction.(*ssa.Defer); deferred && callReceivesAny(instruction, signals) {
				return true
			}
			common := ssaflow.InstructionCall(instruction)
			name := strings.ToLower(ssaflow.CallName(common))
			if common == nil || !ownershipRegistrationName(name) {
				continue
			}
			for _, argument := range common.Args {
				if ssaflow.SameAsAny(argument, signals) || ssaflow.SameAsAny(argument, groups) {
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

func transfersGoroutineOwnership(
	evidence *ssaflow.EvidenceQuery,
	instruction ssa.Instruction,
	signals, groups, owners []ssa.Value,
) bool {
	values := append(slices.Clone(signals), groups...)
	values = append(values, owners...)
	for _, value := range values {
		if evidence.OwnershipTransfer(ssaflow.OwnershipTransferRequest{
			Instruction: instruction,
			Value:       value,
			Modes: ssaflow.TransferStoredInField | ssaflow.TransferOwnerStoredInField |
				ssaflow.TransferStoredInOwnedMap | ssaflow.TransferCallResultStoredInField,
		}).Proven() {
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
			if evidence.Completion(ssaflow.CompletionRequest{
				Instruction: instruction,
				Target:      group,
				Methods:     []string{"Wait"},
				Modes:       ssaflow.CompletionInClosure,
			}).Proven() {
				return true
			}
		}
	}
	common := ssaflow.InstructionCall(instruction)
	if mockReturnOwnsSignal(common, signals) {
		return true
	}
	if common == nil || !ownershipRegistrationName(strings.ToLower(ssaflow.CallName(common))) {
		return false
	}
	for _, argument := range common.Args {
		if ssaflow.SameAsAny(argument, signals) || ssaflow.SameAsAny(argument, groups) {
			return true
		}
	}
	return false
}

func mockReturnOwnsSignal(common *ssa.CallCommon, signals []ssa.Value) bool {
	if !ssaflow.HasLibraryContract(common, ssaflow.ContractGoMockReturn) {
		return false
	}
	// gomock.Return publishes these values as the configured result of the
	// mocked call, transferring a produced stream to the code under test.
	// https://github.com/uber-go/mock/blob/539d81c0f42174d17e8f91abcb869bed37605a15/gomock/call.go#L185-L205
	for _, argument := range common.Args {
		for _, signal := range signals {
			if ssaflow.SameValue(argument, signal) || ssaflow.ValueContainsValue(argument, signal) {
				return true
			}
		}
	}
	return false
}
