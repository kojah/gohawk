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

func goroutineJoinValues(spawn *ssa.Go) (signals, groups []ssa.Value, unsettledDone ssa.Instruction) {
	// Infer join handles from what the goroutine produces (send, close, Done),
	// rather than treating every channel argument as completion. A channel the
	// goroutine only receives from governs its lifetime instead.
	function := spawn.Common().StaticCallee()
	closure, _ := spawn.Common().Value.(*ssa.MakeClosure)
	if closure != nil {
		function, _ = closure.Fn.(*ssa.Function)
	}
	if function == nil {
		return signals, nil, nil
	}
	for _, block := range function.Blocks {
		for _, instruction := range block.Instrs {
			signal := spawnedCompletionSignal(spawn, function, closure, instruction)
			if signal != nil {
				signals = append(signals, signal)
			}
		}
	}
	groups, unsettledDone = waitGroupCompletionValues(spawn, function, closure)
	return signals, groups, unsettledDone
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
		if index < len(spawn.Common().Args) && receiveOnlyChannel(parameter) &&
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
	// WaitGroup completion is proved from settling Done calls in waitgroup.go.
	// Treating its Wait method as a generic lifecycle owner would bypass that
	// proof and accept an early progress signal as goroutine completion.
	if syntax.NamedType(value.Type(), "sync", "WaitGroup") {
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
func ownsGoroutineLifecycle(evidence *ssaflow.LocalEvidence, instruction ssa.Instruction, owners []ssa.Value) bool {
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

func waitsForLifecycleOwner(evidence *ssaflow.LocalEvidence, instruction ssa.Instruction, owners []ssa.Value) bool {
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

func testingCleanupOwnsLaunchedLifecycle(instruction ssa.Instruction, spawn *ssa.Go) bool {
	common := ssaflow.InstructionCall(instruction)
	if !ssaflow.HasLibraryContract(common, ssaflow.ContractTestingCleanup) {
		return false
	}
	callback, ok := testingCleanupCallback(common)
	if !ok {
		return false
	}
	for _, receiver := range launchedMethodReceivers(spawn) {
		for _, method := range []string{"Close", "Kill", "Shutdown", "Stop"} {
			if ssaflow.ClosureCallsMethodBeforeBranch(callback, method, receiver) {
				// testing.T guarantees that Cleanup runs after the test completes.
				// Require the callback's unconditional terminating call to use the
				// exact receiver used by the launched method; capture alone does not
				// prove that the goroutine can stop.
				// https://github.com/ConSol-Monitoring/snclient/blob/35f77e9733036db52f3da12872ae6c16fc2503ad/pkg/snclient/check_dns_test.go#L21-L35
				// https://github.com/miekg/dns/blob/d854399da1ee385b432e8b07f79e53bbfc1ab1b0/server.go#L366-L445
				return true
			}
		}
	}
	return false
}

func testingCleanupJoinsGoroutine(instruction ssa.Instruction, groups []ssa.Value) bool {
	common := ssaflow.InstructionCall(instruction)
	if !ssaflow.HasLibraryContract(common, ssaflow.ContractTestingCleanup) {
		return false
	}
	callback, ok := testingCleanupCallback(common)
	if !ok {
		return false
	}
	for _, group := range groups {
		if ssaflow.ClosureCallsMethodBeforeBranch(callback, "Wait", group) {
			// testing guarantees that Cleanup runs even when the test stops early.
			// Accept only an unconditional Wait on the exact group whose terminal
			// Done settles the worker; merely capturing the group is not a join.
			// https://github.com/charmbracelet/crush/blob/6fa9e6905041c32ffceb1c9b1a3189b3db1eec07/internal/server/socket_test.go#L162-L177
			return true
		}
	}
	return false
}

func testingCleanupCallback(common *ssa.CallCommon) (ssa.Instruction, bool) {
	for _, argument := range common.Args {
		function := callbackFunction(argument)
		instruction, ok := argument.(ssa.Instruction)
		if ok && function != nil && function.Signature.Params().Len() == 0 {
			return instruction, true
		}
	}
	return nil, false
}

func launchedMethodReceivers(spawn *ssa.Go) []ssa.Value {
	if spawn == nil {
		return nil
	}
	if receiver := ssaflow.CallReceiver(spawn.Common()); lifecycleOwner(receiver) {
		return []ssa.Value{receiver}
	}
	closure, ok := spawn.Common().Value.(*ssa.MakeClosure)
	if !ok {
		return nil
	}
	function, ok := closure.Fn.(*ssa.Function)
	if !ok || len(function.Blocks) != 1 {
		return nil
	}
	// The terminating callback is relevant only when the receiver-bound call
	// defines the launched closure's whole lifetime. A branch, another call, or
	// independent blocking instruction would break that relationship.
	var launchedReceiver ssa.Value
	for _, instruction := range function.Blocks[0].Instrs {
		switch typed := instruction.(type) {
		case *ssa.Call:
			if launchedReceiver != nil {
				return nil
			}
			launchedReceiver = ssaflow.SpawnedValueAtCall(spawn, function, closure, ssaflow.CallReceiver(typed.Common()))
			if !lifecycleOwner(launchedReceiver) {
				return nil
			}
		case *ssa.UnOp:
			if typed.Op != token.MUL {
				return nil
			}
		case *ssa.ChangeInterface, *ssa.ChangeType, *ssa.Convert, *ssa.DebugRef, *ssa.Extract, *ssa.MakeInterface, *ssa.Return:
			// These instructions only expose the captured receiver, adapt its
			// type, or consume the launched call's result. They cannot extend the
			// goroutine after that call returns.
		default:
			return nil
		}
	}
	if launchedReceiver == nil {
		return nil
	}
	return []ssa.Value{launchedReceiver}
}

func lifecycleMethod(name string) bool {
	switch strings.ToLower(name) {
	case "close", "kill", "shutdown", "stop", "wait":
		return true
	default:
		return false
	}
}

func ownershipRegisteredBefore(spawn *ssa.Go, signals []ssa.Value) bool {
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
		}
	}
	return false
}

func transfersGoroutineOwnership(
	evidence *ssaflow.LocalEvidence,
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
	return mockReturnOwnsSignal(common, signals)
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
