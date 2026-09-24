package goroutineownership

import (
	"go/token"
	"go/types"
	"strings"

	"github.com/kojah/gohawk/internal/check"
	"github.com/kojah/gohawk/internal/ssaflow"
	"github.com/kojah/gohawk/internal/ssainfer"
	"github.com/kojah/gohawk/internal/syntax"

	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/ssa"
)

// Lifecycle evidence bounds a worker through exact caller-owned signals,
// cleanup, or one-hop WaitGroup dependencies. Uncertain participation is not a
// join, and absent ownership never establishes an obligation on its own.

// A straight-line Wait followed only by completion closes relays one exact
// group. Waiting independently on that group is an alternative completion
// handle; extra calls, sends, defers, or control flow make that inference opaque.
// https://github.com/ConduitIO/conduit/blob/9946a19b9fff997675f78bbc5ff437e760d39f4f/pkg/lifecycle/stream/parallel.go#L94-L103
func (analysis *spawnAnalysis) relayCompletionGroup() ssa.Value { //nolint:ireturn // Retains the caller's exact group identity.
	if analysis.config.mode == goroutineModeJoin {
		return nil
	}
	function, closure := spawnedFunction(analysis.pass, analysis.spawn)
	if len(analysis.signals) == 0 || function == nil || len(function.Blocks) != 1 || len(function.Blocks[0].Instrs) > 64 {
		return nil
	}
	var group ssa.Value
	closed := false
	for _, instruction := range function.Blocks[0].Instrs {
		switch typed := instruction.(type) {
		case *ssa.UnOp:
			if typed.Op != token.MUL {
				return nil
			}
		case *ssa.Call:
			common := typed.Common()
			switch {
			case ssaflow.CallMatchesSymbol(common, waitGroupWait) && group == nil:
				group = ssaflow.SpawnedValueAtCall(analysis.spawn, function, closure, ssaflow.CallReceiver(common))
			case group != nil && ssaflow.CallMatchesSymbol(common, syntax.Builtin("close")) && len(common.Args) == 1:
				signal := ssaflow.SpawnedValueAtCall(analysis.spawn, function, closure, common.Args[0])
				if !ssainfer.MayAliasAny(signal, analysis.signals) {
					return nil
				}
				closed = true
			default:
				return nil
			}
		case *ssa.Return, *ssa.DebugRef:
		default:
			return nil
		}
	}
	if !closed {
		return nil
	}
	return group
}

// A relay can depend on an existing external queue participant or on a worker
// already covered by the local cancellation proof. This is one-hop uncertainty,
// never group-count arithmetic or proof that every participant completes.
// https://github.com/buchgr/bazel-remote/blob/a69b6b5ed933234d93b489ffd216bee5bb74aa06/cache/disk/findmissing.go#L122-L143
// https://github.com/HM2899/grokcli-2api/blob/33a106d902d7627d1cdbc3359768a029112ea808/internal/proxy/chat.go#L445-L565
func (analysis *spawnAnalysis) relayDependencyUncertain() bool {
	if analysis.relayGroup == nil || analysis.config.mode == goroutineModeJoin {
		return false
	}
	budget := analysis.budget()
	for _, block := range analysis.function.Blocks {
		for _, instruction := range block.Instrs {
			if !budget.Spend() {
				return false
			}
			if instruction == analysis.spawn || !ssaflow.InstructionMayFollow(instruction, analysis.spawn) {
				continue
			}
			if send, ok := instruction.(*ssa.Send); ok && ssainfer.MayContainValue(send.X, analysis.relayGroup) {
				return true
			}
			worker, ok := instruction.(*ssa.Go)
			if !ok || analysis.config.mode != goroutineModeContext {
				continue
			}
			function, closure := spawnedFunction(analysis.pass, worker)
			if function == nil {
				continue
			}
			groups, _ := waitGroupCompletionValues(worker, function, closure)
			if ssainfer.MayAliasAny(analysis.relayGroup, groups) && goroutineReceivesLocallyCanceledContext(analysis.pass, worker) {
				return true
			}
		}
	}
	return false
}

// goroutineReceivesCallerSignal reports whether the worker receives from a
// channel supplied by the caller, directly or through static helpers that take
// the exact channel. Kubernetes informers express context ownership as
// Run(ctx.Done()):
// https://github.com/prometheus/prometheus/blob/e06b2dc5a6149e20ca82fe936fb044a6dfe45958/discovery/kubernetes/kubernetes.go#L438-L458
// Reminal passes its stop channel through several small helpers:
// https://github.com/harshalgajjar/Reminal/blob/c4fd9e64b3b1deabaaacd5e10b9090a28792148d/internal/client/directoryhost.go#L62-L106
func goroutineReceivesCallerSignal(pass *analysis.Pass, spawn *ssa.Go) bool {
	function, closure := spawnedFunction(pass, spawn)
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
func goroutineReceivesCallerContext(pass *analysis.Pass, spawn *ssa.Go) bool {
	function, closure := spawnedFunction(pass, spawn)
	if function == nil {
		return false
	}
	return spawnedParameterIsReceived(spawn, function, closure, func(value ssa.Value) bool {
		return syntax.NamedType(value.Type(), "context", "Context")
	})
}

var contextDoneMethod = syntax.PackageMethod(syntax.MethodSymbol{PackagePath: "context", Receiver: "Context", Name: "Done"})

// goroutineReceivesReceiverContext reports whether the worker, or a static
// helper it hands the exact value to, receives from Done on a context field of
// a caller-owned aggregate it captured or was passed, while neither the
// spawning function nor the worker stores that field. Whoever installed the
// context on the receiver owns its cancellation, so the worker is bounded by
// the caller, not by this function. This is uncertainty, never a join, and a
// worker that publishes on a channel is excluded as with every other context
// bound: it can still block after the context is done. my-geektime's segment
// downloader selects on d.ctx.Done() in a helper the worker calls on its
// captured receiver:
// https://github.com/zkep/my-geektime/blob/a614af742806cfb10f84598c71c0dd668e96549b/libs/m3u8/downloader.go#L249-L288
func goroutineReceivesReceiverContext(pass *analysis.Pass, spawn *ssa.Go) bool {
	function, closure := spawnedFunction(pass, spawn)
	if function == nil || workerHasSend(function) || workerHandsOffOutputChannel(function) {
		return false
	}
	bounded := func(local ssa.Value) bool {
		return contextFieldReceivedAnywhere(function, local, spawn.Parent(), map[*ssa.Function]bool{})
	}
	for index, parameter := range function.Params {
		if index < len(spawn.Common().Args) && ssaflow.ExternallyOwnedValue(spawn.Common().Args[index]) && bounded(parameter) {
			return true
		}
	}
	if closure == nil {
		return false
	}
	for index, free := range function.FreeVars {
		if index < len(closure.Bindings) && ssaflow.ExternallyOwnedValue(ssaflow.CapturedBindingValue(closure.Bindings[index])) && bounded(free) {
			return true
		}
	}
	return false
}

func contextFieldReceivedAnywhere(function *ssa.Function, local ssa.Value, spawner *ssa.Function, seen map[*ssa.Function]bool) bool {
	if function == nil || seen[function] {
		return false
	}
	seen[function] = true
	derives := func(value ssa.Value) bool {
		return ssainfer.ValueDerivesFrom(value, local, map[ssa.Value]bool{})
	}
	receivesContextField := func(channel ssa.Value) bool {
		done, ok := channel.(*ssa.Call)
		if !ok || !ssaflow.CallMatchesSymbol(done.Common(), contextDoneMethod) {
			return false
		}
		field := loadedContextField(ssaflow.CallReceiver(done.Common()))
		return field != nil && derives(field.X) && !fieldStoredIn(field, spawner) && !fieldStoredIn(field, function)
	}
	for _, block := range function.Blocks {
		for _, instruction := range block.Instrs {
			if receivesFrom(instruction, receivesContextField) {
				return true
			}
			common := ssaflow.InstructionCall(instruction)
			if common == nil {
				continue
			}
			callee, closure := ssaflow.DirectCallee(common)
			if callee == nil {
				continue
			}
			for _, pair := range ssaflow.CallBindings(common, callee, closure) {
				if derives(pair.Supplied) && contextFieldReceivedAnywhere(callee, pair.Local, spawner, seen) {
					return true
				}
			}
		}
	}
	return false
}

// loadedContextField returns the field address a context value was loaded
// from, when that field is typed as the standard Context interface.
func loadedContextField(value ssa.Value) *ssa.FieldAddr {
	load, ok := value.(*ssa.UnOp)
	if !ok || load.Op != token.MUL {
		return nil
	}
	field, ok := load.X.(*ssa.FieldAddr)
	if !ok || !syntax.NamedType(field.Type().(*types.Pointer).Elem(), "context", "Context") {
		return nil
	}
	return field
}

// fieldStoredIn reports a visible store to the same field of any value of the
// same type inside function: the context may then be one this function chose,
// not one the receiver's owner installed.
func fieldStoredIn(field *ssa.FieldAddr, function *ssa.Function) bool {
	for _, store := range ssaflow.InstructionsOf[*ssa.Store](function) {
		address, ok := store.Addr.(*ssa.FieldAddr)
		if ok && address.Field == field.Field && types.Identical(address.X.Type(), field.X.Type()) {
			return true
		}
	}
	return false
}

// A locally created context can bound a worker when its exact cancellation is
// deferred before launch or covers every later return. Cancellation is not a join; the caller uses
// this only to decline the default context-mode diagnostic.
// https://github.com/c9s/bbgo/blob/4a4a18a08897579d157c8fd3b412309cd4954852/pkg/cmd/exchangetest.go#L233-L255
// https://github.com/inbucket/inbucket/blob/94472ab496822dec612edce7988a1f953a9eafbd/pkg/msghub/hub_test.go#L361-L372
func goroutineReceivesLocallyCanceledContext(pass *analysis.Pass, spawn *ssa.Go) bool {
	function, closure := spawnedFunction(pass, spawn)
	if function == nil {
		return false
	}
	storage := ssainfer.NewStorage(nil)
	for _, pair := range ssaflow.CallBindings(spawn.Common(), function, closure) {
		value := pair.Supplied
		if _, cell := value.(*ssa.Alloc); cell {
			content := storage.StableContent(value, spawn)
			if !content.Proven() {
				continue
			}
			value = content.Value
		}
		contextResult, ok := value.(*ssa.Extract)
		if !ok || contextResult.Index != 0 {
			continue
		}
		call, ok := contextResult.Tuple.(*ssa.Call)
		if !ok || call.Parent() != spawn.Parent() ||
			!ssaflow.CallMatchesSymbol(call.Common(), syntax.PackageFunction("context", "WithCancel")) {
			continue
		}
		cancel := ssaflow.CallResult(call, 1)
		owned := cancelCoversSpawn(spawn, cancel, storage)
		if owned && receivesAnywhere(function, pair.Local, map[*ssa.Function]bool{}) {
			return true
		}
		// An imported or dynamic helper receiving this exact canceled context
		// may own the worker's shutdown. Missing its body is uncertainty, not
		// evidence that the worker ignores cancellation. This remains unknown.
		// https://github.com/iximiuz/cdebug/blob/6c205f0b663df4dec235f42e905e94b40709159a/pkg/containerd/client.go#L98-L122
		if owned && closure != nil {
			evidence, _ := summaryKnowledge.Provider(pass).LifecycleEvidence("goroutineownership", string(check.GoroutineJoin))
			if evidence.ClosureHandsValueToUnreadableCallee(closure, value) {
				return true
			}
		}
	}
	return false
}

// Cancellation is an alternative lifetime boundary, never a join. The same
// every-return query must cover later calls/defers; conditional cancellation
// and an asynchronous invocation do not satisfy it.
func cancelCoversSpawn(spawn *ssa.Go, cancel ssa.Value, storage *ssainfer.Storage) bool {
	cancels := func(instruction ssa.Instruction) bool {
		common := ssaflow.InstructionCall(instruction)
		if common == nil {
			return false
		}
		_, called := instruction.(*ssa.Call)
		_, deferred := instruction.(*ssa.Defer)
		return (called || deferred) && storage.Same(common.Value, cancel).Proven()
	}
	if !ssaflow.UnownedReturn(spawn, cancels, nil) {
		return true
	}
	for _, deferred := range ssaflow.InstructionsOf[*ssa.Defer](spawn.Parent()) {
		if ssaflow.InstructionDominates(deferred, spawn) && cancels(deferred) {
			return true
		}
	}
	return false
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
		return ssainfer.ValueDerivesFrom(value, local, map[ssa.Value]bool{})
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
			callee, closure := ssaflow.DirectCallee(common)
			if callee == nil {
				continue
			}
			for _, pair := range ssaflow.CallBindings(common, callee, closure) {
				if derives(pair.Supplied) && receivesAnywhere(callee, pair.Local, seen) {
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
	function, _ := callbackTarget(value)
	return function
}

// spawnedLifecycleOwners returns the receiver and captured values that expose
// a lifecycle method. This is the only name-based evidence in the analyzer and
// it can only suppress a diagnostic, never establish an obligation. A WaitGroup
// is excluded so its Wait cannot bypass the terminal Done proof.
func spawnedLifecycleOwners(pass *analysis.Pass, spawn *ssa.Go) []ssa.Value {
	var owners []ssa.Value
	if receiver := ssaflow.CallReceiver(spawn.Common()); lifecycleOwner(receiver) {
		owners = append(owners, receiver)
	}
	_, closure := spawnedFunction(pass, spawn)
	if closure == nil {
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

// ownerReceiver matches a lifecycle call's receiver against the tracked
// owners. A type assertion on the captured interface value denotes the same
// object, so closing the asserted listener stops the worker that captured the
// interface. NATS asserts its listener before deferring the close:
// https://github.com/nats-io/nats.go/blob/850f889cf3d63bfd1a549ab9af59f0145146fb41/nats_test.go#L1288-L1301
func ownerReceiver(receiver ssa.Value, owners []ssa.Value) bool {
	if ssainfer.MayAliasAny(receiver, owners) {
		return true
	}
	if extract, ok := receiver.(*ssa.Extract); ok {
		receiver = extract.Tuple
	}
	asserted, ok := receiver.(*ssa.TypeAssert)
	return ok && ssainfer.MayAliasAny(asserted.X, owners)
}

func lifecycleMethod(name string) bool {
	switch strings.ToLower(name) {
	case "close", "kill", "shutdown", "stop", "wait":
		return true
	default:
		return false
	}
}
