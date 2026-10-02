package goroutineownership

import (
	"go/token"
	"go/types"

	"github.com/kojah/gohawk/internal/check"
	"github.com/kojah/gohawk/internal/heapmodel"
	"github.com/kojah/gohawk/internal/ssaflow"
	"github.com/kojah/gohawk/internal/syntax"
	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/ssa"
)

// Caller lifetime bounds explain possible shutdown through supplied channels,
// stable receiver contexts or covered local cancellation. They never prove a
// join. All direct census and coverage queries spend the caller's allowance;
// the authoritative lifecycle proof checks availability before naming a bound.

// goroutineReceivesCallerSignal reports whether the worker receives from a
// channel supplied by the caller, directly or through static helpers that take
// the exact channel. Kubernetes informers express context ownership as
// Run(ctx.Done()):
// https://github.com/prometheus/prometheus/blob/e06b2dc5a6149e20ca82fe936fb044a6dfe45958/discovery/kubernetes/kubernetes.go#L438-L458
// Reminal passes its stop channel through several small helpers:
// https://github.com/harshalgajjar/Reminal/blob/c4fd9e64b3b1deabaaacd5e10b9090a28792148d/internal/client/directoryhost.go#L62-L106
func goroutineReceivesCallerSignal(pass *analysis.Pass, spawn *ssa.Go, budget *ssaflow.SearchBudget) bool {
	function, closure := resolveSpawnedFunction(pass, spawn, budget)
	if function == nil {
		return false
	}
	for instruction := range ssaflow.InstructionsWithin(function, budget) {
		if receivesFromWithin(instruction, func(channel ssa.Value) bool {
			return callerSuppliedValue(spawn, function, closure, channel, budget)
		}, budget) {
			return true
		}
	}
	if budget.Exhausted() {
		return false
	}
	return spawnedParameterIsReceived(spawn, function, closure, func(value ssa.Value) bool {
		channel, ok := value.Type().Underlying().(*types.Chan)
		return ok && channel.Dir() == types.RecvOnly
	}, budget)
}

// goroutineReceivesCallerContext reports whether the worker, or a static helper
// it passes the exact context to, receives from a caller-owned context.
func goroutineReceivesCallerContext(pass *analysis.Pass, spawn *ssa.Go, budget *ssaflow.SearchBudget) bool {
	function, closure := resolveSpawnedFunction(pass, spawn, budget)
	if function == nil {
		return false
	}
	return spawnedParameterIsReceived(spawn, function, closure, func(value ssa.Value) bool {
		return syntax.NamedType(value.Type(), "context", "Context")
	}, budget)
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
func goroutineReceivesReceiverContext(pass *analysis.Pass, spawn *ssa.Go, budget *ssaflow.SearchBudget) bool {
	function, closure := resolveSpawnedFunction(pass, spawn, budget)
	if function == nil {
		return false
	}
	publishes := workerHasSendWithin(function, budget) || workerHandsOffOutputChannelWithin(function, budget)
	if publishes || budget.Exhausted() {
		return false
	}
	bounded := func(local ssa.Value) bool {
		return contextFieldReceivedAnywhere(function, local, spawn.Parent(), budget)
	}
	for binding := range ssaflow.CallBindingsWithin(spawn.Common(), function, closure, budget) {
		if bindingIsExternallyOwned(binding, budget) && bounded(binding.Local) {
			return true
		}
	}
	return false
}

func contextFieldReceivedAnywhere(function *ssa.Function, local ssa.Value, spawner *ssa.Function, budget *ssaflow.SearchBudget) bool {
	search := newWorkerReceiveSearch(budget, func(body *ssa.Function, target, channel ssa.Value) bool {
		done, ok := channel.(*ssa.Call)
		if !ok || !ssaflow.CallMatchesSymbol(done.Common(), contextDoneMethod) {
			return false
		}
		field := loadedContextField(ssaflow.CallReceiver(done.Common()))
		return field != nil && heapmodel.ValueDerivesFrom(field.X, target) && !fieldStoredIn(field, spawner, budget) && !fieldStoredIn(field, body, budget)
	})
	return search.prove(function, local).Proven()
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
func fieldStoredIn(field *ssa.FieldAddr, function *ssa.Function, budget *ssaflow.SearchBudget) bool {
	for instruction := range ssaflow.InstructionsWithin(function, budget) {
		store, ok := instruction.(*ssa.Store)
		if !ok {
			continue
		}
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
func goroutineReceivesLocallyCanceledContext(pass *analysis.Pass, spawn *ssa.Go, budget *ssaflow.SearchBudget) bool {
	function, closure := resolveSpawnedFunction(pass, spawn, budget)
	if function == nil {
		return false
	}
	storage := heapmodel.NewStorage(budget)
	for pair := range ssaflow.CallBindingsWithin(spawn.Common(), function, closure, budget) {
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
		cancel := ssaflow.CallResultWithin(call, 1, budget)
		if budget.Exhausted() {
			return false
		}
		owned := cancelCoversSpawn(spawn, cancel, storage)
		if owned && receivesAnywhere(function, pair.Local, budget) {
			return true
		}
		// An imported or dynamic helper receiving this exact canceled context
		// may own the worker's shutdown. Missing its body is uncertainty, not
		// evidence that the worker ignores cancellation. This remains unknown.
		// https://github.com/iximiuz/cdebug/blob/6c205f0b663df4dec235f42e905e94b40709159a/pkg/containerd/client.go#L98-L122
		if owned && closure != nil {
			evidence, _ := summaryKnowledge.Provider(pass).LifecycleEvidence("goroutineownership", string(check.GoroutineJoin))
			if evidence.ClosureHandsValueToUnreadableCalleeWithin(closure, value, budget) {
				return true
			}
		}
	}
	return false
}

// Cancellation is an alternative lifetime boundary, never a join. The same
// every-return query must cover later calls/defers; conditional cancellation
// and an asynchronous invocation do not satisfy it.
func cancelCoversSpawn(spawn *ssa.Go, cancel ssa.Value, storage *heapmodel.Storage) bool {
	cancels := func(instruction ssa.Instruction) bool {
		common := ssaflow.InstructionCall(instruction)
		if common == nil {
			return false
		}
		_, called := instruction.(*ssa.Call)
		_, deferred := instruction.(*ssa.Defer)
		return (called || deferred) && storage.Same(common.Value, cancel).Proven()
	}
	budget := storage.Budget()
	outcome := ssaflow.EvaluateObligation(ssaflow.ObligationFlow{
		Start: spawn, Budget: budget, Instruction: func(instruction ssa.Instruction) ssaflow.ObligationAction {
			if !budget.Spend() {
				return ssaflow.ObligationUnknown
			}
			if cancels(instruction) {
				return ssaflow.ObligationExact
			}
			return ssaflow.ObligationNone
		},
	})
	if budget.Exhausted() {
		return false
	}
	if outcome == ssaflow.ObligationHonored {
		return true
	}
	// A registered defer may precede launch and lie outside the forward
	// query. Only dominance makes that earlier registration cover this worker.
	for instruction := range ssaflow.InstructionsWithin(spawn.Parent(), budget) {
		deferred, ok := instruction.(*ssa.Defer)
		if !ok {
			continue
		}
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
func callerSuppliedValue(
	spawn *ssa.Go, function *ssa.Function, closure *ssa.MakeClosure, value ssa.Value, budget *ssaflow.SearchBudget,
) bool {
	if supplied := ssaflow.SpawnedValueAtCallWithin(spawn, function, closure, value, budget); supplied != nil {
		return ssaflow.ExternallyOwnedValue(supplied)
	}
	if closure == nil {
		return false
	}
	for captured := range ssaflow.ClosureBindingPairsWithin(function, closure, budget) {
		if ssaflow.ValueIsAccessPathFromWithin(value, captured.Free, budget) &&
			ssaflow.ExternallyOwnedValue(ssaflow.CapturedBindingValueWithin(captured.Binding, budget)) {
			return true
		}
	}
	return false
}

// spawnedParameterIsReceived reports whether a caller-owned parameter or
// capture accepted by typed is received from by the worker or by a static
// helper chain it hands the exact value to.
func spawnedParameterIsReceived(
	spawn *ssa.Go,
	function *ssa.Function,
	closure *ssa.MakeClosure,
	typed func(ssa.Value) bool,
	budget *ssaflow.SearchBudget,
) bool {
	for binding := range ssaflow.CallBindingsWithin(spawn.Common(), function, closure, budget) {
		if typed(binding.Local) && bindingIsExternallyOwned(binding, budget) &&
			receivesAnywhere(function, binding.Local, budget) {
			return true
		}
	}
	return false
}

// A captured cell supplies its contents to the worker. An ordinary argument
// supplies the value evaluated at the launch; loading it would change which
// value the caller-owned lifetime boundary applies to.
func bindingIsExternallyOwned(binding ssaflow.CallBinding, budget *ssaflow.SearchBudget) bool {
	supplied := binding.Supplied
	if binding.Captured {
		supplied = ssaflow.CapturedBindingValueWithin(supplied, budget)
	}
	return ssaflow.ExternallyOwnedValue(supplied)
}

// receivesAnywhere reports whether function, or a static helper it hands the
// exact value to, receives from local on any path. A bounded worker commonly
// selects on its stop signal inside a loop, so every-return coverage is not
// required here; this evidence never proves a join, only a caller-owned bound.
func receivesAnywhere(function *ssa.Function, local ssa.Value, budget *ssaflow.SearchBudget) bool {
	search := newWorkerReceiveSearch(budget, func(_ *ssa.Function, target, channel ssa.Value) bool {
		return heapmodel.ValueDerivesFrom(channel, target)
	})
	return search.prove(function, local).Proven()
}
